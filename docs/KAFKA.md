# Kafka with Pulse — Beginning to End

This guide takes you from zero to producing and consuming Pulse posts in Kafka.

---

## 1. What you’re doing

- **Pulse** crawls Hacker News and Reddit, enriches each post (keywords, etc.), and stores results in **PostgreSQL**.
- If you set **`KAFKA_BROKERS`**, Pulse **also** sends each enriched post to **Kafka** as a JSON message (same data as in the DB).
- You can then consume those messages with any Kafka consumer (Flink, Kafka Connect, custom app, or the CLI).

**Flow:** Crawl → Enrich → **[Produce to Kafka]** + Store in Postgres → Build profiles → Store profiles.

---

## 2. Prerequisites

- **Docker** (and Docker Compose) on your machine.
- The Pulse repo (you’re in it).

No need to install Kafka on your host; we’ll run it in Docker.

---

## 3. Option A — All-in-one: Postgres + Kafka + Pulse (recommended)

One command starts Postgres, Kafka, and Pulse with Kafka enabled.

From the **project root**:

```bash
docker compose -f docker-compose.yml -f docker-compose.kafka.yml up --build
```

- **Postgres** — port `5432`
- **Kafka** — port `9092` (broker)
- **Pulse** — runs and produces to topic `pulse.posts` every 15 minutes

Wait for the first cycle (~15–20 seconds). Logs will show something like:

```text
pipeline-1  | {"level":"info","msg":"kafka enabled","brokers":"kafka:9092","topic":"pulse.posts"}
...
pipeline-1  | {"level":"info","msg":"kafka produced","count":175}
```

Leave it running (or run in background with `-d`). To **consume messages**, open another terminal and go to step 5.

---

## 4. Option B — Run Kafka and Pulse separately

Use this if you prefer to start Kafka yourself or already have a broker.

### Step 4.1 — Start Kafka (Docker)

Single-node Kafka in **KRaft mode** (no Zookeeper). Use the **Confluent** image (same as the compose file):

```bash
docker run -d --name pulse-kafka -p 9092:9092 -p 29093:29093 \
  -h kafka \
  -e KAFKA_NODE_ID=1 \
  -e KAFKA_PROCESS_ROLES=broker,controller \
  -e KAFKA_CONTROLLER_QUORUM_VOTERS=1@kafka:29093 \
  -e KAFKA_LISTENERS=PLAINTEXT://kafka:9092,CONTROLLER://kafka:29093 \
  -e KAFKA_ADVERTISED_LISTENERS=PLAINTEXT://localhost:9092 \
  -e KAFKA_CONTROLLER_LISTENER_NAMES=CONTROLLER \
  -e KAFKA_LISTENER_SECURITY_PROTOCOL_MAP=CONTROLLER:PLAINTEXT,PLAINTEXT:PLAINTEXT \
  -e KAFKA_INTER_BROKER_LISTENER_NAME=PLAINTEXT \
  -e KAFKA_OFFSETS_TOPIC_REPLICATION_FACTOR=1 \
  -e CLUSTER_ID=MkL3xD0ZQo2RDPQVN5nAgg \
  confluentinc/cp-kafka:7.6.9
```

Then run Pulse with `KAFKA_BROKERS=localhost:9092` (from the host, the broker is on 9092).

Check it’s running:

```bash
docker ps | grep pulse-kafka
```

### Step 4.2 — Start Pulse with Kafka

From the **project root**:

```bash
export KAFKA_BROKERS=localhost:9092
export KAFKA_TOPIC=pulse.posts
docker compose up --build
```

Or create a `.env` file in the project root:

```env
KAFKA_BROKERS=localhost:9092
KAFKA_TOPIC=pulse.posts
```

Then run:

```bash
docker compose up --build
```

Pulse will log `kafka enabled` and, after each cycle, `kafka produced` with the message count.

---

## 5. Consume messages (verify end-to-end)

You need a consumer that connects to the **same broker** the producer uses:

- **Option A (compose):** broker is `localhost:9092` from your machine (port is published).
- **Option B (standalone Kafka):** broker is `localhost:9092`.

### 5.1 — Console consumer (Option A — compose)

When you used **Option A**, Kafka and Pulse share a Docker network. Run the consumer on that network so it can reach `kafka:9092`:

```bash
# Find the network name (often pulse_default)
docker network ls | grep pulse

# Run consumer (replace pulse_default with your network name if different)
docker run -it --rm --network pulse_default \
  confluentinc/cp-kafka:7.6.9 \
  kafka-console-consumer \
  --bootstrap-server kafka:9092 \
  --topic pulse.posts \
  --from-beginning
```

You should see one JSON object per line (each line is one enriched post). Stop with `Ctrl+C`.

