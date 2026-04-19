# Market Research System - Data Collection Layer

**Status:** Approved - ready for implementation planning
**Date:** 2026-04-18
**Scope:** Subsystem #1 of 5 in the broader market-research agentic system.

---

## Context and Decomposition

The broader product is an agentic market-research system: given a topic, it surfaces live pain points from public discussion platforms, forward-projects where the pain is heading, generates differentiated solutions, and generates tooling for those solutions.

That product is not buildable as a single spec. It is being decomposed into five subsystems, each with its own spec → plan → implementation cycle:

1. **Data collection** (this spec) - pull relevant raw signal into local storage.
2. **Pain-point extraction** - LLM-driven categorization of problems from raw signal.
3. **Trend forward-projection** - where is a pain point heading, how fast is it growing.
4. **Solution generation** - differentiated solutions ahead of the current market state.
5. **Tool generation** - implementation scaffolds for generated solutions.

This spec covers **only** subsystem #1. Downstream subsystems are addressed at the end under "Downstream Subsystems Contract" to ensure the data layer preserves what they will need.

---

## Decisions (Questions 1-10)

| # | Decision | Rationale |
|---|---|---|
| 1 | Decompose the broader product. Brainstorm data collection first. | Avoids vague "platform" spec; each piece gets focused attention. |
| 2 | "Smart targeted" mode - user gives a topic, agent translates it into sources. | User shouldn't have to know every relevant subreddit. |
| 3 | Agent discovers sources **and** learns over time (expands/prunes). | Source list compounds in quality as system runs. |
| 4 | v1 platforms: **Reddit + Stack Overflow only**. | Both free, both rich in pain-point content. Google and X deferred (cost, complexity, weaker signal for B2B niches). |
| 5 | Daily batch cadence. | Pain points emerge over weeks, not hours. Avoids real-time infra overhead. |
| 6 | **SQLite** local file for storage. | Zero infra, good up to ~10GB, trivial to back up. Migratable later. |
| 7 | Deploy to a **tiny always-on VM** (~$0-5/mo). | Avoids corpus gaps from laptop sleep/travel. Matches infra-engineer instincts. |
| 8 | **Go** as implementation language. | Matches user's skill stack. Small binary, native concurrency, solid Anthropic SDK. |
| 9 | **CLI-managed topics** (`mr topic add/list/remove`) + **weekly source rediscovery** by the agent. | Unix-native tool shape for solo operator on own VM. |
| 10 | **Medium depth**: posts + top-N comments (Reddit), questions + accepted/top answer (SO). | Posts alone miss most pain-point signal; full comment trees are mostly noise. |
| Arch | **Approach 2** - CLI + systemd timers (no in-process scheduler). | Scheduling owned by the OS. Failures visible in `journalctl`. Each job isolated and independently diagnosable. |
| Handoff | Shared SQLite file, **schema-as-contract**. | Matches single-host deployment. Future analysis subsystems bind to schema, not to an API. |

---

## Section 1 - Architecture and Components

Single Go binary `mr` deployed to a small VM, orchestrated by systemd. All state lives in one SQLite file. Four internal packages, each with one job:

```
market-research/
├── cmd/mr/           main.go + CLI entrypoints (cobra): topic, fetch, rediscover, doctor
├── internal/store/   SQLite access. Owns schema. Only package that writes SQL.
├── internal/sources/ Source-discovery agent. Calls Claude API with the topic,
│                     produces a SourcePlan (subreddits, SO tags, search queries).
├── internal/fetch/   Platform clients: fetch/reddit, fetch/stackoverflow.
│                     Pure "given a SourcePlan, pull posts" - no schema coupling.
└── internal/config/  Secrets, paths.
```

**Systemd units on the VM:**

- `mr-fetch.timer` → fires daily (04:00 UTC) → runs `mr fetch --all`
- `mr-rediscover.timer` → fires weekly (Sunday 03:00 UTC) → runs `mr rediscover --all`

