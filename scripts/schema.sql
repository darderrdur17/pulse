-- Pulse — PostgreSQL Schema
-- Run once: psql -U postgres -d pulse -f schema.sql

-- Raw posts from all crawled sources
CREATE TABLE IF NOT EXISTS raw_posts (
    id           TEXT PRIMARY KEY,           -- "<source>_<native_id>"
    source       TEXT        NOT NULL,        -- "hackernews" | "reddit"
    author       TEXT,
    title        TEXT,
    body         TEXT,
    url          TEXT,
    score        INTEGER     DEFAULT 0,
    num_comments INTEGER     DEFAULT 0,
    tags         TEXT[],                      -- extracted keyword tags
    subreddit    TEXT,                        -- reddit only
    created_at   TIMESTAMPTZ NOT NULL,
    fetched_at   TIMESTAMPTZ NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_posts_source     ON raw_posts(source);
CREATE INDEX IF NOT EXISTS idx_posts_author     ON raw_posts(author);
CREATE INDEX IF NOT EXISTS idx_posts_created_at ON raw_posts(created_at DESC);
CREATE INDEX IF NOT EXISTS idx_posts_tags       ON raw_posts USING gin(tags);

-- Aggregated user profiles built from post activity
CREATE TABLE IF NOT EXISTS user_profiles (
    username         TEXT,
    source           TEXT,
    post_count       INTEGER,
    avg_score        DOUBLE PRECISION,
    total_engagement INTEGER,
    top_keywords     TEXT[],
    active_hours     JSONB,                   -- {hour_int: count}
    top_subreddits   JSONB,                   -- {subreddit: count}
    signals          JSONB,                   -- {signal_name: score 0.0–1.0}
    first_seen       TIMESTAMPTZ,
    last_seen        TIMESTAMPTZ,
    updated_at       TIMESTAMPTZ,
    PRIMARY KEY (username, source)
);

CREATE INDEX IF NOT EXISTS idx_profiles_source    ON user_profiles(source);
CREATE INDEX IF NOT EXISTS idx_profiles_last_seen ON user_profiles(last_seen DESC);
-- GIN index enables fast JSONB signal queries, e.g. profiles with high ai_interest
CREATE INDEX IF NOT EXISTS idx_profiles_signals   ON user_profiles USING gin(signals);
