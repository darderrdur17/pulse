# Pulse

A concurrent social media intelligence pipeline written in **Go**. Crawls public data from HackerNews and Reddit, builds structured user profiles, and extracts scored intelligence signals for downstream analytics.

## Architecture

```
┌─────────────────────────────────────────────────────────────┐
│                      Pulse Cycle (15 min)                    │
├──────────────┬──────────────────────┬───────────────────────┤
│   Crawlers   │     Processor        │       Storage          │
│              │                      │                        │
│  HackerNews ─┤─► Keyword Extraction │  PostgreSQL            │
│  (goroutines)│─► Engagement Scoring │  ├── raw_posts         │
│              │─► User Profiling     │  └── user_profiles     │
│  Reddit      │─► Signal Extraction  │       (JSONB signals)  │
│  (goroutines)│                      │                        │
└──────────────┴──────────────────────┴───────────────────────┘
```

## Key Go Features Used

| Feature | Where |
|---|---|
| **Goroutines + WaitGroups** | Concurrent story/subreddit fetching |
| **Channels (fan-out/fan-in)** | Worker pool pattern in crawlers |
| **`golang.org/x/time/rate`** | Per-source rate limiting |
| **Exponential backoff retry** | All HTTP calls |
| **`context.Context`** | Graceful shutdown via OS signals |
| **`database/sql` + `lib/pq`** | Bulk upserts, connection pooling |
| **Structured logging (zap)** | Production-ready observability |

## Project Structure

```
pulse/
├── cmd/pulse/              # Main entrypoint
├── config/                 # Environment-based config
│   └── topic_keywords.json # Optional: custom keywords for signals (edit to add your own)
├── internal/
│   ├── crawler/        # HackerNews + Reddit crawlers
│   ├── models/         # Shared data schemas
│   ├── processor/      # Keyword extraction + user profiling
│   ├── signals/        # Intelligence signal computation
│   └── storage/        # PostgreSQL storage layer
└── scripts/
    └── schema.sql      # DB schema with indexes
```

## How to run

**Recommended (Docker):** from the project root:

```bash
docker compose up --build
```

This starts PostgreSQL and the Pulse pipeline. The pipeline runs a full crawl every 15 minutes. Stop with `Ctrl+C` or run in the background with `docker compose up -d`.