**Consume all messages in the topic:**  
From project root:

```bash
./scripts/consume_kafka.sh
```

The script gets the topic’s message count and consumes that many, so it always exits cleanly (no timeout). Same Docker network and topic as above.

### 5.2 — Console consumer (Option B — Kafka on host port, or from your machine)

When Kafka is reachable at **localhost:9092** (e.g. you started Kafka with the standalone `docker run` in step 4.1, or you’re on Linux with `--network host`):

**On Linux (with `--network host`):**
```bash
docker run -it --rm --network host \
  confluentinc/cp-kafka:7.6.9 \
  kafka-console-consumer \
  --bootstrap-server localhost:9092 \
  --topic pulse.posts \
  --from-beginning
```

**On Mac/Windows (Kafka port 9092 is published to localhost):**  
Use a consumer that runs in a container but talks to the host. One way is to run the consumer in the same Docker network as Kafka and use the host’s published port. Simpler: run the consumer with `--add-host=host.docker.internal:host-gateway` and bootstrap server `host.docker.internal:9092`:

```bash
docker run -it --rm --add-host=host.docker.internal:host-gateway \
  confluentinc/cp-kafka:7.6.9 \
  kafka-console-consumer \
  --bootstrap-server host.docker.internal:9092 \
  --topic pulse.posts \
  --from-beginning
```

If you used **Option A**, prefer 5.1 (same network); then `localhost:9092` from your host also works because the compose file publishes Kafka’s port 9092.

---

## 6. What each message looks like

- **Topic:** `pulse.posts` (or whatever you set in `KAFKA_TOPIC`).
- **Key:** Post ID (e.g. `hn_47206798`, `reddit_1ri9ty3`).
- **Value:** JSON object with the same shape as Pulse’s `RawPost`:

```json
{
  "id": "hn_47206798",
  "source": "hackernews",
  "author": "rmsaksida",
  "title": "Ape Coding [fiction]",
  "body": "",
  "url": "https://...",
  "score": 164,
  "num_comments": 109,
  "tags": ["ape", "coding", "fiction"],
  "subreddit": "",
  "created_at": "2026-03-01T14:07:05Z",
  "fetched_at": "2026-03-02T06:54:04Z"
}
```

You can plug this into Kafka Connect, Flink, or your own consumer and map it to Avro/JSON/DB as needed.

---

## 7. Quick reference

| Goal                         | Command / setting |
|-----------------------------|--------------------|
| Run Postgres + Kafka + Pulse | `docker compose -f docker-compose.yml -f docker-compose.kafka.yml up --build` |
| Run only Pulse with existing Kafka | `KAFKA_BROKERS=localhost:9092 docker compose up --build` |
| Topic name                  | `pulse.posts` (override with `KAFKA_TOPIC`) |
| Consume on host             | `kafka-console-consumer.sh --bootstrap-server localhost:9092 --topic pulse.posts --from-beginning` |
| Disable Kafka               | Do not set `KAFKA_BROKERS` (or unset it). Pulse only uses Postgres. |

---

## 8. Troubleshooting

- **“Connection refused” to Kafka**  
  - Broker not running: start Kafka (Option A or B) and wait a few seconds.  
  - Wrong address: from host use `localhost:9092`; from another container in the same compose use `kafka:9092`.

- **No messages in the consumer**  
  - Wait for at least one Pulse cycle (~15 minutes, or ~15 seconds after startup for the first run).  
  - Use `--from-beginning` so you don’t miss messages produced before the consumer started.

- **Pulse logs “kafka produce” error**  
  - Check broker address and that nothing else is using port 9092.  
  - With Option A, ensure `docker-compose.kafka.yml` is applied so the pipeline has `KAFKA_BROKERS=kafka:9092`.

- **Stop and remove standalone Kafka**  
  ```bash
  docker stop pulse-kafka
  docker rm pulse-kafka
  ```

- **Consumer says “command not found”**  
  With Confluent the command is `kafka-console-consumer` (no `.sh`). If it’s not in PATH, try the full path inside the container, e.g. `/usr/bin/kafka-console-consumer` or look under the image’s Kafka install dir.

- **Apache Kafka image asks for `zookeeper.connect`**  
  The official `apache/kafka` image can default to Zookeeper mode. This project uses **Confluent Kafka** in the compose file (`confluentinc/cp-kafka:7.6.9`) so it runs in KRaft mode (no Zookeeper).

- **`manifest for bitnami/kafka:3.9 not found`**  
  Bitnami image tags change. This project’s compose uses **Confluent** (`confluentinc/cp-kafka:7.6.9`). If you use the standalone `docker run` in step 4.1, use the same Confluent image (see step 4.1 for the correct env vars).

That’s the full path from zero to producing and consuming Pulse data in Kafka.
