package config

import (
	"encoding/json"
	"os"
	"strconv"

	"github.com/joho/godotenv"
)

// Config holds all application configuration
type Config struct {
	// Database
	DatabaseURL string

	// Crawler settings
	HNTopStoriesURL  string
	RedditBaseURL    string
	RedditUserAgent  string
	MaxConcurrency   int
	RequestsPerSec   int
	MaxRetries       int

	// Reddit targets (comma-separated subreddits)
	Subreddits []string
}

// Load reads config from environment variables (with .env fallback)
func Load() *Config {
	_ = godotenv.Load() // non-fatal if .env is absent

	return &Config{
		DatabaseURL:     getEnv("DATABASE_URL", "postgres://postgres:postgres@localhost:5432/pulse?sslmode=disable"),
		HNTopStoriesURL: getEnv("HN_TOP_STORIES_URL", "https://hacker-news.firebaseio.com/v0/topstories.json"),
		RedditBaseURL:   getEnv("REDDIT_BASE_URL", "https://www.reddit.com"),
		RedditUserAgent: getEnv("REDDIT_USER_AGENT", "pulse/1.0 (data research)"),
		MaxConcurrency:  getEnvInt("MAX_CONCURRENCY", 10),
		RequestsPerSec:  getEnvInt("REQUESTS_PER_SEC", 5),
		MaxRetries:      getEnvInt("MAX_RETRIES", 3),
		Subreddits:      []string{"golang", "programming", "datascience", "MachineLearning", "finance"},
	}
}

func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func getEnvInt(key string, fallback int) int {
	if v := os.Getenv(key); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			return n
		}
	}
	return fallback
}

// TopicKeywords maps signal names to keyword lists used for scoring (tech_interest, ai_interest, etc.)
type TopicKeywords map[string][]string

// LoadTopicKeywords reads config/topic_keywords.json if it exists. Returns nil on missing file or error (caller uses built-in defaults).
func LoadTopicKeywords(path string) TopicKeywords {
	if path == "" {
		path = "config/topic_keywords.json"
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	var out TopicKeywords
	if err := json.Unmarshal(data, &out); err != nil {
		return nil
	}
	if len(out) == 0 {
		return nil
	}
	return out
}
