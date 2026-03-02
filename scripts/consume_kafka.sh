#!/usr/bin/env bash
# Consume all messages from Kafka topic pulse.posts.
# Uses the topic's actual message count as --max-messages so the consumer never
# waits for more than exists (no timeout exit). Run from project root.
#
# Usage: ./scripts/consume_kafka.sh
# Optional: PULSE_NETWORK=pulse_default KAFKA_TOPIC=pulse.posts

set -e
TOPIC="${KAFKA_TOPIC:-pulse.posts}"
NETWORK="${PULSE_NETWORK:-pulse_default}"
CONFLUENT_IMAGE="${KAFKA_IMAGE:-confluentinc/cp-kafka:7.6.9}"

# Get actual message count from topic (sum of partition high-water offsets)
OFFSET_OUT=$(docker run --rm --network "$NETWORK" "$CONFLUENT_IMAGE" \
  kafka-run-class kafka.tools.GetOffsetShell \
  --broker-list kafka:9092 \
  --topic "$TOPIC" \
  --time -1 2>/dev/null) || true

if [[ -z "$OFFSET_OUT" ]]; then
  echo "Could not get topic $TOPIC offsets. Is Kafka up? (docker compose -f docker-compose.yml -f docker-compose.kafka.yml up -d)"
  exit 1
fi

MAX_MESSAGES=$(echo "$OFFSET_OUT" | awk -F: '{sum += $3} END {print sum}')
if [[ -z "$MAX_MESSAGES" ]] || [[ "$MAX_MESSAGES" -lt 1 ]]; then
  echo "Topic $TOPIC has no messages (count=$MAX_MESSAGES)."
  exit 0
fi

# Long timeout so we don't exit while reading (only used if broker is slow)
TIMEOUT_MS=300000

echo "Consuming all $MAX_MESSAGES messages from topic $TOPIC."
echo "---"
docker run --rm --network "$NETWORK" "$CONFLUENT_IMAGE" \
  kafka-console-consumer \
  --bootstrap-server kafka:9092 \
  --topic "$TOPIC" \
  --from-beginning \
  --max-messages "$MAX_MESSAGES" \
  --timeout-ms "$TIMEOUT_MS"

echo "---"
echo "Done. Consumed all $MAX_MESSAGES messages."
