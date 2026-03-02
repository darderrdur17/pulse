package models

import "time"

// RawPost represents a normalised post from any social media source
type RawPost struct {
	ID          string    `json:"id"`
	Source      string    `json:"source"` // "hackernews" | "reddit"
	Author      string    `json:"author"`
	Title       string    `json:"title"`
	Body        string    `json:"body"`
	URL         string    `json:"url"`
	Score       int       `json:"score"`
	NumComments int       `json:"num_comments"`
	CreatedAt   time.Time `json:"created_at"`
	FetchedAt   time.Time `json:"fetched_at"`
	Tags        []string  `json:"tags"`
	Subreddit   string    `json:"subreddit,omitempty"`
}

// UserProfile represents an aggregated profile built from user activity
type UserProfile struct {
	Username        string             `json:"username"`
	Source          string             `json:"source"`
	PostCount       int                `json:"post_count"`
	AvgScore        float64            `json:"avg_score"`
	TotalEngagement int                `json:"total_engagement"`
	TopKeywords     []string           `json:"top_keywords"`
	ActiveHours     map[int]int        `json:"active_hours"`    // hour -> post count
	TopSubreddits   map[string]int     `json:"top_subreddits"`  // subreddit -> count
	Signals         map[string]float64 `json:"signals"`         // computed signal scores
	FirstSeen       time.Time          `json:"first_seen"`
	LastSeen        time.Time          `json:"last_seen"`
	UpdatedAt       time.Time          `json:"updated_at"`
}

// Signal represents a scored intelligence signal extracted from data
type Signal struct {
	Name        string    `json:"name"`
	Score       float64   `json:"score"`     // 0.0 - 1.0
	Confidence  float64   `json:"confidence"`
	Description string    `json:"description"`
	ComputedAt  time.Time `json:"computed_at"`
}

// HNItem is the raw HackerNews API response shape
type HNItem struct {
	ID          int    `json:"id"`
	Type        string `json:"type"`
	By          string `json:"by"`
	Title       string `json:"title"`
	Text        string `json:"text"`
	URL         string `json:"url"`
	Score       int    `json:"score"`
	Descendants int    `json:"descendants"`
	Time        int64  `json:"time"`
	Kids        []int  `json:"kids"`
}

// RedditListing is the raw Reddit API response envelope
type RedditListing struct {
	Data struct {
		Children []struct {
			Data RedditPost `json:"data"`
		} `json:"children"`
		After string `json:"after"`
	} `json:"data"`
}

// RedditPost is a single post from the Reddit API
type RedditPost struct {
	ID          string  `json:"id"`
	Author      string  `json:"author"`
	Title       string  `json:"title"`
	Selftext    string  `json:"selftext"`
	URL         string  `json:"url"`
	Score       int     `json:"score"`
	NumComments int     `json:"num_comments"`
	CreatedUTC  float64 `json:"created_utc"`
	Subreddit   string  `json:"subreddit"`
}