**Boundaries that matter:**

- `store` is the only package that touches SQL. Everything else takes typed structs. This is the schema-as-contract boundary the future analysis layer binds to.
- `sources` (agent) and `fetch` (scrapers) are strictly separated. The agent never fetches. The fetchers never make LLM calls. This keeps Claude API spend predictable and makes `fetch` pure and retryable.
- CLI commands in `cmd/` are thin - flag parsing + wiring. No business logic.

---

## Section 2 - Data Model (Schema-as-Contract)

Five tables. Reddit posts and Stack Overflow questions are **unified** into a single `documents` table so downstream consumers see one shape.

```sql
-- User-configured research targets
CREATE TABLE topics (
  id           INTEGER PRIMARY KEY,
  name         TEXT NOT NULL UNIQUE,
  description  TEXT,
  created_at   TIMESTAMP NOT NULL,
  active       BOOLEAN NOT NULL DEFAULT 1
);

-- Per-topic source plan (what to watch)
CREATE TABLE sources (
  id            INTEGER PRIMARY KEY,
  topic_id      INTEGER NOT NULL REFERENCES topics(id),
  platform      TEXT NOT NULL,           -- 'reddit' | 'stackoverflow'
  kind          TEXT NOT NULL,           -- 'subreddit' | 'so_tag' | 'search_query'
  value         TEXT NOT NULL,
  added_at      TIMESTAMP NOT NULL,
  added_by      TEXT NOT NULL,           -- 'agent' | 'manual'
  last_fetched  TIMESTAMP,
  signal_score  REAL,                    -- 0.0-1.0, agent-maintained
  active        BOOLEAN NOT NULL DEFAULT 1,
  UNIQUE(topic_id, platform, kind, value)
);

-- Unified: Reddit posts + SO questions
CREATE TABLE documents (
  id                 INTEGER PRIMARY KEY,
  topic_id           INTEGER NOT NULL REFERENCES topics(id),
  source_id          INTEGER NOT NULL REFERENCES sources(id),
  platform           TEXT NOT NULL,
  platform_id        TEXT NOT NULL,      -- Reddit post ID, SO question ID
  title              TEXT NOT NULL,
  body               TEXT,
  author             TEXT,
  score              INTEGER,
  url                TEXT NOT NULL,      -- permalink for downstream citation
  created_at         TIMESTAMP NOT NULL, -- when posted (for trend velocity)
  fetched_at         TIMESTAMP NOT NULL,
  platform_metadata  TEXT,               -- JSON blob for platform-specific fields
  UNIQUE(platform, platform_id)
);

-- Unified: Reddit top-N comments + SO accepted/top answer
CREATE TABLE document_replies (
  id             INTEGER PRIMARY KEY,
  document_id    INTEGER NOT NULL REFERENCES documents(id),
  platform_id    TEXT NOT NULL UNIQUE,
  body           TEXT NOT NULL,
  author         TEXT,
  score          INTEGER,
  created_at     TIMESTAMP NOT NULL,
  is_accepted    BOOLEAN
);

-- Observability log
CREATE TABLE fetch_runs (
  id             INTEGER PRIMARY KEY,
  topic_id       INTEGER NOT NULL REFERENCES topics(id),
  platform       TEXT NOT NULL,
  started_at     TIMESTAMP NOT NULL,
  ended_at       TIMESTAMP,
  status         TEXT NOT NULL,          -- 'running' | 'success' | 'error'
  documents_new  INTEGER,
  replies_new    INTEGER,
  error_message  TEXT
);

CREATE INDEX idx_documents_topic_created ON documents(topic_id, created_at DESC);
CREATE INDEX idx_documents_fetched ON documents(fetched_at DESC);
CREATE INDEX idx_replies_document ON document_replies(document_id);
```

