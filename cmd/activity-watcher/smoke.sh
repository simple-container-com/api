#!/usr/bin/env bash
# smoke.sh — end-to-end smoke test for the activity-watcher service.
# Usage: BASE_URL=http://localhost:8080 ./cmd/activity-watcher/smoke.sh
set -euo pipefail

BASE="${BASE_URL:-http://localhost:8080}"
PASS=0
FAIL=0

pass() { echo "  PASS: $1"; ((PASS++)) || true; }
fail() { echo "  FAIL: $1"; ((FAIL++)) || true; }

echo "=== Activity Watcher Smoke Test ==="
echo "    Target: $BASE"
echo

# AC5: health
echo "--- AC5: GET /health ---"
HEALTH=$(curl -sf "$BASE/health")
if echo "$HEALTH" | grep -q '"ok"'; then
  pass "GET /health → 200 {\"status\":\"ok\"}"
else
  fail "GET /health → unexpected: $HEALTH"
fi

# AC1: valid ingest
echo "--- AC1: POST /events (valid) ---"
ID=$(curl -sf -X POST "$BASE/events" \
  -H 'Content-Type: application/json' \
  -d "{\"user_id\":\"smoke-user\",\"event_type\":\"page_view\",\"occurred_at\":\"$(date -u +%Y-%m-%dT%H:%M:%SZ -d '1 minute ago' 2>/dev/null || date -u -v-1M +%Y-%m-%dT%H:%M:%SZ)\",\"metadata\":{\"page\":\"/home\"}}" \
  | python3 -c "import sys,json; print(json.load(sys.stdin)['id'])" 2>/dev/null || true)
if [ -n "$ID" ]; then
  pass "POST /events → 201, id=$ID"
else
  fail "POST /events → did not return an id"
fi

# AC2: missing fields → 400
echo "--- AC2: POST /events (invalid, missing fields) ---"
STATUS=$(curl -s -o /dev/null -w "%{http_code}" -X POST "$BASE/events" \
  -H 'Content-Type: application/json' -d '{}')
if [ "$STATUS" = "400" ]; then
  pass "POST /events {} → 400"
else
  fail "POST /events {} → expected 400, got $STATUS"
fi

# AC2: future occurred_at → 400
STATUS=$(curl -s -o /dev/null -w "%{http_code}" -X POST "$BASE/events" \
  -H 'Content-Type: application/json' \
  -d '{"user_id":"u","event_type":"e","occurred_at":"2099-01-01T00:00:00Z","metadata":{}}')
if [ "$STATUS" = "400" ]; then
  pass "POST /events future date → 400"
else
  fail "POST /events future date → expected 400, got $STATUS"
fi

# AC4: retrieve events for user
echo "--- AC4: GET /users/smoke-user/events ---"
EVENTS=$(curl -sf "$BASE/users/smoke-user/events")
COUNT=$(echo "$EVENTS" | python3 -c "import sys,json; print(len(json.load(sys.stdin)))" 2>/dev/null || echo 0)
if [ "$COUNT" -ge 1 ]; then
  pass "GET /users/smoke-user/events → $COUNT event(s)"
else
  fail "GET /users/smoke-user/events → expected ≥1 events, got $COUNT"
fi

echo
echo "=== Results: $PASS passed, $FAIL failed ==="
[ "$FAIL" -eq 0 ] || exit 1