**Prerequisites:** [Docker](https://docs.docker.com/get-docker/) (Docker Desktop or Engine + Compose). No API keys required — HackerNews and Reddit use public endpoints.

---

## Quickstart (alternatives)

### Prerequisites
- Go 1.22+ (for local run)
- PostgreSQL 14+ (or Docker)

### Run with Docker
```bash
docker compose up --build
# or: docker-compose up --build
```

### Run locally
```bash
# 1. Start PostgreSQL (e.g. Docker: docker run -d -p 5432:5432 -e POSTGRES_PASSWORD=postgres -e POSTGRES_DB=pulse postgres:16-alpine)
createdb pulse
psql -d pulse -f scripts/schema.sql

# 2. Set environment (or create .env)
export DATABASE_URL="postgres://localhost:5432/pulse?sslmode=disable"

# 3. Run
go run ./cmd/pulse
```

---

## Results (sample run)

A typical cycle on a fresh run:

| Step | Result |
|------|--------|
| **HackerNews** | 50 top stories fetched (concurrent workers) |
| **Reddit** | 25 hot posts × 5 subreddits = 125 posts (`golang`, `programming`, `datascience`, `MachineLearning`, `finance`) |
| **Total posts** | 175 stored (with keyword tags and engagement scores) |
| **User profiles** | 140 unique users aggregated and scored |
| **Cycle time** | ~10–15 seconds per cycle |
| **Schedule** | New cycle every 15 minutes |

Example log output:

```
{"level":"info","msg":"pipeline starting","concurrency":10,"rps":5}
{"level":"info","msg":"database ready"}
{"level":"info","msg":"starting crawl cycle"}
{"level":"info","msg":"HN crawl complete","fetched":50}
{"level":"info","msg":"fetched subreddit","sub":"golang","count":25}
...
{"level":"info","msg":"crawl complete","total_posts":175}
{"level":"info","msg":"posts stored","count":175}
{"level":"info","msg":"profiles built","users":140}
{"level":"info","msg":"profiles stored","count":140}
{"level":"info","msg":"cycle finished","elapsed":11.2...}
{"level":"info","msg":"cycle complete — sleeping 15 minutes"}
```

---

## What’s the end result?

The pipeline writes everything into **PostgreSQL**. The “end result” is two tables:

| Table | Contents |
|-------|----------|
| **`raw_posts`** | Crawled posts (HN + Reddit): title, author, score, comments, tags, source, timestamps |
| **`user_profiles`** | One row per user: post count, engagement, top keywords, **`signals`** (JSONB with 7 scores) |

### How to view it

**If you run with Docker:** after at least one cycle has finished, connect to the DB from your machine:

```bash
# With stack running: docker compose up -d
docker compose exec postgres psql -U postgres -d pulse -c "
  SELECT 'raw_posts' AS table_name, count(*) FROM raw_posts
  UNION ALL
  SELECT 'user_profiles', count(*) FROM user_profiles;
"
```

**Sample queries (run inside `psql -U postgres -d pulse` or via `docker compose exec postgres psql -U postgres -d pulse`):**

```sql
-- Counts
SELECT source, count(*) FROM raw_posts GROUP BY source;
SELECT count(*) AS total_profiles FROM user_profiles;

-- Recent posts (title + author + score)
SELECT source, author, left(title, 60) AS title, score
FROM raw_posts ORDER BY fetched_at DESC LIMIT 10;

-- Top users by AI-interest signal (the “intelligence” output)
SELECT username, source,
       (signals->>'ai_interest')::float AS ai_score,
       (signals->>'tech_interest')::float AS tech_score,
       post_count
FROM user_profiles
WHERE (signals->>'ai_interest')::float > 0.2
ORDER BY (signals->>'ai_interest')::float DESC
LIMIT 15;
```

So the **end result** you can show or use: **queryable tables** of crawled posts and user profiles with signal scores (tech, finance, AI, influence, etc.) for analytics, dashboards, or downstream tools.

### Export to CSV (tidy, analysis-ready)

Each run of the export script produces **cleaned datasets** suitable for Excel or other tools:

1. Start the stack and wait for at least one cycle: `docker compose up -d`
2. From the **project root**, run:

```bash
./scripts/export_csv.sh
```

This creates:

- **`output/raw_posts.csv`** — One row per post. Columns: `id`, `source`, `author`, `title`, `body` (truncated to 2000 chars), `url`, `score`, `num_comments`, `tags` (semicolon-separated, e.g. `ape; coding; fiction`), `subreddit`, `created_at`, `fetched_at`. Sorted by `fetched_at` descending. Nulls and arrays are normalized so the CSV is tidy.
- **`output/user_profiles.csv`** — One row per user. Columns: `username`, `source`, `post_count`, `avg_score`, `total_engagement`, `top_keywords` (semicolon-separated), `active_hours`, `top_subreddits`, then **one column per signal**: `tech_interest`, `finance_interest`, `ai_interest`, `high_influence`, `influence_score`, `activity_consistency`, `recency` (numeric 0–1), plus `first_seen`, `last_seen`, `updated_at`. Sorted by influence descending.

So every time you run the script, you get **tidier, consistent CSVs**: flattened signals, readable tag lists, rounded numbers, and stable date formatting. Open `output/*.csv` in Excel or Google Sheets. Optional: `./scripts/export_csv.sh ./my_folder` to write to another folder.

### What the CSV results mean

**`raw_posts.csv`** — One row per **post** (story or thread) that was crawled:

| Column | Meaning | Example |
|--------|--------|--------|
| `id` | Unique ID (`hn_123` or `reddit_abc`) | `hn_47206798` |
| `source` | Where it came from | `hackernews` or `reddit` |
| `author` | Username who posted | `rmsaksida`, `ketralnis` |
| `title` | Post title | "Ape Coding [fiction]" |
| `body` | Post text (often empty for link posts) | Long text or empty |
| `url` | Link URL | `https://rsaksida.com/blog/ape-coding/` |
| `score` | Upvotes / points | `164` |
| `num_comments` | Number of comments | `109` |
| `tags` | Top keywords extracted from title+body | `{ape,coding,fiction}` |
| `subreddit` | Reddit only: which subreddit | `programming` or empty for HN |
| `created_at` | When the post was created (UTC) | `2026-03-01 14:07:05+00` |
| `fetched_at` | When Pulse stored it (UTC) | `2026-03-02 06:54:04+00` |

So this file is the **raw feed**: every Hacker News and Reddit post the pipeline collected, with engagement (score, comments) and auto-extracted tags.

---

**`user_profiles.csv`** — One row per **user** (author), aggregated from all their posts in this run:

| Column | Meaning | Example |
|--------|--------|--------|
| `username` | Author name | `mschnell`, `golangparis` |
| `source` | Platform | `hackernews` or `reddit` |
| `post_count` | How many of their posts were in this crawl | `1`, `2`, … |
| `avg_score` | Average engagement (score + 2×comments) per post | `605` |
| `total_engagement` | Sum of engagement across posts | `605` |
| `top_keywords` | Merged top keywords from their posts | `{decision,trees,nested,rules}` |
| `active_hours` | When they post (hour of day → count) | `{"8": 1}` = 1 post at 8:00 |
| `top_subreddits` | Reddit only: which subreddits they posted in | `{"golang": 1}` |
| `signals` | **Intelligence scores (0–1)** — see below | JSON with 7 scores |
| `first_seen` / `last_seen` | First and last post time in this data | UTC timestamps |
| `updated_at` | When the profile was last updated | UTC |

The **`signals`** column is a JSON object with seven scores (each 0.0 to 1.0):

- **`tech_interest`** — How much their keywords match tech (Go, Python, API, cloud, …)
- **`finance_interest`** — Match to finance/crypto/trading terms
- **`ai_interest`** — Match to AI/ML/LLM terms
- **`high_influence`** — Match to “launch”, “release”, “announce”, etc.
- **`influence_score`** — Normalized engagement (high = lots of upvotes/comments per post)
- **`activity_consistency`** — How spread out their posting hours are (entropy)
- **`recency`** — How recently they posted (decays over ~30 days)

So the **result** of the project in CSV form is:

1. **`raw_posts.csv`** — The crawled content (what was posted, by whom, how it performed).
2. **`user_profiles.csv`** — Aggregated **user intelligence**: who posts where, what they talk about, and the seven signal scores you can use to e.g. find “high AI interest” or “high influence” users.

**Tidy CSV format:** Each export run produces cleaned datasets: `tags` and `top_keywords` are semicolon-separated, the seven signals are **separate columns** in `user_profiles.csv`, dates are formatted consistently, and `body` is truncated to 2000 characters.

---

## Custom topic keywords (find/use your own)

The signal scores **tech_interest**, **finance_interest**, **ai_interest**, and **high_influence** are computed by matching user keywords against **topic keyword sets**. You can change or extend these.

**File:** `config/topic_keywords.json` — edit to add/remove keywords. Each key is a signal name; each value is a list of lowercase keywords. To **add a new signal**, add a new key (e.g. `"security_interest": ["vulnerability", "patch", "encryption"]`). If the file is missing or invalid, built-in defaults are used. Restart the pipeline after changes (e.g. `docker compose up -d --build`). So: **to find or use other keywords**, edit that file and run the pipeline again; the next CSV export will reflect the new scores.

---

## Signals Extracted

Each user profile is scored across seven signal dimensions:

| Signal | Description |
|---|---|
| `tech_interest` | Keyword affinity with Go, Python, cloud, APIs etc. |
| `finance_interest` | Affinity with trading, crypto, investment terms |
| `ai_interest` | Affinity with LLMs, ML, neural network vocabulary |
| `high_influence` | Association with launch/release/announcement posts |
| `influence_score` | Normalised engagement-per-post metric |
| `activity_consistency` | Shannon entropy of posting hour distribution |
| `recency` | Exponential decay score based on last-seen date |

### Example: Query high-AI-interest users
```sql
SELECT username, source,
       (signals->>'ai_interest')::float AS ai_score,
       (signals->>'influence_score')::float AS influence
FROM user_profiles
WHERE (signals->>'ai_interest')::float > 0.5
ORDER BY ai_score DESC
LIMIT 20;
```

## Configuration

| Variable | Default | Description |
|---|---|---|
| `DATABASE_URL` | `postgres://localhost/pulse` | PostgreSQL DSN |
| `MAX_CONCURRENCY` | `10` | Worker goroutines per crawler |
| `REQUESTS_PER_SEC` | `5` | Rate limit (req/s per source) |
| `MAX_RETRIES` | `3` | Max retries with exponential backoff |

## Extending Pulse

**Add a new data source:** implement a crawler in `internal/crawler/` that returns `[]models.RawPost` — the processor and storage layers require no changes.

**Add a new signal:** add a new key and keyword list to `config/topic_keywords.json` (see [Custom topic keywords](#custom-topic-keywords-finduse-your-own)) — it will be scored and stored in the JSONB `signals` column and in the CSV export. Alternatively, edit `defaultTopicKeywords` in `internal/signals/extractor.go`.

**Connect to Kafka/Flink:** replace the `store.UpsertPosts` call in `main.go` with a Kafka producer; the normalised `RawPost` schema maps cleanly to an Avro/JSON topic.