**Why unified documents table:**
Downstream pain-point extraction wants a single query across all platforms per topic in a time window. Platform-specific fields (subreddit, flair, SO tags, view_count, accepted_answer_id) live in `platform_metadata` JSON so the schema stays stable when X and Google are added later.

**Why `url` and `created_at` are non-nullable:**
Downstream solution generation needs `url` for citation in generated output. Trend forward-projection needs `created_at` to compute growth velocity. These are contract obligations on the data layer.

**Deduplication:**
`UNIQUE(platform, platform_id)` on documents. Re-fetches and overlapping source lists won't double-count.

---

## Section 3 - Source-Discovery Agent

**Location:** `internal/sources/`
**Model:** `claude-sonnet-4-6` (structured extraction, not reasoning-heavy; ~5x cheaper than Opus)
**Triggers:**
- `mr topic add <name>` → initial discovery
- `mr rediscover --all` → weekly reassessment per existing topic

**Structured output via Claude tool use.** The agent is forced to produce a single tool call matching:

```json
{
  "reddit": {
    "subreddits": ["devsecops", "cybersecurity", "sysadmin"],
    "search_queries": ["\"soc2 audit\" pain", "soc2 compliance tool"]
  },
  "stackoverflow": {
    "tags": ["compliance", "security"],
    "search_queries": ["soc2 audit evidence collection"]
  },
  "reasoning": "Short explanation per choice."
}
```

Tool-call validation eliminates JSON parsing fragility. Invalid outputs fail loudly in `fetch_runs`-style logging.

**Two prompt variants:**

- **Initial discovery** - Topic name + description only. Agent proposes starting source plan from scratch.
- **Weekly rediscovery** - Topic name + description + current source list with per-source stats (docs pulled last 7 days, avg doc score, current signal_score). Agent told: expand, prune, or reprioritize.

**Signal scoring:**

- Initial value per new source: `0.5`.
- Updated during weekly rediscovery from heuristic: `docs_per_week * avg_doc_score`, min-max normalized per topic.
- Low-scoring sources pruned by the agent over time, unless manually pinned.

**Safety rails:**

- Cap per platform per topic: 10 subreddits, 5 SO tags, 5 search queries.
- Manual sources (`added_by = 'manual'`) are immutable by the agent.
- `mr rediscover --dry-run --topic <name>` prints proposed changes without writing.

---

## Section 4 - Fetch Pipeline

**Entrypoint:** `mr fetch --all` invoked daily by systemd. Also `mr fetch --topic <name>` for ad-hoc pulls.

**Flow per run:**

```
for each active topic:
  insert fetch_runs row (status='running')
  for each platform in [reddit, stackoverflow] (concurrent goroutines):
    for each active source (topic_id, platform) (sequential per platform):
      pull new docs since last_fetched
      upsert documents ON CONFLICT (platform, platform_id) DO NOTHING
      for each new doc:
        fetch top-N replies
        upsert document_replies
      update sources.last_fetched
  close fetch_runs row (status + counts)
```

**Rate-limit discipline:**

- **Reddit** - OAuth app creds (`script` type), effective limit ~60 req/min. Client uses `golang.org/x/time/rate` at 50 req/min. Endpoints: `/r/{sub}/new?limit=100`, `/search?q=...&restrict_sr=on`, `/comments/{id}.json?limit=10&sort=top`.
- **Stack Overflow** - Stack Exchange API v2.3. Free API key gets 10k requests/day. Rate limiter at 1 req/sec. `filter=withbody` gets question body + accepted answer in one call. Endpoint: `/questions?tagged={tag}&fromdate={unix_ts}&sort=creation`.

**Incremental fetching:**

- Each source tracks `last_fetched`.
- Reddit: fetch posts with `created_utc > last_fetched`. Cap at 100 posts/source/day.
- Stack Overflow: `fromdate` query param.
- First fetch for a new source: bounded 7-day backfill.

**Concurrency:**

