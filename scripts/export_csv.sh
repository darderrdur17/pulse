#!/usr/bin/env bash
# Export Pulse DB to tidy, analysis-ready CSV. Run from project root with stack up:
#   ./scripts/export_csv.sh
# Output: ./output/raw_posts.csv and ./output/user_profiles.csv (flattened, clean columns)

set -e
OUTDIR="${1:-./output}"
mkdir -p "$OUTDIR"

if ! docker compose exec -T postgres psql -U postgres -d pulse -c "SELECT 1" &>/dev/null; then
  echo "PostgreSQL not reachable. Start the stack first: docker compose up -d"
  exit 1
fi

echo "Exporting tidy CSV to $OUTDIR/ ..."

# Raw posts: clean tags as "a; b; c", body truncated for readability
docker compose exec -T postgres psql -U postgres -d pulse -c "COPY (
  SELECT id, source, COALESCE(author,'') AS author, COALESCE(title,'') AS title, COALESCE(left(body,2000),'') AS body, COALESCE(url,'') AS url, COALESCE(score,0) AS score, COALESCE(num_comments,0) AS num_comments, COALESCE(array_to_string(tags,'; '),'') AS tags, COALESCE(subreddit,'') AS subreddit, to_char(created_at,'YYYY-MM-DD HH24:MI:SS') AS created_at, to_char(fetched_at,'YYYY-MM-DD HH24:MI:SS') AS fetched_at FROM raw_posts ORDER BY fetched_at DESC
) TO STDOUT WITH (FORMAT csv, HEADER true, DELIMITER ',', ENCODING 'UTF8')" > "$OUTDIR/raw_posts.csv"
echo "  raw_posts.csv"

# User profiles: flatten signals into one column each, keywords as "a; b; c"
docker compose exec -T postgres psql -U postgres -d pulse -c "COPY (
  SELECT COALESCE(username,'') AS username, COALESCE(source,'') AS source, COALESCE(post_count,0) AS post_count, round(COALESCE(avg_score,0)::numeric,2) AS avg_score, COALESCE(total_engagement,0) AS total_engagement, COALESCE(array_to_string(top_keywords,'; '),'') AS top_keywords, COALESCE(active_hours::text,'{}') AS active_hours, COALESCE(top_subreddits::text,'{}') AS top_subreddits, round(COALESCE((signals->>'tech_interest')::float,0)::numeric,4) AS tech_interest, round(COALESCE((signals->>'finance_interest')::float,0)::numeric,4) AS finance_interest, round(COALESCE((signals->>'ai_interest')::float,0)::numeric,4) AS ai_interest, round(COALESCE((signals->>'high_influence')::float,0)::numeric,4) AS high_influence, round(COALESCE((signals->>'influence_score')::float,0)::numeric,4) AS influence_score, round(COALESCE((signals->>'activity_consistency')::float,0)::numeric,4) AS activity_consistency, round(COALESCE((signals->>'recency')::float,0)::numeric,4) AS recency, to_char(first_seen,'YYYY-MM-DD HH24:MI:SS') AS first_seen, to_char(last_seen,'YYYY-MM-DD HH24:MI:SS') AS last_seen, to_char(updated_at,'YYYY-MM-DD HH24:MI:SS') AS updated_at FROM user_profiles ORDER BY (signals->>'influence_score')::float DESC NULLS LAST
) TO STDOUT WITH (FORMAT csv, HEADER true, DELIMITER ',', ENCODING 'UTF8')" > "$OUTDIR/user_profiles.csv"
echo "  user_profiles.csv"

echo "Done. Open $OUTDIR/*.csv in Excel or any spreadsheet."
echo "Row counts:"
wc -l "$OUTDIR"/*.csv
