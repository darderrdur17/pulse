package main

import (
	"context"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/derr/pulse/config"
	"github.com/derr/pulse/internal/crawler"
	"github.com/derr/pulse/internal/kafka"
	"github.com/derr/pulse/internal/processor"
	"github.com/derr/pulse/internal/signals"
	"github.com/derr/pulse/internal/storage"
	"go.uber.org/zap"
)

func main() {
	// ── Logger ──────────────────────────────────────────────────────────────
	logger, _ := zap.NewProduction()
	defer logger.Sync()

	// ── Config ──────────────────────────────────────────────────────────────
	cfg := config.Load()
	logger.Info("pipeline starting",
		zap.Int("concurrency", cfg.MaxConcurrency),
		zap.Int("rps", cfg.RequestsPerSec),
	)

	// ── Graceful shutdown via OS signals ────────────────────────────────────
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	// ── Storage ─────────────────────────────────────────────────────────────
	store, err := storage.New(cfg.DatabaseURL)
	if err != nil {
		logger.Fatal("connecting to database", zap.Error(err))
	}
	defer store.Close()

	if err := store.Migrate(ctx); err != nil {
		logger.Fatal("running migrations", zap.Error(err))
	}
	logger.Info("database ready")

	// ── Crawlers ────────────────────────────────────────────────────────────
	hnCrawler := crawler.NewHNCrawler(
		cfg.HNTopStoriesURL, cfg.RequestsPerSec, cfg.MaxConcurrency, cfg.MaxRetries, logger,
	)
	redditCrawler := crawler.NewRedditCrawler(
		cfg.RedditBaseURL, cfg.RedditUserAgent,
		cfg.RequestsPerSec, cfg.MaxConcurrency, cfg.MaxRetries, logger,
	)

	proc := processor.New()
	keywords := config.LoadTopicKeywords("")
	if keywords != nil {
		logger.Info("loaded custom topic keywords", zap.Int("signals", len(keywords)))
	}
	sigExtractor := signals.NewWithKeywords(keywords)

	// ── Kafka (optional) ─────────────────────────────────────────────────────
	var kafkaProducer *kafka.Producer
	if cfg.KafkaBrokers != "" {
		var err error
		kafkaProducer, err = kafka.NewProducer(cfg.KafkaBrokers, cfg.KafkaTopic, logger)
		if err != nil {
			logger.Fatal("kafka producer", zap.Error(err))
		}
		defer kafkaProducer.Close()
		logger.Info("kafka enabled", zap.String("brokers", cfg.KafkaBrokers), zap.String("topic", cfg.KafkaTopic))
	}

	// ── Run pipeline ────────────────────────────────────────────────────────
	for {
		runCycle(ctx, logger, cfg, hnCrawler, redditCrawler, proc, sigExtractor, store, kafkaProducer)

		logger.Info("cycle complete — sleeping 15 minutes")
		select {
		case <-ctx.Done():
			logger.Info("shutdown signal received")
			return
		case <-time.After(15 * time.Minute):
		}
	}
}

// runCycle executes one full crawl → process → store cycle
func runCycle(
	ctx context.Context,
	logger *zap.Logger,
	cfg *config.Config,
	hn *crawler.HNCrawler,
	reddit *crawler.RedditCrawler,
	proc *processor.Processor,
	sigExt *signals.Extractor,
	store *storage.Store,
	kafkaProducer *kafka.Producer,
) {
	start := time.Now()
	logger.Info("starting crawl cycle")

	// 1. Crawl ──────────────────────────────────────────────────────────────
	hnPosts, err := hn.FetchTopStories(ctx, 50)
	if err != nil {
		logger.Error("HN crawl failed", zap.Error(err))
	}

	redditPosts, err := reddit.FetchSubreddits(ctx, cfg.Subreddits, 25)
	if err != nil {
		logger.Error("Reddit crawl failed", zap.Error(err))
	}

	allPosts := append(hnPosts, redditPosts...)
	logger.Info("crawl complete", zap.Int("total_posts", len(allPosts)))

	// 2. Enrich posts (keyword extraction, engagement scoring) ───────────────
	enriched := proc.EnrichBatch(allPosts)

	// 2b. Produce to Kafka (optional) ────────────────────────────────────────
	if kafkaProducer != nil {
		if n, err := kafkaProducer.SendPosts(ctx, enriched); err != nil {
			logger.Error("kafka produce", zap.Error(err))
		} else if n > 0 {
			logger.Info("kafka produced", zap.Int("count", n))
		}
	}

	// 3. Store posts ─────────────────────────────────────────────────────────
	saved, err := store.UpsertPosts(ctx, enriched)
	if err != nil {
		logger.Error("storing posts", zap.Error(err))
	} else {
		logger.Info("posts stored", zap.Int("count", saved))
	}

	// 4. Build user profiles ─────────────────────────────────────────────────
	profiles := processor.BuildUserProfiles(enriched)
	logger.Info("profiles built", zap.Int("users", len(profiles)))

	// 5. Compute signals per profile ─────────────────────────────────────────
	for _, p := range profiles {
		sigExt.ComputeProfileSignals(p)
	}

	// 6. Store profiles ──────────────────────────────────────────────────────
	savedProfiles, err := store.UpsertProfiles(ctx, profiles)
	if err != nil {
		logger.Error("storing profiles", zap.Error(err))
	} else {
		logger.Info("profiles stored", zap.Int("count", savedProfiles))
	}

	logger.Info("cycle finished", zap.Duration("elapsed", time.Since(start)))
}