- Topics processed sequentially.
- Within a topic, platforms concurrent (2 goroutines).
- Within a platform, sources sequential (respect rate limits).

**Error handling:**

- **Transient** (5xx, network, 429): exponential backoff, max 3 retries. Source logged as errored; run continues with other sources.
- **Permanent** (403, 404): source auto-deactivated (`active = 0`), surfaced via `mr topic list --issues`.
- **Partial success**: `fetch_runs.documents_new` reflects actual inserts, not attempts.

**Secrets:**

`/etc/mr/env` on the VM. File mode 0600, owned by the `mr` systemd user. Holds `REDDIT_CLIENT_ID`, `REDDIT_CLIENT_SECRET`, `STACKEXCHANGE_KEY`, `ANTHROPIC_API_KEY`. Loaded by systemd via `EnvironmentFile=`. No secrets in SQLite, no secrets in source control.

---

## Section 5 - Observability and System-Level Error Handling

The `fetch_runs` table gives per-run visibility inside the DB, but that alone is not enough: if the binary fails to start (bad env file, DB locked, panic at init) there is no row to read. Observability must survive process failure.

**Three layers, cheapest first:**

**1) systemd + journald (free, on the VM)**

- `mr-fetch.service` and `mr-rediscover.service` write stdout/stderr to journald automatically.
- Structured logs via `log/slog` with the JSON handler. `journalctl -u mr-fetch -o json` gives queryable history.
- Exit codes are meaningful: `0` success, `1` partial (some sources errored), `2` fatal (no runs completed).
- `OnFailure=` hook triggers a `mr-notify@.service` unit on exit code 2. Hook wired now; notifier deferred.

**2) Self-diagnostic CLI (`mr doctor`)**

Single command that reports health without external tooling. Reports:

- Last successful `fetch_runs` per topic per platform (and how long ago).
- Sources with no successful fetch in >7 days.
- Topics with zero documents in the last 14 days (likely dead source plan).
- API key validity via lightweight probes: Reddit `/api/v1/me`, Stack Exchange `/me`, Anthropic 1-token `/v1/messages`.
- SQLite file size, free pages, last VACUUM timestamp.

Intended usage: `ssh vm mr doctor`, or wire to a weekly cron that emails the output.

**3) External alerting (deferred to v1.1)**

The `OnFailure=` systemd hook is wired in v1. The notifier implementation (email, Telegram, ntfy.sh) is deliberately deferred until the system has run long enough to reveal which alerts matter. Shipping alerting before knowing the signal-to-noise ratio leads to alert fatigue and custom filtering later.

**Panic and unexpected-error discipline:**

- Top-level `recover()` in `main()` logs the panic with full stack via `slog`, marks any open `fetch_runs` row as `status='error'` with the panic message, exits with code 2.
- `defer` pattern used per topic-loop iteration so one topic's panic does not abort the whole daily run.

**Claude API failure modes:**

- `rediscover` failure: sources untouched (last good plan remains active), failure logged loudly in `fetch_runs`, next weekly run retries.
- `topic add` failure: topic row is created with `active = 0`. User can re-run `mr topic rediscover <name>` manually.

---

## Section 6 - Testing Approach

Four layers of tests, ordered by how much code they cover per unit of cost, plus one end-to-end smoke test.

**1) Unit tests (`*_test.go` next to source)**

- Source plan validation (caps, dedup, manual-source immutability).
- Rate-limiter wrapping under load (fake clock).
- Platform-metadata JSON marshaling.
- Signal-score heuristic math.
- CLI flag parsing in `cmd/`.

**2) Store integration tests (real SQLite, no mocks)**

Integration tests hit a real database, never a mock. Each test spins up a fresh `:memory:` SQLite (or a temp file for WAL-mode tests), runs schema migrations, exercises `internal/store/` directly. Covers:

- Migrations apply cleanly on empty and populated DBs.
- `UNIQUE(platform, platform_id)` dedup behavior.
- Upsert semantics under concurrent writes (SQLite WAL mode).
- `fetch_runs` state transitions (running → success/error).
- Foreign-key cascade on topic deletion.

