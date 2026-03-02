package crawler

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sync"
	"time"

	"github.com/derr/pulse/internal/models"
	"go.uber.org/zap"
	"golang.org/x/time/rate"
)

// HNCrawler fetches stories from the HackerNews Firebase API
type HNCrawler struct {
	client      *http.Client
	limiter     *rate.Limiter
	maxRetries  int
	topURL      string
	concurrency int
	logger      *zap.Logger
}

// NewHNCrawler constructs a HNCrawler with rate limiting baked in
func NewHNCrawler(topURL string, rps, concurrency, maxRetries int, logger *zap.Logger) *HNCrawler {
	return &HNCrawler{
		client:      &http.Client{Timeout: 10 * time.Second},
		limiter:     rate.NewLimiter(rate.Limit(rps), rps),
		maxRetries:  maxRetries,
		topURL:      topURL,
		concurrency: concurrency,
		logger:      logger,
	}
}

// FetchTopStories concurrently fetches the top N HN stories
func (c *HNCrawler) FetchTopStories(ctx context.Context, limit int) ([]models.RawPost, error) {
	ids, err := c.fetchTopIDs(ctx)
	if err != nil {
		return nil, fmt.Errorf("fetching top story IDs: %w", err)
	}
	if len(ids) > limit {
		ids = ids[:limit]
	}

	// Fan-out: dispatch IDs to a worker pool via channel
	idCh := make(chan int, len(ids))
	for _, id := range ids {
		idCh <- id
	}
	close(idCh)

	resultCh := make(chan models.RawPost, len(ids))
	var wg sync.WaitGroup

	for i := 0; i < c.concurrency; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for id := range idCh {
				post, err := c.fetchItemWithRetry(ctx, id)
				if err != nil {
					c.logger.Warn("failed to fetch HN item", zap.Int("id", id), zap.Error(err))
					continue
				}
				resultCh <- post
			}
		}()
	}

	// Close result channel once all workers finish
	go func() {
		wg.Wait()
		close(resultCh)
	}()

	var posts []models.RawPost
	for p := range resultCh {
		posts = append(posts, p)
	}

	c.logger.Info("HN crawl complete", zap.Int("fetched", len(posts)))
	return posts, nil
}

// fetchTopIDs retrieves the ordered list of top story IDs
func (c *HNCrawler) fetchTopIDs(ctx context.Context) ([]int, error) {
	body, err := c.getWithRetry(ctx, c.topURL)
	if err != nil {
		return nil, err
	}
	var ids []int
	if err := json.Unmarshal(body, &ids); err != nil {
		return nil, fmt.Errorf("unmarshalling top IDs: %w", err)
	}
	return ids, nil
}

// fetchItemWithRetry retrieves a single HN item and normalises it
func (c *HNCrawler) fetchItemWithRetry(ctx context.Context, id int) (models.RawPost, error) {
	url := fmt.Sprintf("https://hacker-news.firebaseio.com/v0/item/%d.json", id)
	body, err := c.getWithRetry(ctx, url)
	if err != nil {
		return models.RawPost{}, err
	}

	var item models.HNItem
	if err := json.Unmarshal(body, &item); err != nil {
		return models.RawPost{}, fmt.Errorf("unmarshalling item %d: %w", id, err)
	}

	return normaliseHNItem(item), nil
}

// getWithRetry performs an HTTP GET with exponential backoff on failure
func (c *HNCrawler) getWithRetry(ctx context.Context, url string) ([]byte, error) {
	var lastErr error
	for attempt := 0; attempt <= c.maxRetries; attempt++ {
		if attempt > 0 {
			// Exponential backoff: 1s, 2s, 4s …
			backoff := time.Duration(1<<uint(attempt-1)) * time.Second
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(backoff):
			}
		}

		// Honour rate limit before every request
		if err := c.limiter.Wait(ctx); err != nil {
			return nil, fmt.Errorf("rate limiter: %w", err)
		}

		req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
		if err != nil {
			return nil, err
		}

		resp, err := c.client.Do(req)
		if err != nil {
			lastErr = err
			c.logger.Warn("request failed, retrying", zap.String("url", url), zap.Int("attempt", attempt), zap.Error(err))
			continue
		}
		defer resp.Body.Close()

		if resp.StatusCode == http.StatusTooManyRequests {
			lastErr = fmt.Errorf("rate limited by server (429)")
			continue
		}
		if resp.StatusCode != http.StatusOK {
			lastErr = fmt.Errorf("unexpected status %d for %s", resp.StatusCode, url)
			continue
		}

		data, err := io.ReadAll(resp.Body)
		if err != nil {
			lastErr = err
			continue
		}
		return data, nil
	}
	return nil, fmt.Errorf("all retries exhausted for %s: %w", url, lastErr)
}

// normaliseHNItem converts a raw HN API response to the shared RawPost model
func normaliseHNItem(item models.HNItem) models.RawPost {
	return models.RawPost{
		ID:          fmt.Sprintf("hn_%d", item.ID),
		Source:      "hackernews",
		Author:      item.By,
		Title:       item.Title,
		Body:        item.Text,
		URL:         item.URL,
		Score:       item.Score,
		NumComments: item.Descendants,
		CreatedAt:   time.Unix(item.Time, 0).UTC(),
		FetchedAt:   time.Now().UTC(),
	}
}
