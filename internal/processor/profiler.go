package processor

import (
	"time"

	"github.com/derr/pulse/internal/models"
)

// BuildUserProfiles aggregates raw posts into per-user profile summaries
func BuildUserProfiles(posts []models.RawPost) map[string]*models.UserProfile {
	profiles := make(map[string]*models.UserProfile)

	for _, post := range posts {
		key := post.Source + ":" + post.Author
		if post.Author == "" || post.Author == "[deleted]" {
			continue
		}

		p, exists := profiles[key]
		if !exists {
			p = &models.UserProfile{
				Username:      post.Author,
				Source:        post.Source,
				ActiveHours:   make(map[int]int),
				TopSubreddits: make(map[string]int),
				Signals:       make(map[string]float64),
				FirstSeen:     post.CreatedAt,
				LastSeen:      post.CreatedAt,
			}
			profiles[key] = p
		}

		// Accumulate metrics
		p.PostCount++
		p.TotalEngagement += post.Score + post.NumComments*2
		p.ActiveHours[post.CreatedAt.Hour()]++

		if post.Subreddit != "" {
			p.TopSubreddits[post.Subreddit]++
		}
		if post.CreatedAt.Before(p.FirstSeen) {
			p.FirstSeen = post.CreatedAt
		}
		if post.CreatedAt.After(p.LastSeen) {
			p.LastSeen = post.CreatedAt
		}

		// Merge keyword tags
		p.TopKeywords = mergeTopKeywords(p.TopKeywords, post.Tags, 15)
	}

	// Post-process: compute derived fields
	for _, p := range profiles {
		if p.PostCount > 0 {
			p.AvgScore = float64(p.TotalEngagement) / float64(p.PostCount)
		}
		p.UpdatedAt = time.Now().UTC()
	}

	return profiles
}

// mergeTopKeywords merges two keyword slices, deduplicating, capped at maxN
func mergeTopKeywords(existing, incoming []string, maxN int) []string {
	seen := make(map[string]bool)
	result := make([]string, 0, maxN)
	for _, k := range existing {
		if !seen[k] {
			seen[k] = true
			result = append(result, k)
		}
	}
	for _, k := range incoming {
		if !seen[k] && len(result) < maxN {
			seen[k] = true
			result = append(result, k)
		}
	}
	return result
}
