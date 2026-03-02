# Verify Pulse and Kafka

Use this checklist to confirm the project and Kafka are working end-to-end.

---

## 1. Start the stack (with Kafka)

From the **project root**:

```bash
docker compose -f docker-compose.yml -f docker-compose.kafka.yml up --build
```

Wait until you see:

- `pulse-kafka` — "Kafka Server started" / "Awaiting socket connections on kafka:9092"
- `kafka-init` — runs once and exits (creates topic `pulse.posts`)
- `pipeline-1` — "database ready", "kafka enabled", then after ~15–20 s: "crawl complete", "**kafka produced**", "posts stored", "profiles stored", "cycle complete — sleeping 15 minutes"

If you see **"kafka produced","count":175** (or similar) with **no** "kafka produce" error, Kafka is working.

---

## 2. Verify PostgreSQL (project output)

With the stack still running, in another terminal:

```bash
# Row counts
docker compose -f docker-compose.yml -f docker-compose.kafka.yml exec postgres \
  psql -U postgres -d pulse -t -c \
  "SELECT 'raw_posts: ' || count(*) FROM raw_posts UNION ALL SELECT 'user_profiles: ' || count(*) FROM user_profiles;"
```

You should see non-zero counts (e.g. `raw_posts: 175`, `user_profiles: 140` after one cycle; numbers grow over time).

---

## 3. Verify CSV export (project output)

In the same or another terminal, from the **project root**:

```bash
./scripts/export_csv.sh
```

Check:

```bash
wc -l output/raw_posts.csv output/user_profiles.csv
head -1 output/raw_posts.csv output/user_profiles.csv
```

You should see:

- `output/raw_posts.csv` — header row + many data rows (id, source, author, title, …)
- `output/user_profiles.csv` — header row + many data rows (username, source, and columns like tech_interest, ai_interest, …)

Opening the CSVs in Excel or a text editor confirms the project output is correct.

---

## 4. Verify Kafka (produce + consume)

**4a. Confirm the pipeline is producing**

In the pipeline logs you should have seen (in step 1):

```text
{"level":"info","msg":"kafka produced","count":175}
```

No `"level":"error","msg":"kafka produce"` means produce is OK.

**4b. Consume messages from the topic**

In a **new terminal**, run a console consumer (use your project’s network name if different from `pulse_default`):

```bash
docker network ls | grep pulse
docker run -it --rm --network pulse_default \
  confluentinc/cp-kafka:7.6.9 \
  kafka-console-consumer \
  --bootstrap-server kafka:9092 \
  --topic pulse.posts \
  --from-beginning \
  --max-messages 3
```

You should see **3 lines** of JSON (each line is one enriched post). Then the consumer exits. If you see JSON with fields like `"id"`, `"source"`, `"author"`, `"title"`, Kafka produce and consume are working.

**Consume all messages in the topic:**  
From project root run:

```bash
./scripts/consume_kafka.sh
```

This gets the topic’s actual message count and consumes that many (no timeout exit); it does not require exporting CSV first.

**4c. Optional — list topic and count messages**

```bash
# List topics (should include pulse.posts)
docker run -it --rm --network pulse_default confluentinc/cp-kafka:7.6.9 \
  kafka-topics --bootstrap-server kafka:9092 --list

# Describe topic
docker run -it --rm --network pulse_default confluentinc/cp-kafka:7.6.9 \
  kafka-topics --bootstrap-server kafka:9092 --describe --topic pulse.posts
```

---

## 5. Quick reference

| Check              | Command / where to look |
|--------------------|-------------------------|
| Stack + Kafka up   | `docker compose -f docker-compose.yml -f docker-compose.kafka.yml up --build` → see "kafka produced" in logs |
| Postgres data      | `docker compose ... exec postgres psql -U postgres -d pulse -c "SELECT count(*) FROM raw_posts; SELECT count(*) FROM user_profiles;"` |
| CSV export         | `./scripts/export_csv.sh` then inspect `output/*.csv` |
| Kafka consume      | `./scripts/consume_kafka.sh` (uses CSV row count) or `docker run ... kafka-console-consumer ... --max-messages N` |

---

## 6. Without Kafka (Postgres + pipeline only)

To confirm the **project** works without Kafka:

```bash
docker compose up --build
```

Then:

- Wait for "cycle complete — sleeping 15 minutes".
- Run `./scripts/export_csv.sh` and check `output/*.csv`.
- Query Postgres as in step 2.

No `KAFKA_BROKERS` is set, so the pipeline only uses Postgres; Kafka is unused. This verifies the core pipeline and CSV export.

---

Once steps 1–4 pass, both the **project** (crawl → Postgres → CSV) and **Kafka** (produce to `pulse.posts` and consume) are verified.
