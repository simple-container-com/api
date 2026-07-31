# Activity Watcher — Phase 1 Backend MVP

**Date:** 2026-06-12
**Slice:** `activity-watcher-phase-1-backend-mvp`
**Status:** Implemented

## Problem Statement

The platform lacked any mechanism to record user or system activity events. This design covers the minimal backend service to ingest, persist, and retrieve activity events — enough to prove the pattern and unblock downstream consumers.

## Decisions

| Decision | Choice | Rationale |
|---|---|---|
| Language | Go 1.26 | Matches repo standard; single static binary |
| Storage | SQLite (WAL mode) via `modernc.org/sqlite` | Zero infra, ACID, no CGO required, trivially swappable in Phase 2 |
| Router | `net/http` stdlib (Go 1.22 pattern routing) | 3 routes; no framework needed |
| Architecture | Handler → Service → Repository | Layered; Repository interface allows Phase 2 swap to Postgres |
| Event ID | UUID v4 | Safe for distributed insertion in Phase 2 |
| Timestamps | `occurred_at` (client) + `created_at` (server) | Correct semantic split for activity tracking |
| Auth | None (Phase 1) | Internal only; stub TODO comment in handler |

## Directory Layout

```
cmd/activity-watcher/
  main.go          — wiring, signal handling, HTTP server
  main_test.go     — integration tests (real SQLite, httptest)
  helpers_test.go  — test utilities
  Dockerfile       — multi-stage, CGO_ENABLED=0
  smoke.sh         — end-to-end smoke script

internal/activitywatcher/
  model/event.go         — Event, EventInput, Validate()
  repository/interface.go — EventRepository interface
  repository/sqlite.go   — SQLite implementation + schema migration
  service/events.go      — CreateEvent, ListUserEvents
  handler/events.go      — POST /events, GET /users/:id/events
  handler/health.go      — GET /health

api/activity-watcher-openapi.yaml — OpenAPI 3.0 spec
```

## API

| Method | Path | Purpose |
|---|---|---|
| GET | /health | Liveness probe |
| POST | /events | Ingest event |
| GET | /users/{user_id}/events | List user's events, newest first |

## Data Model

```sql
CREATE TABLE events (
    id          TEXT PRIMARY KEY,          -- UUID v4
    user_id     TEXT NOT NULL,
    event_type  TEXT NOT NULL,
    occurred_at DATETIME NOT NULL,         -- client-supplied, RFC3339
    metadata    TEXT NOT NULL DEFAULT '{}', -- free-form JSON
    created_at  DATETIME NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now'))
);
CREATE INDEX idx_events_user_occurred ON events (user_id, occurred_at DESC);
PRAGMA journal_mode=WAL;
```

## Validation Rules

| Field | Rule |
|---|---|
| user_id | Required, non-empty, ≤255 chars |
| event_type | Required, non-empty, ≤255 chars |
| occurred_at | Required, RFC3339, not future, not >30 days old |
| metadata | Required, valid JSON object (may be `{}`) |

## Configuration

All config via environment variables:

| Variable | Default | Purpose |
|---|---|---|
| DB_PATH | /data/events.db | SQLite file path |
| PORT | 8080 | HTTP listen port |
| LOG_LEVEL | info | Logging verbosity (debug/info/warn/error) |

## Phase 2 Roadmap

- Swap `SQLiteRepository` for `PostgresRepository` (interface already extracted)
- Add API-key middleware (stub `TODO: auth` present in handler)
- Pagination + filtering on list endpoint
- Metrics endpoint (`/metrics`)
