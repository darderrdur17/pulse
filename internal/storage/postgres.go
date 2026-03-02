package storage

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/derr/pulse/internal/models"
	_ "github.com/lib/pq"
)

// Store wraps the PostgreSQL connection and exposes typed write methods
type Store struct {
	db *sql.DB
}

// New opens a PostgreSQL connection and pings it
func New(dsn string) (*Store, error) {
	db, err := sql.Open("postgres", dsn)
	if err != nil {
		return nil, fmt.Errorf("opening db: %w", err)
	}
	if err := db.Ping(); err != nil {
		return nil, fmt.Errorf("pinging db: %w", err)
	}
	db.SetMaxOpenConns(20)
	db.SetMaxIdleConns(5)
	return &Store{db: db}, nil
}

// Migrate creates tables and indexes if they do not already exist
func (s *Store) Migrate(ctx context.Context) error {
	ddl := `
	CREATE TABLE IF NOT EXISTS raw_posts (
		id           TEXT PRIMARY KEY,
		source       TEXT        NOT NULL,
		author       TEXT,
		title        TEXT,
		body         TEXT,
		url          TEXT,
		score        INTEGER     DEFAULT 0,
		num_comments INTEGER     DEFAULT 0,
		tags         TEXT[],
		subreddit    TEXT,
		created_at   TIMESTAMPTZ NOT NULL,
		fetched_at   TIMESTAMPTZ NOT NULL
	);
	CREATE INDEX IF NOT EXISTS idx_posts_source      ON raw_posts(source);
	CREATE INDEX IF NOT EXISTS idx_posts_author      ON raw_posts(author);
	CREATE INDEX IF NOT EXISTS idx_posts_created_at  ON raw_posts(created_at DESC);

	CREATE TABLE IF NOT EXISTS user_profiles (
		username         TEXT,
		source           TEXT,
		post_count       INTEGER,
		avg_score        DOUBLE PRECISION,
		total_engagement INTEGER,
		top_keywords     TEXT[],
		active_hours     JSONB,
		top_subreddits   JSONB,
		signals          JSONB,
		first_seen       TIMESTAMPTZ,
		last_seen        TIMESTAMPTZ,
		updated_at       TIMESTAMPTZ,
		PRIMARY KEY (username, source)
	);
	CREATE INDEX IF NOT EXISTS idx_profiles_source   ON user_profiles(source);
	CREATE INDEX IF NOT EXISTS idx_profiles_last_seen ON user_profiles(last_seen DESC);
	`
	_, err := s.db.ExecContext(ctx, ddl)
	return err
}

// UpsertPosts bulk-inserts posts, skipping duplicates
func (s *Store) UpsertPosts(ctx context.Context, posts []models.RawPost) (int, error) {
	if len(posts) == 0 {
		return 0, nil
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()

	stmt, err := tx.PrepareContext(ctx, `
		INSERT INTO raw_posts
			(id, source, author, title, body, url, score, num_comments, tags, subreddit, created_at, fetched_at)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12)
		ON CONFLICT (id) DO UPDATE SET
			score        = EXCLUDED.score,
			num_comments = EXCLUDED.num_comments,
			fetched_at   = EXCLUDED.fetched_at
	`)
	if err != nil {
		return 0, err
	}
	defer stmt.Close()

	count := 0
	for _, p := range posts {
		tags := "{" + strings.Join(p.Tags, ",") + "}"
		_, err := stmt.ExecContext(ctx,
			p.ID, p.Source, p.Author, p.Title, p.Body, p.URL,
			p.Score, p.NumComments, tags, p.Subreddit,
			p.CreatedAt, p.FetchedAt,
		)
		if err != nil {
			return count, fmt.Errorf("upserting post %s: %w", p.ID, err)
		}
		count++
	}

	return count, tx.Commit()
}

// UpsertProfiles writes user profiles using INSERT … ON CONFLICT UPDATE
func (s *Store) UpsertProfiles(ctx context.Context, profiles map[string]*models.UserProfile) (int, error) {
	if len(profiles) == 0 {
		return 0, nil
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()

	stmt, err := tx.PrepareContext(ctx, `
		INSERT INTO user_profiles
			(username, source, post_count, avg_score, total_engagement, top_keywords,
			 active_hours, top_subreddits, signals, first_seen, last_seen, updated_at)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12)
		ON CONFLICT (username, source) DO UPDATE SET
			post_count       = EXCLUDED.post_count,
			avg_score        = EXCLUDED.avg_score,
			total_engagement = EXCLUDED.total_engagement,
			top_keywords     = EXCLUDED.top_keywords,
			active_hours     = EXCLUDED.active_hours,
			top_subreddits   = EXCLUDED.top_subreddits,
			signals          = EXCLUDED.signals,
			last_seen        = EXCLUDED.last_seen,
			updated_at       = EXCLUDED.updated_at
	`)
	if err != nil {
		return 0, err
	}
	defer stmt.Close()

	count := 0
	for _, p := range profiles {
		activeHoursJSON, _ := json.Marshal(p.ActiveHours)
		topSubsJSON, _ := json.Marshal(p.TopSubreddits)
		signalsJSON, _ := json.Marshal(p.Signals)
		keywords := "{" + strings.Join(p.TopKeywords, ",") + "}"

		_, err := stmt.ExecContext(ctx,
			p.Username, p.Source, p.PostCount, p.AvgScore, p.TotalEngagement,
			keywords, activeHoursJSON, topSubsJSON, signalsJSON,
			p.FirstSeen, p.LastSeen, p.UpdatedAt,
		)
		if err != nil {
			return count, fmt.Errorf("upserting profile %s: %w", p.Username, err)
		}
		count++
	}

	return count, tx.Commit()
}

// Close shuts down the database connection pool
func (s *Store) Close() error { return s.db.Close() }
