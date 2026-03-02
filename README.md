# Pulse

A **concurrent social media intelligence pipeline** written in **Go**. Pulse crawls public data from Hacker News and Reddit, enriches each post with keyword extraction and engagement metrics, aggregates activity into **user profiles**, and scores each user on seven **intelligence signals** (e.g. tech interest, AI interest, influence). Results are stored in PostgreSQL and can be exported as tidy CSVs for analysis, dashboards, or portfolio demos.

**Use cases:** identify high-signal users by topic (e.g. “who talks about AI?”), measure engagement and influence, feed downstream analytics or ML, or demonstrate concurrent Go, APIs, and data pipelines.

**In short:** run `docker compose up --build`, wait for a cycle, then query Postgres or run `./scripts/export_csv.sh` to get **raw_posts** (crawled content and tags) and **user_profiles** (one row per user with seven 0–1 signal scores). See [Project results](#project-results--what-the-pipeline-produces) for a detailed explanation of the outputs and how to interpret them.

---

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

**Data flow (each 15‑minute cycle):**

1. **Crawl** — Fetch top 50 Hacker News stories and 25 hot posts from each of 5 subreddits (`golang`, `programming`, `datascience`, `MachineLearning`, `finance`) using concurrent workers and rate limiting.
2. **Enrich** — Extract top keywords from each post’s title and body (stopwords removed), and compute engagement (score + 2× comments).
3. **Store posts** — Upsert into `raw_posts` (id, source, author, title, body, url, score, num_comments, tags, subreddit, timestamps).
4. **Build profiles** — Group posts by author and source; aggregate post count, engagement, top keywords, active hours, and subreddit distribution per user.
5. **Compute signals** — Score each profile on seven dimensions (topic affinity from keywords, influence from engagement, activity spread, recency).
6. **Store profiles** — Upsert into `user_profiles` (username, source, metrics, and a JSONB `signals` object with all seven scores).

---

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
├── docs/
│   ├── KAFKA.md            # Step-by-step Kafka guide (beginning to end)
│   └── VERIFY.md           # Checklist: verify project + Kafka (Postgres, CSV, consume)
├── internal/
│   ├── crawler/        # HackerNews + Reddit crawlers
│   ├── kafka/          # Optional Kafka producer (enriched posts as JSON)
│   ├── models/         # Shared data schemas
│   ├── processor/      # Keyword extraction + user profiling
│   ├── signals/        # Intelligence signal computation
│   └── storage/        # PostgreSQL storage layer
└── scripts/
    ├── schema.sql      # DB schema with indexes
    └── export_csv.sh   # Export tidy raw_posts + user_profiles to output/*.csv
```

## How to run

**Recommended (Docker):** from the project root:

```bash
docker compose up --build
```

This starts PostgreSQL and the Pulse pipeline. The pipeline runs a full crawl every 15 minutes. Stop with `Ctrl+C` or run in the background with `docker compose up -d`.

**Prerequisites:** [Docker](https://docs.docker.com/get-docker/) (Docker Desktop or Engine + Compose). No API keys required — HackerNews and Reddit use public endpoints.

---

## GitHub

**Repository:** [github.com/derr/pulse](https://github.com/derr/pulse) (replace `derr` with your username if you forked or cloned from your own repo)

**Clone and run:**

```bash
git clone https://github.com/derr/pulse.git
cd pulse
docker compose up --build
```

After the first cycle (~15 seconds), export CSVs: `./scripts/export_csv.sh` — outputs are in `output/raw_posts.csv` and `output/user_profiles.csv`. See [Project results](#project-results--what-the-pipeline-produces) for what the data means.

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

## Project results — what the pipeline produces

Pulse produces **two main outputs**: a table of **crawled posts** and a table of **user profiles with signal scores**. Below is what each is, what the numbers mean, and how to interpret them.

### 1. Raw posts (`raw_posts` / `raw_posts.csv`)

**What it is:** One row per story or thread that was crawled from Hacker News or Reddit.

| What you get | Description |
|--------------|--------------|
| **Volume per cycle** | ~50 HN stories + ~125 Reddit posts (25 × 5 subreddits) → **~175 posts** per run. Over time, rows accumulate (upsert by id), so total count grows. |
| **Fields** | `id`, `source` (hackernews | reddit), `author`, `title`, `body`, `url`, `score`, `num_comments`, `tags` (auto-extracted keywords), `subreddit` (Reddit only), `created_at`, `fetched_at`. |
| **What it’s for** | See exactly what was posted, by whom, how it performed (score, comments), and what topics appear (tags). Use for content analysis, trend checks, or feeding other tools. |

**Interpretation:** Higher `score` and `num_comments` mean more engagement. `tags` are the main repeated terms from title+body (stopwords removed) and reflect the post’s topic in a simple way.

---

### 2. User profiles with signals (`user_profiles` / `user_profiles.csv`)

**What it is:** One row per **user** (author), with aggregated activity and **seven signal scores** (0–1) that describe their interests and behavior.

| What you get | Description |
|--------------|--------------|
| **Volume per cycle** | ~140–150 unique users per run (combined HN + Reddit). Profiles are upserted by (username, source), so data accumulates and updates over time. |
| **Aggregated fields** | `post_count`, `avg_score`, `total_engagement`, `top_keywords` (merged from their posts), `active_hours` (when they post), `top_subreddits` (Reddit only). |
| **Signal columns** | Seven numeric scores (0–1): `tech_interest`, `finance_interest`, `ai_interest`, `high_influence`, `influence_score`, `activity_consistency`, `recency`. In the DB they live in a JSONB `signals` object; in the CSV export they are **separate columns** for easy filtering and sorting. |

**What each signal means and how to interpret it:**

| Signal | How it’s computed | How to interpret |
|--------|-------------------|------------------|
| **tech_interest** | Overlap between user’s top keywords and a tech keyword set (e.g. golang, python, api, cloud). | **Higher (e.g. >0.3)** → user’s posts often mention tech terms. Good for finding “tech-oriented” users. |
| **finance_interest** | Overlap with finance/crypto/trading keywords. | **Higher** → user talks about markets, crypto, investing. |
| **ai_interest** | Overlap with AI/ML/LLM keywords. | **Higher** → user discusses AI, ML, models, etc. Use to find “AI-interested” users. |
| **high_influence** | Overlap with “launch”, “release”, “announce”, “show”, etc. | **Higher** → user tends to post announcement-style content. |
| **influence_score** | Normalized engagement (avg engagement per post, capped). | **Higher** → posts get more upvotes/comments on average. Pure reach/engagement. |
| **activity_consistency** | Entropy of posting hour distribution. | **Higher** → posts are spread across many hours (consistent); **lower** → posts cluster in few hours. |
| **recency** | Decay since last seen (≈ 30-day half-life). | **Higher** → user posted recently; **lower** → inactive for a while. |

**Example interpretations:**

- **High `ai_interest` + high `influence_score`** → user who talks about AI and gets strong engagement; good candidate for “influential AI” lists.
- **High `tech_interest` + high `recency`** → active tech-oriented user.
- **Low `recency`** → user hasn’t appeared in the crawl recently; profile may be stale.

So the **project results** are: (1) a **post-level dataset** (what was said, by whom, how it performed), and (2) a **user-level dataset** with **seven interpretable scores** you can use to segment and analyze audiences (e.g. for portfolios, demos, or downstream analytics).

---

### 3. Typical run (numbers and logs)

A single cycle usually looks like this:

| Step | Result |
|------|--------|
| **Hacker News** | 50 top stories fetched (concurrent workers) |
| **Reddit** | 25 hot posts × 5 subreddits = 125 posts (`golang`, `programming`, `datascience`, `MachineLearning`, `finance`) |
| **Total posts stored** | ~175 (with keyword tags and engagement) |
| **User profiles built** | ~140 unique users, each with 7 signal scores |
| **Cycle time** | ~10–15 seconds |
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

**Results summary:** Pulse gives you (1) **post-level data** — what was posted, by whom, engagement, and tags — and (2) **user-level data** — one row per author with seven **interpretable scores** (tech/finance/AI interest, influence, activity pattern, recency). Use the CSVs or SQL to filter and sort (e.g. “users with high AI interest and high influence”) for analytics or demos.

---

## What’s the end result?

The pipeline writes everything into **PostgreSQL**. The “end result” is two tables:

| Table | Contents |
|-------|----------|
| **`raw_posts`** | Crawled posts (HN + Reddit): title, author, score, comments, tags, source, timestamps |
| **`user_profiles`** | One row per user: post count, engagement, top keywords, **`signals`** (JSONB with 7 scores) |

**Full explanation of the two tables and how to interpret the seven signals:** see [Project results](#project-results--what-the-pipeline-produces) above.

### View in the database

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
| `tags` | Top keywords extracted from title+body (semicolon-separated in CSV) | `ape; coding; fiction` |
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
| `top_keywords` | Merged top keywords from their posts (semicolon-separated in CSV) | `decision; trees; nested; rules` |
| `active_hours` | When they post (hour of day → count) | `{"8": 1}` = 1 post at 8:00 |
| `top_subreddits` | Reddit only: which subreddits they posted in | `{"golang": 1}` |
| **Signal columns** | In **CSV export**, the seven scores are **separate columns**: `tech_interest`, `finance_interest`, `ai_interest`, `high_influence`, `influence_score`, `activity_consistency`, `recency` (each 0–1). In the DB they are stored as JSONB in `signals`. | e.g. `0.25`, `0.00`, `0.10` |
| `first_seen` / `last_seen` | First and last post time in this data | UTC timestamps |
| `updated_at` | When the profile was last updated | UTC |

The **seven signal columns** (or the `signals` JSON in the DB) contain:

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
| `KAFKA_BROKERS` | *(empty)* | If set, enriched posts are also produced to Kafka (comma-separated brokers, e.g. `localhost:9092`) |
| `KAFKA_TOPIC` | `pulse.posts` | Kafka topic for post messages |

---

## Kafka (optional)

When **`KAFKA_BROKERS`** is set, Pulse produces each **enriched post** (after keyword extraction) to Kafka in addition to writing to PostgreSQL. Messages are JSON-serialised `RawPost` records; the message **key** is the post `id` (e.g. `hn_123`, `reddit_abc`).

**Full guide (beginning to end):** [docs/KAFKA.md](docs/KAFKA.md) — start Kafka, run Pulse with Kafka, and consume messages step by step.

**Quick start with Kafka (Postgres + Kafka + Pulse in one command):**
```bash
docker compose -f docker-compose.yml -f docker-compose.kafka.yml up --build
```

**Verify project and Kafka:** [docs/VERIFY.md](docs/VERIFY.md) — checklist to confirm Postgres, CSV export, and Kafka produce/consume all work.

**Environment variables:**

| Variable | Example | Description |
|----------|---------|-------------|
| `KAFKA_BROKERS` | `localhost:9092` or `kafka1:9092,kafka2:9092` | One or more broker addresses (comma-separated). If empty, Kafka is disabled. |
| `KAFKA_TOPIC` | `pulse.posts` | Topic name (default `pulse.posts`). |

**Run with Kafka (Docker):**

1. Start Kafka (e.g. with Docker). Example one-liner:
   ```bash
   docker run -d --name kafka -p 9092:9092 -e KAFKA_CFG_NODE_ID=0 -e KAFKA_CFG_PROCESS_ROLES=controller,broker -e KAFKA_CFG_LISTENERS=PLAINTEXT://:9092 apache/kafka:3.7.0
   ```
2. Start Pulse with Kafka env set:
   ```bash
   export KAFKA_BROKERS=localhost:9092
   docker compose up --build
   ```
   Or add to `docker-compose.yml` under `pipeline`:
   ```yaml
   environment:
     KAFKA_BROKERS: kafka:9092   # if Kafka is a service in the same compose
     KAFKA_TOPIC: pulse.posts
   ```

**Message format:** Each Kafka message is a JSON object with the same shape as `RawPost`: `id`, `source`, `author`, `title`, `body`, `url`, `score`, `num_comments`, `tags` (array), `subreddit`, `created_at`, `fetched_at`. You can consume with Kafka Connect, Flink, or any consumer and map the payload to Avro/JSON as needed.

---

## Extending Pulse

**Add a new data source:** implement a crawler in `internal/crawler/` that returns `[]models.RawPost` — the processor and storage layers require no changes.

**Add a new signal:** add a new key and keyword list to `config/topic_keywords.json` (see [Custom topic keywords](#custom-topic-keywords-finduse-your-own)) — it will be scored and stored in the JSONB `signals` column and in the CSV export. Alternatively, edit `defaultTopicKeywords` in `internal/signals/extractor.go`.

**Kafka:** set `KAFKA_BROKERS` (see [Kafka (optional)](#kafka-optional)); the pipeline will produce enriched posts to the configured topic in addition to storing in PostgreSQL.
