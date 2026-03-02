package crawler

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/derr/pulse/internal/models"
	"go.uber.org/zap"
	"golang.org/x/time/rate"
)

// RedditCrawler fetches posts from Reddit's public JSON API (no OAuth required)
type RedditCrawler struct {
	client      *http.Client
	limiter     *rate.Limiter
	maxRetries  int
	baseURL     string
	userAgent   string
	concurrency int
	logger      *zap.Logger
}

// NewRedditCrawler constructs a RedditCrawler
func NewRedditCrawler(baseURL, userAgent string, rps, concurrency, maxRetries int, logger *zap.Logger) *RedditCrawler {
	return &RedditCrawler{
		client:      &http.Client{Timeout: 10 * time.Second},
		limiter:     rate.NewLimiter(rate.Limit(rps), rps),
		maxRetries:  maxRetries,
		baseURL:     baseURL,
		userAgent:   userAgent,
		concurrency: concurrency,
		logger:      logger,
	}
}

// FetchSubreddits concurrently scrapes multiple subreddits
func (c *RedditCrawler) FetchSubreddits(ctx context.Context, subreddits []string, postsPerSub int) ([]models.RawPost, error) {
	subCh := make(chan string, len(subreddits))
	for _, s := range subreddits {
		subCh <- s
	}
	close(subCh)

	var (
		mu      sync.Mutex
		allPosts []models.RawPost
		wg      sync.WaitGroup
	)

	for i := 0; i < c.concurrency; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for sub := range subCh {
				posts, err := c.fetchSubreddit(ctx, sub, postsPerSub)
				if err != nil {
					c.logger.Warn("failed to fetch subreddit", zap.String("sub", sub), zap.Error(err))
					continue
				}
				mu.Lock()
				allPosts = append(allPosts, posts...)
				mu.Unlock()
				c.logger.Info("fetched subreddit", zap.String("sub", sub), zap.Int("count", len(posts)))
			}
		}()
	}

	wg.Wait()
	return allPosts, nil
}

// fetchSubreddit retrieves top posts from a single subreddit
func (c *RedditCrawler) fetchSubreddit(ctx context.Context, subreddit string, limit int) ([]models.RawPost, error) {
	url := fmt.Sprintf("%s/r/%s/hot.json?limit=%d", c.baseURL, subreddit, limit)

	if err := c.limiter.Wait(ctx); err != nil {
		return nil, err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", c.userAgent)

	var lastErr error
	for attempt := 0; attempt <= c.maxRetries; attempt++ {
		if attempt > 0 {
			time.Sleep(time.Duration(1<<uint(attempt-1)) * time.Second)
		}

		resp, err := c.client.Do(req)
		if err != nil {
			lastErr = err
			continue
		}
		defer resp.Body.Close()

		if resp.StatusCode == http.StatusTooManyRequests {
			lastErr = fmt.Errorf("rate limited (429)")
			continue
		}
		if resp.StatusCode != http.StatusOK {
			lastErr = fmt.Errorf("status %d", resp.StatusCode)
			continue
		}

		var listing models.RedditListing
		if err := json.NewDecoder(resp.Body).Decode(&listing); err != nil {
			return nil, fmt.Errorf("decoding reddit response: %w", err)
		}

		posts := make([]models.RawPost, 0, len(listing.Data.Children))
		for _, child := range listing.Data.Children {
			posts = append(posts, normaliseRedditPost(child.Data))
		}
		return posts, nil
	}

	return nil, fmt.Errorf("all retries exhausted: %w", lastErr)
}

// normaliseRedditPost maps Reddit API fields to the shared RawPost schema
func normaliseRedditPost(p models.RedditPost) models.RawPost {
	return models.RawPost{
		ID:          fmt.Sprintf("reddit_%s", p.ID),
		Source:      "reddit",
		Author:      p.Author,
		Title:       p.Title,
		Body:        p.Selftext,
		URL:         p.URL,
		Score:       p.Score,
		NumComments: p.NumComments,
		CreatedAt:   time.Unix(int64(p.CreatedUTC), 0).UTC(),
		FetchedAt:   time.Now().UTC(),
		Subreddit:   p.Subreddit,
	}
}