**3) Platform client tests (`httptest.Server` fixtures)**

`internal/fetch/reddit` and `internal/fetch/stackoverflow` are tested against in-process HTTP servers returning recorded JSON responses. No live API calls, no flakiness, no rate-limit concerns.

Fixtures live in `internal/fetch/testdata/` as real Reddit/SO response payloads captured once and scrubbed of user data. When platforms change schemas, fixtures get re-recorded. Covers:

- Pagination edge cases (empty, partial, deleted-content markers).
- 429 backoff and retry behavior.
- Permanent-error source deactivation (403, 404).
- Malformed JSON handling.

**4) Agent tests (interface stub, no live Claude calls)**

`internal/sources/` takes a `ClaudeClient` interface, not the concrete `anthropic.Client`. Tests inject a stub returning canned tool-call responses. No LLM calls in CI, no cost, fully deterministic. Covers:

- Tool-call schema validation (invalid agent output → typed error).
- Cap enforcement (agent returns 20 subreddits → system trims to 10).
- Manual-source immutability during rediscovery.
- Signal-score recalculation inputs match prompt requirements.

**5) One end-to-end smoke test**

`cmd/mr/e2e_test.go` exercises the full pipeline once with:

- Real SQLite (temp file).
- Stubbed Claude client (canned SourcePlan).
- Stubbed Reddit + Stack Overflow servers (`httptest`).
- Single topic, single source per platform.

Asserts: `mr topic add` → `mr fetch` → documents in DB, replies in DB, `fetch_runs` closed success. Only test that exercises `cmd/` wiring end-to-end. Runs in under 2 seconds.

**Explicitly NOT doing:**

- Mocks for SQLite. Real DB always.
- Live Reddit / Stack Overflow / Claude calls in CI. Deterministic fixtures only.
- Coverage targets. Cover what matters (store, fetch, sources). Skip glue code.

**CI:**

GitHub Actions workflow runs `go test ./... -race` on every push. No secrets in CI for v1 (no live-API tests). Nightly live-API smoke tests are a future add once fixture-drift rate is understood.

---

## Downstream Subsystems Contract

Subsystems 2-5 (pain-point extraction, trend forward-projection, solution generation, tool generation) will consume from this data layer. This section documents constraints the data layer must uphold so those downstream layers function.

**What downstream reads:**

1. `documents` + `document_replies` filtered by `topic_id` and time window.
2. `platform_metadata` for credibility weighting (e.g., SO accepted answers, high-score Reddit posts).
3. `fetch_runs` for corpus-completeness gating. If a platform has no successful run in the last N days for a topic, downstream should refuse to draw trend conclusions.

**Fields downstream depends on:**

- `documents.url` - citation in generated marketing/solution content.
- `documents.created_at` - trend velocity (growth rate, half-life, seasonality).
- `documents.score` + `document_replies.score` - credibility weighting.
- `documents.platform_metadata.accepted` (SO) - resolved-question signal.
- `documents.platform_metadata.subreddit` (Reddit) - audience segmentation.

**Schema-change rules:**

After v1 of this layer ships, changes to `documents` or `document_replies` break downstream. Breaking changes require a bumped schema version column in a new `schema_meta` table, plus a coordinated update to the analysis subsystem.

Additive changes (new columns, new tables) are safe.

---

## Open Questions and Future Work

- External alerting notifier (Section 5, layer 3) deferred to v1.1 once alert signal-to-noise is known.
- Google Trends and X ingestion deferred to future specs. When added they slot into the existing `documents` table via new platform values.
- Full-text search over `documents` is likely needed by downstream consumers. SQLite FTS5 is the planned add, scoped as a separate follow-up, not v1 of this layer.
- Nightly live-API smoke tests deferred until fixture-drift rate is understood.
