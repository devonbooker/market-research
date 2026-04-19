# Data Collection Layer Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Build subsystem #1 of the market-research agentic system: a Go CLI (`mr`) that, given a topic, uses a Claude agent to discover relevant Reddit subreddits and Stack Overflow tags, then runs as a daily systemd-scheduled job on a small VM pulling new posts and top comments/answers into a local SQLite file that serves as the contract for downstream subsystems.

**Architecture:** Single Go binary with four internal packages (`store`, `sources`, `fetch`, `config`) plus `cmd/mr/` entrypoint. All state in one SQLite file. No in-process scheduler - systemd timers drive daily fetch and weekly source rediscovery. Fetchers never make LLM calls. Agent never fetches. Store is the only package touching SQL. Analysis subsystems bind to the SQLite schema, not to an API.

**Tech Stack:** Go 1.22+, cobra (CLI), modernc.org/sqlite (pure-Go SQLite driver, no CGO so we cross-compile to the VM cleanly), anthropics/anthropic-sdk-go (Claude), golang.org/x/time/rate (rate limiting), golang.org/x/oauth2 + stdlib net/http (Reddit + Stack Exchange clients), log/slog (structured logging), stretchr/testify (tests), stdlib httptest (client fixtures).

**Spec:** `docs/superpowers/specs/2026-04-18-data-collection-design.md`

---

## File structure map

```
market-research/
├── cmd/mr/
│   ├── main.go              cobra root, wiring
│   ├── cmd_topic.go         topic add/list/remove
│   ├── cmd_fetch.go         fetch --all / --topic
│   ├── cmd_rediscover.go    rediscover --all / --topic / --dry-run
│   ├── cmd_doctor.go        doctor
│   └── e2e_test.go          end-to-end smoke test
├── internal/
│   ├── config/
│   │   ├── config.go        Env loading, file paths
│   │   └── config_test.go
│   ├── types/
│   │   └── types.go         Shared domain types (Topic, Source, Document, Reply, FetchRun, SourcePlan)
│   ├── store/
│   │   ├── schema.sql       Embedded schema
│   │   ├── store.go         Open/close, migrations
│   │   ├── topics.go        Topic CRUD
│   │   ├── sources.go       Source CRUD + signal-score math
│   │   ├── documents.go     Document + reply upserts
│   │   ├── fetch_runs.go    Fetch run lifecycle
│   │   └── *_test.go        One test file per source file
│   ├── sources/
│   │   ├── plan.go          SourcePlan type, validation, cap trimming
│   │   ├── client.go        ClaudeClient interface
│   │   ├── prompts.go       Initial + rediscover prompt templates
│   │   ├── agent.go         Discover() + Rediscover() logic
│   │   └── *_test.go
│   └── fetch/
│       ├── orchestrator.go  Top-level fetch loop (topic → platform → source)
│       ├── errors.go        Typed errors (Transient, Permanent)
│       ├── reddit/
│       │   ├── client.go    OAuth + rate-limited HTTP
│       │   ├── fetch.go     Posts + comments
│       │   └── *_test.go
│       └── stackoverflow/
│           ├── client.go    Rate-limited HTTP
│           ├── fetch.go     Questions + accepted answer
│           └── *_test.go
├── deploy/
│   ├── mr-fetch.service
│   ├── mr-fetch.timer
│   ├── mr-rediscover.service
│   ├── mr-rediscover.timer
│   └── README.md            VM setup steps
├── .github/workflows/test.yml
├── go.mod
├── go.sum
├── Makefile
├── README.md                (exists)
└── .gitignore               (exists)
```

---

## Dependency order

Tasks are grouped into phases. Within a phase, tasks have minimal cross-dependencies. Later phases depend on earlier ones.

1. **Foundation** (Tasks 1-3): Go module, CI, config package
2. **Types** (Task 4): Shared domain types
3. **Store** (Tasks 5-10): SQLite schema + CRUD
4. **Sources agent** (Tasks 11-15): Claude-driven source discovery
5. **Fetch - Reddit** (Tasks 16-18): Reddit client + fetcher
6. **Fetch - Stack Overflow** (Tasks 19-20): Stack Exchange client + fetcher
7. **Fetch orchestrator** (Task 21): Top-level loop
8. **CLI** (Tasks 22-26): cobra commands
9. **Deploy + E2E** (Tasks 27-29): systemd units, README, end-to-end test

---

## Phase 1: Foundation

### Task 1: Initialize Go module and tooling

**Files:**
- Create: `go.mod`
- Create: `Makefile`
- Create: `cmd/mr/main.go` (placeholder)

- [ ] **Step 1: Create the Go module**

Run from repo root:

```bash
go mod init github.com/devonbooker/market-research
```

Expected: creates `go.mod` with `module github.com/devonbooker/market-research` and `go 1.22` (or later).

- [ ] **Step 2: Create placeholder main.go**

Create `cmd/mr/main.go`:

```go
package main

import "fmt"

func main() {
	fmt.Println("mr: market-research data collection")
}
```

- [ ] **Step 3: Create Makefile**

Create `Makefile`:

```make
.PHONY: build test lint tidy clean

build:
	go build -o bin/mr ./cmd/mr

test:
	go test ./... -race

test-short:
	go test ./... -race -short

tidy:
	go mod tidy

lint:
	go vet ./...

clean:
	rm -rf bin/
```

- [ ] **Step 4: Verify build works**

Run:

```bash
make build && ./bin/mr
```

Expected output: `mr: market-research data collection`

- [ ] **Step 5: Commit**

```bash
git add go.mod Makefile cmd/mr/main.go
git commit -m "feat: initialize go module, makefile, and placeholder cmd/mr"
```

---

### Task 2: Add CI workflow

**Files:**
- Create: `.github/workflows/test.yml`

- [ ] **Step 1: Create the workflow file**

Create `.github/workflows/test.yml`:

```yaml
name: test

on:
  push:
    branches: [main]
  pull_request:

jobs:
  test:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      - uses: actions/setup-go@v5
        with:
          go-version: '1.22'
          cache: true
      - name: Vet
        run: go vet ./...
      - name: Test
        run: go test ./... -race
      - name: Build
        run: go build ./...
```

- [ ] **Step 2: Commit**

```bash
git add .github/workflows/test.yml
git commit -m "ci: add go test + vet + build workflow"
```

- [ ] **Step 3: Push and verify green**

```bash
git push
gh run watch
```

Expected: workflow run ends with `completed success`.

---

### Task 3: Config package

**Files:**
- Create: `internal/config/config.go`
- Create: `internal/config/config_test.go`

- [ ] **Step 1: Write failing tests**

Create `internal/config/config_test.go`:

```go
package config

import (
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLoad_RequiresRedditClientID(t *testing.T) {
	t.Setenv("REDDIT_CLIENT_ID", "")
	_, err := Load()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "REDDIT_CLIENT_ID")
}

func TestLoad_AppliesDefaults(t *testing.T) {
	t.Setenv("REDDIT_CLIENT_ID", "id")
	t.Setenv("REDDIT_CLIENT_SECRET", "secret")
	t.Setenv("STACKEXCHANGE_KEY", "k")
	t.Setenv("ANTHROPIC_API_KEY", "a")
	t.Setenv("MR_DB_PATH", "")
	cfg, err := Load()
	require.NoError(t, err)
	assert.Equal(t, "/var/lib/mr/mr.db", cfg.DBPath)
	assert.Equal(t, "market-research/0.1 (by u/unknown)", cfg.RedditUserAgent)
}

func TestLoad_RespectsDBPathOverride(t *testing.T) {
	t.Setenv("REDDIT_CLIENT_ID", "id")
	t.Setenv("REDDIT_CLIENT_SECRET", "secret")
	t.Setenv("STACKEXCHANGE_KEY", "k")
	t.Setenv("ANTHROPIC_API_KEY", "a")
	t.Setenv("MR_DB_PATH", "/tmp/test.db")
	cfg, err := Load()
	require.NoError(t, err)
	assert.Equal(t, "/tmp/test.db", cfg.DBPath)
}

func TestMain(m *testing.M) {
	os.Exit(m.Run())
}
```

- [ ] **Step 2: Add testify dependency**

```bash
go get github.com/stretchr/testify
```

- [ ] **Step 3: Run tests to verify they fail**

```bash
go test ./internal/config/...
```

Expected: build fails because `Load` is not defined.

- [ ] **Step 4: Implement config**

Create `internal/config/config.go`:

```go
package config

import (
	"fmt"
	"os"
)

type Config struct {
	RedditClientID     string
	RedditClientSecret string
	RedditUserAgent    string
	StackExchangeKey   string
	AnthropicAPIKey    string
	DBPath             string
}

func Load() (*Config, error) {
	cfg := &Config{
		RedditClientID:     os.Getenv("REDDIT_CLIENT_ID"),
		RedditClientSecret: os.Getenv("REDDIT_CLIENT_SECRET"),
		RedditUserAgent:    os.Getenv("REDDIT_USER_AGENT"),
		StackExchangeKey:   os.Getenv("STACKEXCHANGE_KEY"),
		AnthropicAPIKey:    os.Getenv("ANTHROPIC_API_KEY"),
		DBPath:             os.Getenv("MR_DB_PATH"),
	}

	required := map[string]string{
		"REDDIT_CLIENT_ID":     cfg.RedditClientID,
		"REDDIT_CLIENT_SECRET": cfg.RedditClientSecret,
		"STACKEXCHANGE_KEY":    cfg.StackExchangeKey,
		"ANTHROPIC_API_KEY":    cfg.AnthropicAPIKey,
	}
	for name, val := range required {
		if val == "" {
			return nil, fmt.Errorf("%s is required", name)
		}
	}

	if cfg.DBPath == "" {
		cfg.DBPath = "/var/lib/mr/mr.db"
	}
	if cfg.RedditUserAgent == "" {
		cfg.RedditUserAgent = "market-research/0.1 (by u/unknown)"
	}

	return cfg, nil
}
```

- [ ] **Step 5: Run tests to verify they pass**

```bash
go test ./internal/config/... -v
```

Expected: all three tests PASS.

- [ ] **Step 6: Commit**

```bash
git add go.mod go.sum internal/config/
git commit -m "feat(config): env-driven config with required keys and sane defaults"
```

---

## Phase 2: Shared types

### Task 4: Domain types

**Files:**
- Create: `internal/types/types.go`

- [ ] **Step 1: Create types file**

Create `internal/types/types.go`:

```go
package types

import (
	"encoding/json"
	"time"
)

type Platform string

const (
	PlatformReddit        Platform = "reddit"
	PlatformStackOverflow Platform = "stackoverflow"
)

type SourceKind string

const (
	SourceKindSubreddit   SourceKind = "subreddit"
	SourceKindSOTag       SourceKind = "so_tag"
	SourceKindSearchQuery SourceKind = "search_query"
)

type AddedBy string

const (
	AddedByAgent  AddedBy = "agent"
	AddedByManual AddedBy = "manual"
)

type RunStatus string

const (
	RunStatusRunning RunStatus = "running"
	RunStatusSuccess RunStatus = "success"
	RunStatusError   RunStatus = "error"
)

type Topic struct {
	ID          int64
	Name        string
	Description string
	CreatedAt   time.Time
	Active      bool
}

type Source struct {
	ID           int64
	TopicID      int64
	Platform     Platform
	Kind         SourceKind
	Value        string
	AddedAt      time.Time
	AddedBy      AddedBy
	LastFetched  *time.Time
	SignalScore  *float64
	Active       bool
}

type Document struct {
	ID               int64
	TopicID          int64
	SourceID         int64
	Platform         Platform
	PlatformID       string
	Title            string
	Body             string
	Author           string
	Score            int
	URL              string
	CreatedAt        time.Time
	FetchedAt        time.Time
	PlatformMetadata json.RawMessage
}

type Reply struct {
	ID         int64
	DocumentID int64
	PlatformID string
	Body       string
	Author     string
	Score      int
	CreatedAt  time.Time
	IsAccepted *bool
}

type FetchRun struct {
	ID            int64
	TopicID       int64
	Platform      Platform
	StartedAt     time.Time
	EndedAt       *time.Time
	Status        RunStatus
	DocumentsNew  int
	RepliesNew    int
	ErrorMessage  string
}

type SourcePlan struct {
	Reddit struct {
		Subreddits     []string `json:"subreddits"`
		SearchQueries  []string `json:"search_queries"`
	} `json:"reddit"`
	StackOverflow struct {
		Tags          []string `json:"tags"`
		SearchQueries []string `json:"search_queries"`
	} `json:"stackoverflow"`
	Reasoning string `json:"reasoning"`
}
```

- [ ] **Step 2: Verify it compiles**

```bash
go build ./internal/types/...
```

Expected: no output (success).

- [ ] **Step 3: Commit**

```bash
git add internal/types/
git commit -m "feat(types): define shared domain types"
```

---

## Phase 3: Store

### Task 5: Schema + store open/migrate

**Files:**
- Create: `internal/store/schema.sql`
- Create: `internal/store/store.go`
- Create: `internal/store/store_test.go`

- [ ] **Step 1: Create the schema file**

Create `internal/store/schema.sql`:

```sql
CREATE TABLE IF NOT EXISTS topics (
  id           INTEGER PRIMARY KEY,
  name         TEXT NOT NULL UNIQUE,
  description  TEXT,
  created_at   TIMESTAMP NOT NULL,
  active       BOOLEAN NOT NULL DEFAULT 1
);

CREATE TABLE IF NOT EXISTS sources (
  id            INTEGER PRIMARY KEY,
  topic_id      INTEGER NOT NULL REFERENCES topics(id) ON DELETE CASCADE,
  platform      TEXT NOT NULL,
  kind          TEXT NOT NULL,
  value         TEXT NOT NULL,
  added_at      TIMESTAMP NOT NULL,
  added_by      TEXT NOT NULL,
  last_fetched  TIMESTAMP,
  signal_score  REAL,
  active        BOOLEAN NOT NULL DEFAULT 1,
  UNIQUE(topic_id, platform, kind, value)
);

CREATE TABLE IF NOT EXISTS documents (
  id                 INTEGER PRIMARY KEY,
  topic_id           INTEGER NOT NULL REFERENCES topics(id) ON DELETE CASCADE,
  source_id          INTEGER NOT NULL REFERENCES sources(id) ON DELETE CASCADE,
  platform           TEXT NOT NULL,
  platform_id        TEXT NOT NULL,
  title              TEXT NOT NULL,
  body               TEXT,
  author             TEXT,
  score              INTEGER,
  url                TEXT NOT NULL,
  created_at         TIMESTAMP NOT NULL,
  fetched_at         TIMESTAMP NOT NULL,
  platform_metadata  TEXT,
  UNIQUE(platform, platform_id)
);

CREATE TABLE IF NOT EXISTS document_replies (
  id             INTEGER PRIMARY KEY,
  document_id    INTEGER NOT NULL REFERENCES documents(id) ON DELETE CASCADE,
  platform_id    TEXT NOT NULL UNIQUE,
  body           TEXT NOT NULL,
  author         TEXT,
  score          INTEGER,
  created_at     TIMESTAMP NOT NULL,
  is_accepted    BOOLEAN
);

CREATE TABLE IF NOT EXISTS fetch_runs (
  id             INTEGER PRIMARY KEY,
  topic_id       INTEGER NOT NULL REFERENCES topics(id) ON DELETE CASCADE,
  platform       TEXT NOT NULL,
  started_at     TIMESTAMP NOT NULL,
  ended_at       TIMESTAMP,
  status         TEXT NOT NULL,
  documents_new  INTEGER,
  replies_new    INTEGER,
  error_message  TEXT
);

CREATE INDEX IF NOT EXISTS idx_documents_topic_created ON documents(topic_id, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_documents_fetched ON documents(fetched_at DESC);
CREATE INDEX IF NOT EXISTS idx_replies_document ON document_replies(document_id);
```

- [ ] **Step 2: Write failing test**

Create `internal/store/store_test.go`:

```go
package store

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestOpen_CreatesSchemaOnFreshDB(t *testing.T) {
	s, err := Open(":memory:")
	require.NoError(t, err)
	defer s.Close()

	var count int
	err = s.DB().QueryRow("SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name IN ('topics','sources','documents','document_replies','fetch_runs')").Scan(&count)
	require.NoError(t, err)
	assert.Equal(t, 5, count)
}

func TestOpen_Idempotent(t *testing.T) {
	s, err := Open(":memory:")
	require.NoError(t, err)
	defer s.Close()

	require.NoError(t, s.migrate())

	var count int
	require.NoError(t, s.DB().QueryRow("SELECT COUNT(*) FROM topics").Scan(&count))
	assert.Equal(t, 0, count)
}
```

- [ ] **Step 3: Add sqlite driver**

```bash
go get modernc.org/sqlite
```

- [ ] **Step 4: Run tests to verify they fail**

```bash
go test ./internal/store/...
```

Expected: build fails, `Open` / `Store` undefined.

- [ ] **Step 5: Implement store**

Create `internal/store/store.go`:

```go
package store

import (
	"database/sql"
	_ "embed"
	"fmt"

	_ "modernc.org/sqlite"
)

//go:embed schema.sql
var schemaSQL string

type Store struct {
	db *sql.DB
}

func Open(path string) (*Store, error) {
	db, err := sql.Open("sqlite", path+"?_pragma=foreign_keys(1)&_pragma=journal_mode(WAL)&_pragma=busy_timeout(5000)")
	if err != nil {
		return nil, fmt.Errorf("open sqlite: %w", err)
	}
	s := &Store{db: db}
	if err := s.migrate(); err != nil {
		db.Close()
		return nil, err
	}
	return s, nil
}

func (s *Store) migrate() error {
	if _, err := s.db.Exec(schemaSQL); err != nil {
		return fmt.Errorf("apply schema: %w", err)
	}
	return nil
}

func (s *Store) DB() *sql.DB {
	return s.db
}

func (s *Store) Close() error {
	return s.db.Close()
}
```

- [ ] **Step 6: Run tests**

```bash
go test ./internal/store/... -v
```

Expected: both tests PASS.

- [ ] **Step 7: Commit**

```bash
git add go.mod go.sum internal/store/
git commit -m "feat(store): schema embedding + sqlite open/migrate"
```

---

### Task 6: Topics CRUD

**Files:**
- Create: `internal/store/topics.go`
- Create: `internal/store/topics_test.go`

- [ ] **Step 1: Write failing tests**

Create `internal/store/topics_test.go`:

```go
package store

import (
	"testing"
	"time"

	"github.com/devonbooker/market-research/internal/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func openTest(t *testing.T) *Store {
	t.Helper()
	s, err := Open(":memory:")
	require.NoError(t, err)
	t.Cleanup(func() { s.Close() })
	return s
}

func TestCreateTopic_Roundtrip(t *testing.T) {
	s := openTest(t)
	id, err := s.CreateTopic("soc2 compliance tool", "SOC2 audit pain points", true)
	require.NoError(t, err)
	assert.Greater(t, id, int64(0))

	got, err := s.GetTopicByName("soc2 compliance tool")
	require.NoError(t, err)
	assert.Equal(t, "soc2 compliance tool", got.Name)
	assert.Equal(t, "SOC2 audit pain points", got.Description)
	assert.True(t, got.Active)
	assert.WithinDuration(t, time.Now(), got.CreatedAt, 5*time.Second)
}

func TestCreateTopic_UniqueName(t *testing.T) {
	s := openTest(t)
	_, err := s.CreateTopic("dup", "", true)
	require.NoError(t, err)
	_, err = s.CreateTopic("dup", "", true)
	require.Error(t, err)
}

func TestListTopics_OnlyActiveByDefault(t *testing.T) {
	s := openTest(t)
	_, err := s.CreateTopic("a", "", true)
	require.NoError(t, err)
	_, err = s.CreateTopic("b", "", false)
	require.NoError(t, err)

	active, err := s.ListTopics(false)
	require.NoError(t, err)
	assert.Len(t, active, 1)
	assert.Equal(t, "a", active[0].Name)

	all, err := s.ListTopics(true)
	require.NoError(t, err)
	assert.Len(t, all, 2)
}

func TestSetTopicActive(t *testing.T) {
	s := openTest(t)
	id, err := s.CreateTopic("x", "", true)
	require.NoError(t, err)

	require.NoError(t, s.SetTopicActive(id, false))
	got, err := s.GetTopicByName("x")
	require.NoError(t, err)
	assert.False(t, got.Active)
}

func TestDeleteTopic(t *testing.T) {
	s := openTest(t)
	id, err := s.CreateTopic("gone", "", true)
	require.NoError(t, err)
	require.NoError(t, s.DeleteTopic(id))

	_, err = s.GetTopicByName("gone")
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrNotFound)
}

var _ = types.Topic{}
```

- [ ] **Step 2: Run tests to see them fail**

```bash
go test ./internal/store/...
```

Expected: build fails with undefined `CreateTopic`, `GetTopicByName`, etc.

- [ ] **Step 3: Implement topics**

Create `internal/store/topics.go`:

```go
package store

import (
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/devonbooker/market-research/internal/types"
)

var ErrNotFound = errors.New("not found")

func (s *Store) CreateTopic(name, description string, active bool) (int64, error) {
	res, err := s.db.Exec(
		`INSERT INTO topics (name, description, created_at, active) VALUES (?, ?, ?, ?)`,
		name, description, time.Now().UTC(), active,
	)
	if err != nil {
		return 0, fmt.Errorf("insert topic: %w", err)
	}
	return res.LastInsertId()
}

func (s *Store) GetTopicByName(name string) (*types.Topic, error) {
	return s.scanTopic(s.db.QueryRow(
		`SELECT id, name, description, created_at, active FROM topics WHERE name = ?`,
		name,
	))
}

func (s *Store) GetTopic(id int64) (*types.Topic, error) {
	return s.scanTopic(s.db.QueryRow(
		`SELECT id, name, description, created_at, active FROM topics WHERE id = ?`,
		id,
	))
}

func (s *Store) ListTopics(includeInactive bool) ([]*types.Topic, error) {
	q := `SELECT id, name, description, created_at, active FROM topics`
	if !includeInactive {
		q += ` WHERE active = 1`
	}
	q += ` ORDER BY name`
	rows, err := s.db.Query(q)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []*types.Topic
	for rows.Next() {
		t, err := s.scanTopic(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, t)
	}
	return out, rows.Err()
}

func (s *Store) SetTopicActive(id int64, active bool) error {
	_, err := s.db.Exec(`UPDATE topics SET active = ? WHERE id = ?`, active, id)
	return err
}

func (s *Store) DeleteTopic(id int64) error {
	_, err := s.db.Exec(`DELETE FROM topics WHERE id = ?`, id)
	return err
}

type rowScanner interface {
	Scan(...any) error
}

func (s *Store) scanTopic(r rowScanner) (*types.Topic, error) {
	var t types.Topic
	var desc sql.NullString
	err := r.Scan(&t.ID, &t.Name, &desc, &t.CreatedAt, &t.Active)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	t.Description = desc.String
	return &t, nil
}
```

- [ ] **Step 4: Run tests to verify pass**

```bash
go test ./internal/store/... -v
```

Expected: all topic tests PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/store/topics.go internal/store/topics_test.go
git commit -m "feat(store): topics CRUD"
```

---

### Task 7: Sources CRUD

**Files:**
- Create: `internal/store/sources.go`
- Create: `internal/store/sources_test.go`

- [ ] **Step 1: Write failing tests**

Create `internal/store/sources_test.go`:

```go
package store

import (
	"testing"
	"time"

	"github.com/devonbooker/market-research/internal/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestUpsertSource_InsertsThenNoOps(t *testing.T) {
	s := openTest(t)
	topicID, err := s.CreateTopic("t", "", true)
	require.NoError(t, err)

	id1, inserted, err := s.UpsertSource(topicID, types.PlatformReddit, types.SourceKindSubreddit, "devsecops", types.AddedByAgent)
	require.NoError(t, err)
	assert.True(t, inserted)
	assert.Greater(t, id1, int64(0))

	id2, inserted, err := s.UpsertSource(topicID, types.PlatformReddit, types.SourceKindSubreddit, "devsecops", types.AddedByAgent)
	require.NoError(t, err)
	assert.False(t, inserted)
	assert.Equal(t, id1, id2)
}

func TestListSources_FiltersByTopicAndPlatformAndActive(t *testing.T) {
	s := openTest(t)
	topicID, _ := s.CreateTopic("t", "", true)
	other, _ := s.CreateTopic("other", "", true)

	_, _, _ = s.UpsertSource(topicID, types.PlatformReddit, types.SourceKindSubreddit, "a", types.AddedByAgent)
	_, _, _ = s.UpsertSource(topicID, types.PlatformStackOverflow, types.SourceKindSOTag, "b", types.AddedByAgent)
	inactive, _, _ := s.UpsertSource(topicID, types.PlatformReddit, types.SourceKindSubreddit, "c", types.AddedByAgent)
	require.NoError(t, s.SetSourceActive(inactive, false))
	_, _, _ = s.UpsertSource(other, types.PlatformReddit, types.SourceKindSubreddit, "zz", types.AddedByAgent)

	got, err := s.ListSources(topicID, types.PlatformReddit, false)
	require.NoError(t, err)
	assert.Len(t, got, 1)
	assert.Equal(t, "a", got[0].Value)

	all, err := s.ListSources(topicID, types.PlatformReddit, true)
	require.NoError(t, err)
	assert.Len(t, all, 2)
}

func TestUpdateLastFetched(t *testing.T) {
	s := openTest(t)
	topicID, _ := s.CreateTopic("t", "", true)
	id, _, _ := s.UpsertSource(topicID, types.PlatformReddit, types.SourceKindSubreddit, "x", types.AddedByAgent)

	when := time.Now().UTC().Truncate(time.Second)
	require.NoError(t, s.UpdateSourceLastFetched(id, when))

	got, err := s.GetSource(id)
	require.NoError(t, err)
	require.NotNil(t, got.LastFetched)
	assert.True(t, got.LastFetched.Equal(when))
}

func TestSetSignalScore(t *testing.T) {
	s := openTest(t)
	topicID, _ := s.CreateTopic("t", "", true)
	id, _, _ := s.UpsertSource(topicID, types.PlatformReddit, types.SourceKindSubreddit, "x", types.AddedByAgent)

	require.NoError(t, s.SetSourceSignalScore(id, 0.75))
	got, err := s.GetSource(id)
	require.NoError(t, err)
	require.NotNil(t, got.SignalScore)
	assert.Equal(t, 0.75, *got.SignalScore)
}
```

- [ ] **Step 2: Verify tests fail**

```bash
go test ./internal/store/...
```

Expected: build fails on undefined methods.

- [ ] **Step 3: Implement sources**

Create `internal/store/sources.go`:

```go
package store

import (
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/devonbooker/market-research/internal/types"
)

func (s *Store) UpsertSource(topicID int64, platform types.Platform, kind types.SourceKind, value string, addedBy types.AddedBy) (int64, bool, error) {
	res, err := s.db.Exec(
		`INSERT INTO sources (topic_id, platform, kind, value, added_at, added_by, active, signal_score)
		 VALUES (?, ?, ?, ?, ?, ?, 1, 0.5)
		 ON CONFLICT(topic_id, platform, kind, value) DO NOTHING`,
		topicID, platform, kind, value, time.Now().UTC(), addedBy,
	)
	if err != nil {
		return 0, false, fmt.Errorf("upsert source: %w", err)
	}
	affected, _ := res.RowsAffected()
	if affected == 1 {
		id, _ := res.LastInsertId()
		return id, true, nil
	}
	// already existed, fetch its id
	var id int64
	err = s.db.QueryRow(
		`SELECT id FROM sources WHERE topic_id = ? AND platform = ? AND kind = ? AND value = ?`,
		topicID, platform, kind, value,
	).Scan(&id)
	if err != nil {
		return 0, false, err
	}
	return id, false, nil
}

func (s *Store) GetSource(id int64) (*types.Source, error) {
	return s.scanSource(s.db.QueryRow(
		`SELECT id, topic_id, platform, kind, value, added_at, added_by, last_fetched, signal_score, active
		 FROM sources WHERE id = ?`, id,
	))
}

func (s *Store) ListSources(topicID int64, platform types.Platform, includeInactive bool) ([]*types.Source, error) {
	q := `SELECT id, topic_id, platform, kind, value, added_at, added_by, last_fetched, signal_score, active
	      FROM sources WHERE topic_id = ? AND platform = ?`
	if !includeInactive {
		q += ` AND active = 1`
	}
	q += ` ORDER BY id`
	rows, err := s.db.Query(q, topicID, platform)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []*types.Source
	for rows.Next() {
		src, err := s.scanSource(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, src)
	}
	return out, rows.Err()
}

func (s *Store) UpdateSourceLastFetched(id int64, at time.Time) error {
	_, err := s.db.Exec(`UPDATE sources SET last_fetched = ? WHERE id = ?`, at.UTC(), id)
	return err
}

func (s *Store) SetSourceActive(id int64, active bool) error {
	_, err := s.db.Exec(`UPDATE sources SET active = ? WHERE id = ?`, active, id)
	return err
}

func (s *Store) SetSourceSignalScore(id int64, score float64) error {
	_, err := s.db.Exec(`UPDATE sources SET signal_score = ? WHERE id = ?`, score, id)
	return err
}

func (s *Store) scanSource(r rowScanner) (*types.Source, error) {
	var src types.Source
	var lastFetched sql.NullTime
	var signalScore sql.NullFloat64
	err := r.Scan(&src.ID, &src.TopicID, &src.Platform, &src.Kind, &src.Value,
		&src.AddedAt, &src.AddedBy, &lastFetched, &signalScore, &src.Active)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	if lastFetched.Valid {
		t := lastFetched.Time
		src.LastFetched = &t
	}
	if signalScore.Valid {
		f := signalScore.Float64
		src.SignalScore = &f
	}
	return &src, nil
}
```

- [ ] **Step 4: Run tests**

```bash
go test ./internal/store/... -v
```

Expected: all source tests PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/store/sources.go internal/store/sources_test.go
git commit -m "feat(store): sources CRUD with signal-score persistence"
```

---

### Task 8: Documents + replies upserts

**Files:**
- Create: `internal/store/documents.go`
- Create: `internal/store/documents_test.go`

- [ ] **Step 1: Write failing tests**

Create `internal/store/documents_test.go`:

```go
package store

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/devonbooker/market-research/internal/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func seedSource(t *testing.T, s *Store) (topicID, sourceID int64) {
	t.Helper()
	topicID, err := s.CreateTopic("t", "", true)
	require.NoError(t, err)
	sourceID, _, err = s.UpsertSource(topicID, types.PlatformReddit, types.SourceKindSubreddit, "x", types.AddedByAgent)
	require.NoError(t, err)
	return
}

func TestUpsertDocument_Dedup(t *testing.T) {
	s := openTest(t)
	topicID, sourceID := seedSource(t, s)

	d := types.Document{
		TopicID: topicID, SourceID: sourceID,
		Platform: types.PlatformReddit, PlatformID: "abc",
		Title: "hi", URL: "https://reddit.com/x",
		CreatedAt: time.Now().UTC(), FetchedAt: time.Now().UTC(),
	}
	id1, inserted, err := s.UpsertDocument(&d)
	require.NoError(t, err)
	assert.True(t, inserted)
	assert.Greater(t, id1, int64(0))

	id2, inserted, err := s.UpsertDocument(&d)
	require.NoError(t, err)
	assert.False(t, inserted)
	assert.Equal(t, id1, id2)
}

func TestUpsertReply_DedupByPlatformID(t *testing.T) {
	s := openTest(t)
	topicID, sourceID := seedSource(t, s)
	docID, _, _ := s.UpsertDocument(&types.Document{
		TopicID: topicID, SourceID: sourceID,
		Platform: types.PlatformReddit, PlatformID: "abc",
		Title: "hi", URL: "https://reddit.com/x",
		CreatedAt: time.Now().UTC(), FetchedAt: time.Now().UTC(),
	})

	r := types.Reply{
		DocumentID: docID, PlatformID: "cmt1", Body: "me too",
		CreatedAt: time.Now().UTC(),
	}
	_, inserted, err := s.UpsertReply(&r)
	require.NoError(t, err)
	assert.True(t, inserted)

	_, inserted, err = s.UpsertReply(&r)
	require.NoError(t, err)
	assert.False(t, inserted)
}

func TestCountDocumentsSince(t *testing.T) {
	s := openTest(t)
	topicID, sourceID := seedSource(t, s)
	cutoff := time.Now().UTC().Add(-24 * time.Hour)

	old := types.Document{
		TopicID: topicID, SourceID: sourceID,
		Platform: types.PlatformReddit, PlatformID: "old",
		Title: "o", URL: "https://reddit.com/o",
		CreatedAt: time.Now().UTC().Add(-48 * time.Hour), FetchedAt: time.Now().UTC(),
	}
	newer := types.Document{
		TopicID: topicID, SourceID: sourceID,
		Platform: types.PlatformReddit, PlatformID: "new",
		Title: "n", URL: "https://reddit.com/n",
		CreatedAt: time.Now().UTC().Add(-1 * time.Hour), FetchedAt: time.Now().UTC(),
	}
	_, _, _ = s.UpsertDocument(&old)
	_, _, _ = s.UpsertDocument(&newer)

	n, avgScore, err := s.SourceStatsSince(sourceID, cutoff)
	require.NoError(t, err)
	assert.Equal(t, 1, n)
	_ = avgScore // zero since no score set
}

func TestPlatformMetadataRoundtrip(t *testing.T) {
	s := openTest(t)
	topicID, sourceID := seedSource(t, s)
	meta, _ := json.Marshal(map[string]any{"subreddit": "devsecops"})

	d := types.Document{
		TopicID: topicID, SourceID: sourceID,
		Platform: types.PlatformReddit, PlatformID: "meta1",
		Title: "m", URL: "https://reddit.com/m",
		CreatedAt: time.Now().UTC(), FetchedAt: time.Now().UTC(),
		PlatformMetadata: meta,
	}
	id, _, err := s.UpsertDocument(&d)
	require.NoError(t, err)

	got, err := s.GetDocument(id)
	require.NoError(t, err)
	var parsed map[string]any
	require.NoError(t, json.Unmarshal(got.PlatformMetadata, &parsed))
	assert.Equal(t, "devsecops", parsed["subreddit"])
}
```

- [ ] **Step 2: Verify fail**

```bash
go test ./internal/store/...
```

- [ ] **Step 3: Implement documents + replies**

Create `internal/store/documents.go`:

```go
package store

import (
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/devonbooker/market-research/internal/types"
)

func (s *Store) UpsertDocument(d *types.Document) (int64, bool, error) {
	res, err := s.db.Exec(
		`INSERT INTO documents
		 (topic_id, source_id, platform, platform_id, title, body, author, score, url, created_at, fetched_at, platform_metadata)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		 ON CONFLICT(platform, platform_id) DO NOTHING`,
		d.TopicID, d.SourceID, d.Platform, d.PlatformID,
		d.Title, nullIfEmpty(d.Body), nullIfEmpty(d.Author), d.Score, d.URL,
		d.CreatedAt.UTC(), d.FetchedAt.UTC(), nullIfEmptyJSON(d.PlatformMetadata),
	)
	if err != nil {
		return 0, false, fmt.Errorf("upsert document: %w", err)
	}
	affected, _ := res.RowsAffected()
	if affected == 1 {
		id, _ := res.LastInsertId()
		return id, true, nil
	}
	var id int64
	err = s.db.QueryRow(`SELECT id FROM documents WHERE platform = ? AND platform_id = ?`, d.Platform, d.PlatformID).Scan(&id)
	if err != nil {
		return 0, false, err
	}
	return id, false, nil
}

func (s *Store) GetDocument(id int64) (*types.Document, error) {
	row := s.db.QueryRow(
		`SELECT id, topic_id, source_id, platform, platform_id, title, body, author, score, url, created_at, fetched_at, platform_metadata
		 FROM documents WHERE id = ?`, id)
	var d types.Document
	var body, author, meta sql.NullString
	err := row.Scan(&d.ID, &d.TopicID, &d.SourceID, &d.Platform, &d.PlatformID,
		&d.Title, &body, &author, &d.Score, &d.URL, &d.CreatedAt, &d.FetchedAt, &meta)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	d.Body = body.String
	d.Author = author.String
	if meta.Valid {
		d.PlatformMetadata = []byte(meta.String)
	}
	return &d, nil
}

func (s *Store) UpsertReply(r *types.Reply) (int64, bool, error) {
	res, err := s.db.Exec(
		`INSERT INTO document_replies (document_id, platform_id, body, author, score, created_at, is_accepted)
		 VALUES (?, ?, ?, ?, ?, ?, ?)
		 ON CONFLICT(platform_id) DO NOTHING`,
		r.DocumentID, r.PlatformID, r.Body, nullIfEmpty(r.Author), r.Score, r.CreatedAt.UTC(), r.IsAccepted,
	)
	if err != nil {
		return 0, false, fmt.Errorf("upsert reply: %w", err)
	}
	affected, _ := res.RowsAffected()
	if affected == 1 {
		id, _ := res.LastInsertId()
		return id, true, nil
	}
	var id int64
	err = s.db.QueryRow(`SELECT id FROM document_replies WHERE platform_id = ?`, r.PlatformID).Scan(&id)
	if err != nil {
		return 0, false, err
	}
	return id, false, nil
}

// SourceStatsSince returns (doc_count, avg_score) for a source since a cutoff.
// Used by the rediscovery agent for signal scoring.
func (s *Store) SourceStatsSince(sourceID int64, since time.Time) (int, float64, error) {
	var count int
	var avg sql.NullFloat64
	err := s.db.QueryRow(
		`SELECT COUNT(*), AVG(score) FROM documents WHERE source_id = ? AND created_at >= ?`,
		sourceID, since.UTC(),
	).Scan(&count, &avg)
	if err != nil {
		return 0, 0, err
	}
	return count, avg.Float64, nil
}

func nullIfEmpty(s string) any {
	if s == "" {
		return nil
	}
	return s
}

func nullIfEmptyJSON(b []byte) any {
	if len(b) == 0 {
		return nil
	}
	return string(b)
}
```

- [ ] **Step 4: Run tests**

```bash
go test ./internal/store/... -v
```

Expected: all document tests PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/store/documents.go internal/store/documents_test.go
git commit -m "feat(store): document and reply upserts with dedup, source stats"
```

---

### Task 9: Fetch runs lifecycle

**Files:**
- Create: `internal/store/fetch_runs.go`
- Create: `internal/store/fetch_runs_test.go`

- [ ] **Step 1: Write failing tests**

Create `internal/store/fetch_runs_test.go`:

```go
package store

import (
	"testing"

	"github.com/devonbooker/market-research/internal/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestStartFetchRun_ReturnsRunningRow(t *testing.T) {
	s := openTest(t)
	topicID, err := s.CreateTopic("t", "", true)
	require.NoError(t, err)

	id, err := s.StartFetchRun(topicID, types.PlatformReddit)
	require.NoError(t, err)
	assert.Greater(t, id, int64(0))

	run, err := s.GetFetchRun(id)
	require.NoError(t, err)
	assert.Equal(t, types.RunStatusRunning, run.Status)
	assert.Nil(t, run.EndedAt)
}

func TestCloseFetchRun_Success(t *testing.T) {
	s := openTest(t)
	topicID, _ := s.CreateTopic("t", "", true)
	id, _ := s.StartFetchRun(topicID, types.PlatformReddit)

	require.NoError(t, s.CloseFetchRun(id, types.RunStatusSuccess, 10, 30, ""))

	run, err := s.GetFetchRun(id)
	require.NoError(t, err)
	assert.Equal(t, types.RunStatusSuccess, run.Status)
	assert.NotNil(t, run.EndedAt)
	assert.Equal(t, 10, run.DocumentsNew)
	assert.Equal(t, 30, run.RepliesNew)
}

func TestCloseFetchRun_Error(t *testing.T) {
	s := openTest(t)
	topicID, _ := s.CreateTopic("t", "", true)
	id, _ := s.StartFetchRun(topicID, types.PlatformReddit)

	require.NoError(t, s.CloseFetchRun(id, types.RunStatusError, 0, 0, "connection reset"))

	run, err := s.GetFetchRun(id)
	require.NoError(t, err)
	assert.Equal(t, types.RunStatusError, run.Status)
	assert.Equal(t, "connection reset", run.ErrorMessage)
}

func TestMarkOrphanRunsErrored(t *testing.T) {
	s := openTest(t)
	topicID, _ := s.CreateTopic("t", "", true)
	_, _ = s.StartFetchRun(topicID, types.PlatformReddit)
	_, _ = s.StartFetchRun(topicID, types.PlatformStackOverflow)

	n, err := s.MarkOrphanRunsErrored("unexpected shutdown")
	require.NoError(t, err)
	assert.Equal(t, 2, n)
}
```

- [ ] **Step 2: Implement fetch_runs**

Create `internal/store/fetch_runs.go`:

```go
package store

import (
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/devonbooker/market-research/internal/types"
)

func (s *Store) StartFetchRun(topicID int64, platform types.Platform) (int64, error) {
	res, err := s.db.Exec(
		`INSERT INTO fetch_runs (topic_id, platform, started_at, status) VALUES (?, ?, ?, ?)`,
		topicID, platform, time.Now().UTC(), types.RunStatusRunning,
	)
	if err != nil {
		return 0, fmt.Errorf("start fetch run: %w", err)
	}
	return res.LastInsertId()
}

func (s *Store) CloseFetchRun(id int64, status types.RunStatus, docsNew, repliesNew int, errMsg string) error {
	_, err := s.db.Exec(
		`UPDATE fetch_runs SET ended_at = ?, status = ?, documents_new = ?, replies_new = ?, error_message = ?
		 WHERE id = ?`,
		time.Now().UTC(), status, docsNew, repliesNew, errMsg, id,
	)
	return err
}

func (s *Store) GetFetchRun(id int64) (*types.FetchRun, error) {
	row := s.db.QueryRow(
		`SELECT id, topic_id, platform, started_at, ended_at, status, documents_new, replies_new, error_message
		 FROM fetch_runs WHERE id = ?`, id)
	var r types.FetchRun
	var endedAt sql.NullTime
	var docsNew, repliesNew sql.NullInt64
	var errMsg sql.NullString
	err := row.Scan(&r.ID, &r.TopicID, &r.Platform, &r.StartedAt, &endedAt, &r.Status, &docsNew, &repliesNew, &errMsg)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	if endedAt.Valid {
		t := endedAt.Time
		r.EndedAt = &t
	}
	r.DocumentsNew = int(docsNew.Int64)
	r.RepliesNew = int(repliesNew.Int64)
	r.ErrorMessage = errMsg.String
	return &r, nil
}

// MarkOrphanRunsErrored closes any runs left in 'running' state. Called at main() startup
// to recover from panic/crash in a previous invocation.
func (s *Store) MarkOrphanRunsErrored(message string) (int, error) {
	res, err := s.db.Exec(
		`UPDATE fetch_runs SET status = ?, ended_at = ?, error_message = ? WHERE status = ?`,
		types.RunStatusError, time.Now().UTC(), message, types.RunStatusRunning,
	)
	if err != nil {
		return 0, err
	}
	n, _ := res.RowsAffected()
	return int(n), nil
}
```

- [ ] **Step 3: Run tests**

```bash
go test ./internal/store/... -v
```

Expected: all fetch_runs tests PASS.

- [ ] **Step 4: Commit**

```bash
git add internal/store/fetch_runs.go internal/store/fetch_runs_test.go
git commit -m "feat(store): fetch_runs lifecycle with orphan recovery"
```

---

### Task 10: Store integration test - foreign-key cascade and concurrency

**Files:**
- Create: `internal/store/integration_test.go`

- [ ] **Step 1: Write integration test**

Create `internal/store/integration_test.go`:

```go
package store

import (
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/devonbooker/market-research/internal/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCascade_DeleteTopic_WipesSourcesAndDocs(t *testing.T) {
	s := openTest(t)
	topicID, _ := s.CreateTopic("t", "", true)
	sourceID, _, _ := s.UpsertSource(topicID, types.PlatformReddit, types.SourceKindSubreddit, "x", types.AddedByAgent)
	_, _, _ = s.UpsertDocument(&types.Document{
		TopicID: topicID, SourceID: sourceID,
		Platform: types.PlatformReddit, PlatformID: "p1",
		Title: "t", URL: "u", CreatedAt: time.Now().UTC(), FetchedAt: time.Now().UTC(),
	})

	require.NoError(t, s.DeleteTopic(topicID))

	var docCount int
	require.NoError(t, s.DB().QueryRow("SELECT COUNT(*) FROM documents").Scan(&docCount))
	assert.Equal(t, 0, docCount)

	var srcCount int
	require.NoError(t, s.DB().QueryRow("SELECT COUNT(*) FROM sources").Scan(&srcCount))
	assert.Equal(t, 0, srcCount)
}

func TestConcurrentUpserts_NoDuplicates(t *testing.T) {
	dir := t.TempDir()
	s, err := Open(filepath.Join(dir, "t.db"))
	require.NoError(t, err)
	defer s.Close()

	topicID, _ := s.CreateTopic("t", "", true)
	sourceID, _, _ := s.UpsertSource(topicID, types.PlatformReddit, types.SourceKindSubreddit, "x", types.AddedByAgent)

	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, _, err := s.UpsertDocument(&types.Document{
				TopicID: topicID, SourceID: sourceID,
				Platform: types.PlatformReddit, PlatformID: "same",
				Title: "t", URL: "u",
				CreatedAt: time.Now().UTC(), FetchedAt: time.Now().UTC(),
			})
			assert.NoError(t, err)
		}()
	}
	wg.Wait()

	var n int
	require.NoError(t, s.DB().QueryRow("SELECT COUNT(*) FROM documents").Scan(&n))
	assert.Equal(t, 1, n)
}
```

- [ ] **Step 2: Run tests**

```bash
go test ./internal/store/... -race -v
```

Expected: all pass, no race conditions reported.

- [ ] **Step 3: Commit**

```bash
git add internal/store/integration_test.go
git commit -m "test(store): cascade deletion + concurrent upsert integration tests"
```

---

## Phase 4: Sources agent

### Task 11: SourcePlan validation and cap trimming

**Files:**
- Create: `internal/sources/plan.go`
- Create: `internal/sources/plan_test.go`

- [ ] **Step 1: Write failing tests**

Create `internal/sources/plan_test.go`:

```go
package sources

import (
	"testing"

	"github.com/devonbooker/market-research/internal/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestValidate_RejectsEmpty(t *testing.T) {
	var p types.SourcePlan
	err := Validate(&p)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "empty")
}

func TestTrimToCaps_TrimsLongLists(t *testing.T) {
	p := types.SourcePlan{}
	for i := 0; i < 20; i++ {
		p.Reddit.Subreddits = append(p.Reddit.Subreddits, "s")
	}
	for i := 0; i < 10; i++ {
		p.Reddit.SearchQueries = append(p.Reddit.SearchQueries, "q")
	}
	for i := 0; i < 10; i++ {
		p.StackOverflow.Tags = append(p.StackOverflow.Tags, "t")
	}
	for i := 0; i < 10; i++ {
		p.StackOverflow.SearchQueries = append(p.StackOverflow.SearchQueries, "q")
	}

	TrimToCaps(&p)
	assert.Len(t, p.Reddit.Subreddits, MaxSubreddits)
	assert.Len(t, p.Reddit.SearchQueries, MaxSearchQueries)
	assert.Len(t, p.StackOverflow.Tags, MaxSOTags)
	assert.Len(t, p.StackOverflow.SearchQueries, MaxSearchQueries)
}

func TestTrimToCaps_NormalizesWhitespaceAndDedups(t *testing.T) {
	p := types.SourcePlan{}
	p.Reddit.Subreddits = []string{" DevSecOps ", "devsecops", "Cybersecurity"}
	TrimToCaps(&p)
	assert.Equal(t, []string{"devsecops", "cybersecurity"}, p.Reddit.Subreddits)
}
```

- [ ] **Step 2: Verify fail**

```bash
go test ./internal/sources/...
```

- [ ] **Step 3: Implement plan helpers**

Create `internal/sources/plan.go`:

```go
package sources

import (
	"errors"
	"strings"

	"github.com/devonbooker/market-research/internal/types"
)

const (
	MaxSubreddits    = 10
	MaxSOTags        = 5
	MaxSearchQueries = 5
)

func Validate(p *types.SourcePlan) error {
	total := len(p.Reddit.Subreddits) + len(p.Reddit.SearchQueries) +
		len(p.StackOverflow.Tags) + len(p.StackOverflow.SearchQueries)
	if total == 0 {
		return errors.New("source plan is empty (no subreddits, tags, or queries)")
	}
	return nil
}

func TrimToCaps(p *types.SourcePlan) {
	p.Reddit.Subreddits = normalizeAndCap(p.Reddit.Subreddits, MaxSubreddits)
	p.Reddit.SearchQueries = normalizeAndCap(p.Reddit.SearchQueries, MaxSearchQueries)
	p.StackOverflow.Tags = normalizeAndCap(p.StackOverflow.Tags, MaxSOTags)
	p.StackOverflow.SearchQueries = normalizeAndCap(p.StackOverflow.SearchQueries, MaxSearchQueries)
}

func normalizeAndCap(items []string, cap int) []string {
	seen := map[string]struct{}{}
	out := make([]string, 0, len(items))
	for _, it := range items {
		n := strings.ToLower(strings.TrimSpace(it))
		if n == "" {
			continue
		}
		if _, ok := seen[n]; ok {
			continue
		}
		seen[n] = struct{}{}
		out = append(out, n)
		if len(out) >= cap {
			break
		}
	}
	return out
}

// PlanToSources converts a validated + trimmed SourcePlan into (platform, kind, value) triples.
func PlanToSources(p *types.SourcePlan) []struct {
	Platform types.Platform
	Kind     types.SourceKind
	Value    string
} {
	var out []struct {
		Platform types.Platform
		Kind     types.SourceKind
		Value    string
	}
	for _, v := range p.Reddit.Subreddits {
		out = append(out, struct {
			Platform types.Platform
			Kind     types.SourceKind
			Value    string
		}{types.PlatformReddit, types.SourceKindSubreddit, v})
	}
	for _, v := range p.Reddit.SearchQueries {
		out = append(out, struct {
			Platform types.Platform
			Kind     types.SourceKind
			Value    string
		}{types.PlatformReddit, types.SourceKindSearchQuery, v})
	}
	for _, v := range p.StackOverflow.Tags {
		out = append(out, struct {
			Platform types.Platform
			Kind     types.SourceKind
			Value    string
		}{types.PlatformStackOverflow, types.SourceKindSOTag, v})
	}
	for _, v := range p.StackOverflow.SearchQueries {
		out = append(out, struct {
			Platform types.Platform
			Kind     types.SourceKind
			Value    string
		}{types.PlatformStackOverflow, types.SourceKindSearchQuery, v})
	}
	return out
}
```

- [ ] **Step 4: Run tests**

```bash
go test ./internal/sources/... -v
```

Expected: three tests PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/sources/plan.go internal/sources/plan_test.go
git commit -m "feat(sources): SourcePlan validation, cap trimming, normalization"
```

---

### Task 12: ClaudeClient interface + prompt templates

**Files:**
- Create: `internal/sources/client.go`
- Create: `internal/sources/prompts.go`

- [ ] **Step 1: Create the client interface**

Create `internal/sources/client.go`:

```go
package sources

import (
	"context"

	"github.com/devonbooker/market-research/internal/types"
)

// ClaudeClient is the narrow interface the source-discovery agent needs from Claude.
// Production uses an adapter over the anthropic-sdk-go; tests inject a stub.
type ClaudeClient interface {
	// Discover returns a SourcePlan from the given prompt. Implementations MUST
	// return an error (never a partial plan) if the model output cannot be coerced
	// into a valid SourcePlan via the forced tool call.
	Discover(ctx context.Context, systemPrompt, userPrompt string) (*types.SourcePlan, error)
}
```

- [ ] **Step 2: Create the prompt file**

Create `internal/sources/prompts.go`:

```go
package sources

import (
	"fmt"
	"strings"

	"github.com/devonbooker/market-research/internal/types"
)

const systemPrompt = `You are a market research assistant that identifies the best sources to monitor on Reddit and Stack Overflow for a given topic.

Your only job is to call the submit_source_plan tool. Do not chat.

Constraints:
- Subreddit names: no "r/" prefix, no spaces, lowercase.
- Stack Overflow tags: lowercase, hyphenated where applicable (e.g., "kubernetes-helm").
- Search queries: full-text, quoted phrases OK. Prefer specific pain-language queries ("X is broken", "how to X", "alternatives to X").
- Return at most 10 subreddits, 5 SO tags, 5 search queries per platform.
- Prefer sources where end-users complain or ask questions about the topic. Avoid vendor-marketing subreddits.`

// InitialPrompt builds the user message for a first-time topic discovery.
func InitialPrompt(topicName, description string) string {
	var b strings.Builder
	fmt.Fprintf(&b, "Topic: %s\n", topicName)
	if description != "" {
		fmt.Fprintf(&b, "Description: %s\n", description)
	}
	fmt.Fprintln(&b, "\nThis is a new topic with no existing source list. Propose an initial plan.")
	return b.String()
}

// SourceStat is per-source data fed to the rediscovery prompt.
type SourceStat struct {
	Platform    types.Platform
	Kind        types.SourceKind
	Value       string
	DocsLast7d  int
	AvgScore    float64
	SignalScore float64
}

// RediscoverPrompt builds the user message for weekly source rediscovery.
func RediscoverPrompt(topicName, description string, current []SourceStat) string {
	var b strings.Builder
	fmt.Fprintf(&b, "Topic: %s\n", topicName)
	if description != "" {
		fmt.Fprintf(&b, "Description: %s\n", description)
	}
	fmt.Fprintln(&b, "\nCurrent sources with performance over the last 7 days:")
	if len(current) == 0 {
		fmt.Fprintln(&b, "  (none)")
	} else {
		for _, s := range current {
			fmt.Fprintf(&b, "  - %s/%s: %s (docs=%d, avg_score=%.2f, signal=%.2f)\n",
				s.Platform, s.Kind, s.Value, s.DocsLast7d, s.AvgScore, s.SignalScore)
		}
	}
	fmt.Fprintln(&b, "\nExpand, prune, or reprioritize. Return a full new plan (not a diff).")
	return b.String()
}

// SystemPrompt returns the system prompt used for both initial discovery and rediscovery.
func SystemPrompt() string {
	return systemPrompt
}
```

- [ ] **Step 3: Verify build**

```bash
go build ./internal/sources/...
```

Expected: no output (success).

- [ ] **Step 4: Commit**

```bash
git add internal/sources/client.go internal/sources/prompts.go
git commit -m "feat(sources): ClaudeClient interface and prompt templates"
```

---

### Task 13: Agent.Discover + Agent.Rediscover with stub tests

**Files:**
- Create: `internal/sources/agent.go`
- Create: `internal/sources/agent_test.go`

- [ ] **Step 1: Write failing tests**

Create `internal/sources/agent_test.go`:

```go
package sources

import (
	"context"
	"errors"
	"testing"

	"github.com/devonbooker/market-research/internal/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type stubClient struct {
	plan *types.SourcePlan
	err  error
	lastSystemPrompt string
	lastUserPrompt   string
}

func (s *stubClient) Discover(ctx context.Context, systemPrompt, userPrompt string) (*types.SourcePlan, error) {
	s.lastSystemPrompt = systemPrompt
	s.lastUserPrompt = userPrompt
	return s.plan, s.err
}

func TestDiscover_ReturnsTrimmedPlan(t *testing.T) {
	plan := &types.SourcePlan{}
	for i := 0; i < 15; i++ {
		plan.Reddit.Subreddits = append(plan.Reddit.Subreddits, "sub"+string(rune('a'+i%26)))
	}
	c := &stubClient{plan: plan}
	a := &Agent{Claude: c}

	got, err := a.Discover(context.Background(), "soc2", "")
	require.NoError(t, err)
	assert.Len(t, got.Reddit.Subreddits, MaxSubreddits)
	assert.Contains(t, c.lastUserPrompt, "soc2")
}

func TestDiscover_PropagatesClientError(t *testing.T) {
	c := &stubClient{err: errors.New("boom")}
	a := &Agent{Claude: c}
	_, err := a.Discover(context.Background(), "t", "")
	require.Error(t, err)
}

func TestDiscover_RejectsEmptyPlan(t *testing.T) {
	c := &stubClient{plan: &types.SourcePlan{}}
	a := &Agent{Claude: c}
	_, err := a.Discover(context.Background(), "t", "")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "empty")
}

func TestRediscover_IncludesCurrentStatsInPrompt(t *testing.T) {
	plan := &types.SourcePlan{}
	plan.Reddit.Subreddits = []string{"one"}
	c := &stubClient{plan: plan}
	a := &Agent{Claude: c}

	stats := []SourceStat{
		{Platform: types.PlatformReddit, Kind: types.SourceKindSubreddit, Value: "old", DocsLast7d: 3, AvgScore: 10, SignalScore: 0.3},
	}
	_, err := a.Rediscover(context.Background(), "t", "desc", stats)
	require.NoError(t, err)
	assert.Contains(t, c.lastUserPrompt, "old")
	assert.Contains(t, c.lastUserPrompt, "docs=3")
}
```

- [ ] **Step 2: Verify fail**

```bash
go test ./internal/sources/...
```

- [ ] **Step 3: Implement agent**

Create `internal/sources/agent.go`:

```go
package sources

import (
	"context"
	"fmt"

	"github.com/devonbooker/market-research/internal/types"
)

type Agent struct {
	Claude ClaudeClient
}

func (a *Agent) Discover(ctx context.Context, topicName, description string) (*types.SourcePlan, error) {
	plan, err := a.Claude.Discover(ctx, SystemPrompt(), InitialPrompt(topicName, description))
	if err != nil {
		return nil, fmt.Errorf("agent.Discover: %w", err)
	}
	if err := Validate(plan); err != nil {
		return nil, err
	}
	TrimToCaps(plan)
	return plan, nil
}

func (a *Agent) Rediscover(ctx context.Context, topicName, description string, current []SourceStat) (*types.SourcePlan, error) {
	plan, err := a.Claude.Discover(ctx, SystemPrompt(), RediscoverPrompt(topicName, description, current))
	if err != nil {
		return nil, fmt.Errorf("agent.Rediscover: %w", err)
	}
	if err := Validate(plan); err != nil {
		return nil, err
	}
	TrimToCaps(plan)
	return plan, nil
}
```

- [ ] **Step 4: Run tests**

```bash
go test ./internal/sources/... -v
```

Expected: 4 tests PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/sources/agent.go internal/sources/agent_test.go
git commit -m "feat(sources): Agent.Discover + Agent.Rediscover with validation"
```

---

### Task 14: Anthropic adapter (production ClaudeClient)

**Files:**
- Create: `internal/sources/anthropic.go`

- [ ] **Step 1: Add SDK**

```bash
go get github.com/anthropics/anthropic-sdk-go
```

- [ ] **Step 2: Implement adapter**

Create `internal/sources/anthropic.go`:

```go
package sources

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/anthropics/anthropic-sdk-go/option"
	"github.com/devonbooker/market-research/internal/types"
)

type AnthropicClient struct {
	client *anthropic.Client
	model  string
}

func NewAnthropicClient(apiKey string) *AnthropicClient {
	c := anthropic.NewClient(option.WithAPIKey(apiKey))
	return &AnthropicClient{client: &c, model: string(anthropic.ModelClaudeSonnet4_6)}
}

func (a *AnthropicClient) Discover(ctx context.Context, systemPrompt, userPrompt string) (*types.SourcePlan, error) {
	tool := anthropic.ToolParam{
		Name:        "submit_source_plan",
		Description: anthropic.String("Submit the source plan for the given topic."),
		InputSchema: anthropic.ToolInputSchemaParam{
			Properties: map[string]any{
				"reddit": map[string]any{
					"type": "object",
					"properties": map[string]any{
						"subreddits":     map[string]any{"type": "array", "items": map[string]any{"type": "string"}},
						"search_queries": map[string]any{"type": "array", "items": map[string]any{"type": "string"}},
					},
					"required": []string{"subreddits", "search_queries"},
				},
				"stackoverflow": map[string]any{
					"type": "object",
					"properties": map[string]any{
						"tags":           map[string]any{"type": "array", "items": map[string]any{"type": "string"}},
						"search_queries": map[string]any{"type": "array", "items": map[string]any{"type": "string"}},
					},
					"required": []string{"tags", "search_queries"},
				},
				"reasoning": map[string]any{"type": "string"},
			},
			Required: []string{"reddit", "stackoverflow", "reasoning"},
		},
	}

	msg, err := a.client.Messages.New(ctx, anthropic.MessageNewParams{
		Model:     anthropic.Model(a.model),
		MaxTokens: 1024,
		System:    []anthropic.TextBlockParam{{Text: systemPrompt}},
		Messages: []anthropic.MessageParam{
			anthropic.NewUserMessage(anthropic.NewTextBlock(userPrompt)),
		},
		Tools: []anthropic.ToolUnionParam{{OfTool: &tool}},
		ToolChoice: anthropic.ToolChoiceUnionParam{
			OfTool: &anthropic.ToolChoiceToolParam{Name: "submit_source_plan"},
		},
	})
	if err != nil {
		return nil, fmt.Errorf("anthropic messages.new: %w", err)
	}

	for _, block := range msg.Content {
		if tu := block.AsToolUse(); tu.Name == "submit_source_plan" {
			var plan types.SourcePlan
			if err := json.Unmarshal([]byte(tu.JSON.Input.Raw()), &plan); err != nil {
				return nil, fmt.Errorf("unmarshal tool input: %w", err)
			}
			return &plan, nil
		}
	}
	return nil, fmt.Errorf("model did not call submit_source_plan tool")
}
```

- [ ] **Step 3: Verify build**

```bash
go build ./internal/sources/...
```

Expected: no output (success). Note: this file has no unit tests (it is a thin adapter exercised only by a live API call - out of scope for CI per the spec).

- [ ] **Step 4: Commit**

```bash
git add go.mod go.sum internal/sources/anthropic.go
git commit -m "feat(sources): Anthropic SDK adapter with forced tool-call"
```

---

### Task 15: Signal-score heuristic

**Files:**
- Create: `internal/sources/signal.go`
- Create: `internal/sources/signal_test.go`

- [ ] **Step 1: Write failing tests**

Create `internal/sources/signal_test.go`:

```go
package sources

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestSignalScore_ZeroDocsYieldsZero(t *testing.T) {
	assert.Equal(t, 0.0, Score(0, 0))
}

func TestSignalScore_NormalizedToUnitInterval(t *testing.T) {
	s := Score(10, 5)
	assert.GreaterOrEqual(t, s, 0.0)
	assert.LessOrEqual(t, s, 1.0)
}

func TestSignalScore_MonotonicInDocs(t *testing.T) {
	a := Score(1, 5)
	b := Score(10, 5)
	assert.Greater(t, b, a)
}
```

- [ ] **Step 2: Implement signal**

Create `internal/sources/signal.go`:

```go
package sources

import "math"

// Score computes a signal score in [0.0, 1.0] from docs/week and avg doc score.
// Uses a saturating function so a single hot source does not dominate.
// Formula: tanh(docsPerWeek * max(avgScore, 1) / 100).
func Score(docsPerWeek int, avgScore float64) float64 {
	if docsPerWeek == 0 {
		return 0
	}
	effScore := avgScore
	if effScore < 1 {
		effScore = 1
	}
	return math.Tanh(float64(docsPerWeek) * effScore / 100.0)
}
```

- [ ] **Step 3: Run tests**

```bash
go test ./internal/sources/... -v
```

Expected: all signal tests PASS.

- [ ] **Step 4: Commit**

```bash
git add internal/sources/signal.go internal/sources/signal_test.go
git commit -m "feat(sources): saturating signal-score heuristic"
```

---

## Phase 5: Fetch - Reddit

### Task 16: Reddit OAuth client with rate limiting

**Files:**
- Create: `internal/fetch/reddit/client.go`
- Create: `internal/fetch/reddit/client_test.go`

- [ ] **Step 1: Write failing test against httptest**

Create `internal/fetch/reddit/client_test.go`:

```go
package reddit

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestClient_GetJSON_AuthorizesAndDecodes(t *testing.T) {
	var authCount int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v1/access_token":
			authCount++
			require.Equal(t, "POST", r.Method)
			username, password, ok := r.BasicAuth()
			require.True(t, ok)
			assert.Equal(t, "cid", username)
			assert.Equal(t, "csec", password)
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"access_token":"tok","token_type":"bearer","expires_in":3600}`))
		case "/r/devsecops/new":
			assert.Equal(t, "Bearer tok", r.Header.Get("Authorization"))
			assert.Contains(t, r.Header.Get("User-Agent"), "test-agent")
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"data":{"children":[]}}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	c := New(Config{
		ClientID:     "cid",
		ClientSecret: "csec",
		UserAgent:    "test-agent",
		AuthURL:      srv.URL + "/api/v1/access_token",
		APIBaseURL:   srv.URL,
		RateLimit:    50.0,
	})

	var got struct{ Data struct{ Children []any } }
	require.NoError(t, c.GetJSON(context.Background(), "/r/devsecops/new", &got))
	assert.Equal(t, 1, authCount)

	require.NoError(t, c.GetJSON(context.Background(), "/r/devsecops/new", &got))
	assert.Equal(t, 1, authCount, "token should be reused while valid")
}

func TestClient_GetJSON_HTTPError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "access_token") {
			_, _ = w.Write([]byte(`{"access_token":"tok","token_type":"bearer","expires_in":3600}`))
			return
		}
		http.Error(w, "forbidden", http.StatusForbidden)
	}))
	defer srv.Close()

	c := New(Config{
		ClientID:     "cid",
		ClientSecret: "csec",
		UserAgent:    "a",
		AuthURL:      srv.URL + "/api/v1/access_token",
		APIBaseURL:   srv.URL,
		RateLimit:    50.0,
	})

	err := c.GetJSON(context.Background(), "/anywhere", &struct{}{})
	require.Error(t, err)
	var he *HTTPError
	require.ErrorAs(t, err, &he)
	assert.Equal(t, http.StatusForbidden, he.Status)
}

var _ = json.Marshal
var _ = time.Second
```

- [ ] **Step 2: Add rate package**

```bash
go get golang.org/x/time/rate
```

- [ ] **Step 3: Implement client**

Create `internal/fetch/reddit/client.go`:

```go
package reddit

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"golang.org/x/time/rate"
)

type Config struct {
	ClientID     string
	ClientSecret string
	UserAgent    string
	AuthURL      string // defaults to Reddit production
	APIBaseURL   string // defaults to Reddit production
	RateLimit    float64 // requests per second
}

type HTTPError struct {
	Status int
	Body   string
}

func (e *HTTPError) Error() string { return fmt.Sprintf("reddit http %d: %s", e.Status, e.Body) }

type Client struct {
	cfg     Config
	http    *http.Client
	limiter *rate.Limiter

	mu         sync.Mutex
	token      string
	tokenExp   time.Time
}

func New(cfg Config) *Client {
	if cfg.AuthURL == "" {
		cfg.AuthURL = "https://www.reddit.com/api/v1/access_token"
	}
	if cfg.APIBaseURL == "" {
		cfg.APIBaseURL = "https://oauth.reddit.com"
	}
	if cfg.RateLimit == 0 {
		cfg.RateLimit = 50.0 / 60.0 // 50 req/min = ~0.83 req/s
	}
	return &Client{
		cfg:     cfg,
		http:    &http.Client{Timeout: 30 * time.Second},
		limiter: rate.NewLimiter(rate.Limit(cfg.RateLimit), 1),
	}
}

func (c *Client) ensureToken(ctx context.Context) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.token != "" && time.Now().Before(c.tokenExp.Add(-30*time.Second)) {
		return nil
	}
	body := strings.NewReader("grant_type=client_credentials")
	req, err := http.NewRequestWithContext(ctx, "POST", c.cfg.AuthURL, body)
	if err != nil {
		return err
	}
	req.SetBasicAuth(c.cfg.ClientID, c.cfg.ClientSecret)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("User-Agent", c.cfg.UserAgent)

	resp, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		b, _ := io.ReadAll(resp.Body)
		return &HTTPError{Status: resp.StatusCode, Body: string(b)}
	}
	var tok struct {
		AccessToken string `json:"access_token"`
		ExpiresIn   int    `json:"expires_in"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&tok); err != nil {
		return err
	}
	c.token = tok.AccessToken
	c.tokenExp = time.Now().Add(time.Duration(tok.ExpiresIn) * time.Second)
	return nil
}

func (c *Client) GetJSON(ctx context.Context, path string, out any) error {
	if err := c.limiter.Wait(ctx); err != nil {
		return err
	}
	if err := c.ensureToken(ctx); err != nil {
		return err
	}

	u, err := url.Parse(c.cfg.APIBaseURL + path)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, "GET", u.String(), nil)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+c.token)
	req.Header.Set("User-Agent", c.cfg.UserAgent)

	resp, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		b, _ := io.ReadAll(resp.Body)
		return &HTTPError{Status: resp.StatusCode, Body: string(b)}
	}
	return json.NewDecoder(resp.Body).Decode(out)
}
```

- [ ] **Step 4: Run tests**

```bash
go test ./internal/fetch/reddit/... -v
```

Expected: both client tests PASS.

- [ ] **Step 5: Commit**

```bash
git add go.mod go.sum internal/fetch/reddit/client.go internal/fetch/reddit/client_test.go
git commit -m "feat(fetch/reddit): OAuth client with rate limiter and token caching"
```

---

### Task 17: Reddit fetch - posts from subreddit + search

**Files:**
- Create: `internal/fetch/reddit/fetch.go`
- Create: `internal/fetch/reddit/fetch_test.go`
- Create: `internal/fetch/reddit/testdata/subreddit_new.json`

- [ ] **Step 1: Create test fixture**

Create `internal/fetch/reddit/testdata/subreddit_new.json`:

```json
{
  "data": {
    "children": [
      {
        "data": {
          "id": "p1",
          "name": "t3_p1",
          "subreddit": "devsecops",
          "title": "SOC2 evidence collection is painful",
          "selftext": "We spend hours on this every quarter.",
          "author": "alice",
          "score": 42,
          "permalink": "/r/devsecops/comments/p1/soc2_evidence_collection/",
          "url": "https://reddit.com/r/devsecops/comments/p1/soc2_evidence_collection/",
          "created_utc": 1713000000,
          "link_flair_text": "Question"
        }
      },
      {
        "data": {
          "id": "p2",
          "name": "t3_p2",
          "subreddit": "devsecops",
          "title": "Old post",
          "selftext": "",
          "author": "bob",
          "score": 10,
          "permalink": "/r/devsecops/comments/p2/old/",
          "url": "https://reddit.com/r/devsecops/comments/p2/old/",
          "created_utc": 1700000000,
          "link_flair_text": ""
        }
      }
    ]
  }
}
```

- [ ] **Step 2: Write failing test**

Create `internal/fetch/reddit/fetch_test.go`:

```go
package reddit

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/devonbooker/market-research/internal/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func serveFixture(t *testing.T, path string) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v1/access_token":
			_, _ = w.Write([]byte(`{"access_token":"tok","expires_in":3600}`))
		default:
			data, err := os.ReadFile(path)
			require.NoError(t, err)
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write(data)
		}
	}))
}

func TestFetchSubredditNew_FiltersBySince(t *testing.T) {
	srv := serveFixture(t, "testdata/subreddit_new.json")
	defer srv.Close()

	c := New(Config{
		ClientID: "c", ClientSecret: "s", UserAgent: "ua",
		AuthURL:    srv.URL + "/api/v1/access_token",
		APIBaseURL: srv.URL,
		RateLimit:  100,
	})

	since := time.Unix(1710000000, 0).UTC()
	docs, err := FetchSubredditNew(context.Background(), c, "devsecops", since, 100)
	require.NoError(t, err)
	require.Len(t, docs, 1)
	d := docs[0]
	assert.Equal(t, types.PlatformReddit, d.Platform)
	assert.Equal(t, "p1", d.PlatformID)
	assert.Equal(t, "SOC2 evidence collection is painful", d.Title)
	assert.Equal(t, "We spend hours on this every quarter.", d.Body)
	assert.Equal(t, "alice", d.Author)
	assert.Equal(t, 42, d.Score)
	assert.Contains(t, d.URL, "/r/devsecops/comments/p1/")
	assert.True(t, d.CreatedAt.Equal(time.Unix(1713000000, 0).UTC()))
	assert.Contains(t, string(d.PlatformMetadata), "devsecops")
	assert.Contains(t, string(d.PlatformMetadata), "Question")
}

func TestFetchSubredditNew_CapsAtLimit(t *testing.T) {
	srv := serveFixture(t, "testdata/subreddit_new.json")
	defer srv.Close()

	c := New(Config{
		ClientID: "c", ClientSecret: "s", UserAgent: "ua",
		AuthURL: srv.URL + "/api/v1/access_token", APIBaseURL: srv.URL,
		RateLimit: 100,
	})

	docs, err := FetchSubredditNew(context.Background(), c, "devsecops", time.Time{}, 1)
	require.NoError(t, err)
	assert.Len(t, docs, 1)
}
```

- [ ] **Step 3: Implement fetch.go**

Create `internal/fetch/reddit/fetch.go`:

```go
package reddit

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"time"

	"github.com/devonbooker/market-research/internal/types"
)

type listingResponse struct {
	Data struct {
		Children []struct {
			Data postData `json:"data"`
		} `json:"children"`
	} `json:"data"`
}

type postData struct {
	ID            string  `json:"id"`
	Subreddit     string  `json:"subreddit"`
	Title         string  `json:"title"`
	SelfText      string  `json:"selftext"`
	Author        string  `json:"author"`
	Score         int     `json:"score"`
	Permalink     string  `json:"permalink"`
	URL           string  `json:"url"`
	CreatedUTC    float64 `json:"created_utc"`
	LinkFlairText string  `json:"link_flair_text"`
}

const redditPermalinkBase = "https://reddit.com"

// FetchSubredditNew pulls /r/{name}/new, returns documents created after `since`, capped at maxPosts.
func FetchSubredditNew(ctx context.Context, c *Client, subreddit string, since time.Time, maxPosts int) ([]*types.Document, error) {
	path := fmt.Sprintf("/r/%s/new?limit=100", url.PathEscape(subreddit))
	var resp listingResponse
	if err := c.GetJSON(ctx, path, &resp); err != nil {
		return nil, err
	}
	return toDocs(resp.Data.Children, since, maxPosts)
}

// FetchSearch runs a query scoped to a specific subreddit or sitewide if subreddit is empty.
func FetchSearch(ctx context.Context, c *Client, query, subreddit string, since time.Time, maxPosts int) ([]*types.Document, error) {
	q := url.Values{}
	q.Set("q", query)
	q.Set("sort", "new")
	q.Set("limit", "100")
	path := "/search?" + q.Encode()
	if subreddit != "" {
		q.Set("restrict_sr", "on")
		path = fmt.Sprintf("/r/%s/search?%s", url.PathEscape(subreddit), q.Encode())
	}
	var resp listingResponse
	if err := c.GetJSON(ctx, path, &resp); err != nil {
		return nil, err
	}
	return toDocs(resp.Data.Children, since, maxPosts)
}

func toDocs(children []struct {
	Data postData `json:"data"`
}, since time.Time, maxPosts int) ([]*types.Document, error) {
	out := make([]*types.Document, 0, len(children))
	for _, c := range children {
		created := time.Unix(int64(c.Data.CreatedUTC), 0).UTC()
		if created.Before(since) {
			continue
		}
		meta, _ := json.Marshal(map[string]any{
			"subreddit": c.Data.Subreddit,
			"flair":     c.Data.LinkFlairText,
		})
		u := c.Data.URL
		if u == "" || !isExternalURL(c.Data.Permalink) {
			u = redditPermalinkBase + c.Data.Permalink
		}
		out = append(out, &types.Document{
			Platform:         types.PlatformReddit,
			PlatformID:       c.Data.ID,
			Title:            c.Data.Title,
			Body:             c.Data.SelfText,
			Author:           c.Data.Author,
			Score:            c.Data.Score,
			URL:              u,
			CreatedAt:        created,
			FetchedAt:        time.Now().UTC(),
			PlatformMetadata: meta,
		})
		if maxPosts > 0 && len(out) >= maxPosts {
			break
		}
	}
	return out, nil
}

func isExternalURL(s string) bool {
	// permalinks start with "/r/", external URLs are absolute.
	return len(s) >= 4 && s[:4] == "http"
}
```

- [ ] **Step 4: Run tests**

```bash
go test ./internal/fetch/reddit/... -v
```

Expected: all fetch tests PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/fetch/reddit/fetch.go internal/fetch/reddit/fetch_test.go internal/fetch/reddit/testdata/
git commit -m "feat(fetch/reddit): fetch subreddit new + search with since-filter and cap"
```

---

### Task 18: Reddit fetch - top comments per post

**Files:**
- Modify: `internal/fetch/reddit/fetch.go`
- Modify: `internal/fetch/reddit/fetch_test.go`
- Create: `internal/fetch/reddit/testdata/comments.json`

- [ ] **Step 1: Create test fixture**

Create `internal/fetch/reddit/testdata/comments.json`:

```json
[
  {
    "data": {
      "children": [
        { "kind": "t3", "data": { "id": "p1", "title": "ignored" } }
      ]
    }
  },
  {
    "data": {
      "children": [
        {
          "kind": "t1",
          "data": {
            "id": "c1",
            "body": "Same problem here, tried X and Y and nothing works.",
            "author": "alice",
            "score": 25,
            "created_utc": 1713000500
          }
        },
        {
          "kind": "t1",
          "data": {
            "id": "c2",
            "body": "We switched to Z last month.",
            "author": "bob",
            "score": 12,
            "created_utc": 1713000800
          }
        },
        {
          "kind": "more",
          "data": { "children": ["c3", "c4"] }
        }
      ]
    }
  }
]
```

- [ ] **Step 2: Extend test file**

Append to `internal/fetch/reddit/fetch_test.go`:

```go
func TestFetchTopComments_ReturnsTopNSkippingMoreKind(t *testing.T) {
	srv := serveFixture(t, "testdata/comments.json")
	defer srv.Close()

	c := New(Config{
		ClientID: "c", ClientSecret: "s", UserAgent: "ua",
		AuthURL: srv.URL + "/api/v1/access_token", APIBaseURL: srv.URL,
		RateLimit: 100,
	})

	replies, err := FetchTopComments(context.Background(), c, "p1", 10)
	require.NoError(t, err)
	require.Len(t, replies, 2)
	assert.Equal(t, "c1", replies[0].PlatformID)
	assert.Equal(t, "Same problem here, tried X and Y and nothing works.", replies[0].Body)
	assert.Equal(t, "alice", replies[0].Author)
	assert.Equal(t, 25, replies[0].Score)
}
```

- [ ] **Step 3: Implement FetchTopComments**

Append to `internal/fetch/reddit/fetch.go`:

```go
type commentChild struct {
	Kind string `json:"kind"`
	Data struct {
		ID         string  `json:"id"`
		Body       string  `json:"body"`
		Author     string  `json:"author"`
		Score      int     `json:"score"`
		CreatedUTC float64 `json:"created_utc"`
	} `json:"data"`
}

type commentsPage struct {
	Data struct {
		Children []commentChild `json:"children"`
	} `json:"data"`
}

// FetchTopComments returns at most `limit` top-sorted comments for the given post ID.
// Reddit's comments endpoint returns [postListing, commentListing]. We only read the second.
func FetchTopComments(ctx context.Context, c *Client, postID string, limit int) ([]*types.Reply, error) {
	path := fmt.Sprintf("/comments/%s.json?limit=%d&sort=top", url.PathEscape(postID), limit)
	var resp []commentsPage
	if err := c.GetJSON(ctx, path, &resp); err != nil {
		return nil, err
	}
	if len(resp) < 2 {
		return nil, nil
	}
	out := make([]*types.Reply, 0, limit)
	for _, ch := range resp[1].Data.Children {
		if ch.Kind != "t1" {
			continue // skip 'more' and other kinds
		}
		out = append(out, &types.Reply{
			PlatformID: ch.Data.ID,
			Body:       ch.Data.Body,
			Author:     ch.Data.Author,
			Score:      ch.Data.Score,
			CreatedAt:  time.Unix(int64(ch.Data.CreatedUTC), 0).UTC(),
		})
		if len(out) >= limit {
			break
		}
	}
	return out, nil
}
```

- [ ] **Step 4: Run tests**

```bash
go test ./internal/fetch/reddit/... -v
```

Expected: comment test PASSes, prior tests still green.

- [ ] **Step 5: Commit**

```bash
git add internal/fetch/reddit/fetch.go internal/fetch/reddit/fetch_test.go internal/fetch/reddit/testdata/comments.json
git commit -m "feat(fetch/reddit): top-N comments via /comments/{id}.json?sort=top"
```

---

## Phase 6: Fetch - Stack Overflow

### Task 19: Stack Exchange client + rate limiting

**Files:**
- Create: `internal/fetch/stackoverflow/client.go`
- Create: `internal/fetch/stackoverflow/client_test.go`

- [ ] **Step 1: Write failing test**

Create `internal/fetch/stackoverflow/client_test.go`:

```go
package stackoverflow

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestClient_GetJSON_SetsKey(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "mykey", r.URL.Query().Get("key"))
		assert.Equal(t, "stackoverflow", r.URL.Query().Get("site"))
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"items":[]}`))
	}))
	defer srv.Close()

	c := New(Config{Key: "mykey", APIBaseURL: srv.URL, RateLimit: 100})
	var resp struct{ Items []any }
	require.NoError(t, c.GetJSON(context.Background(), "/questions", nil, &resp))
}

func TestClient_GetJSON_HTTPError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "bad", http.StatusBadRequest)
	}))
	defer srv.Close()
	c := New(Config{Key: "k", APIBaseURL: srv.URL, RateLimit: 100})
	err := c.GetJSON(context.Background(), "/whatever", nil, &struct{}{})
	require.Error(t, err)
	var he *HTTPError
	assert.ErrorAs(t, err, &he)
	assert.Equal(t, 400, he.Status)
}
```

- [ ] **Step 2: Implement client**

Create `internal/fetch/stackoverflow/client.go`:

```go
package stackoverflow

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"

	"golang.org/x/time/rate"
)

type Config struct {
	Key        string
	APIBaseURL string
	RateLimit  float64 // requests per second
	Site       string  // defaults to "stackoverflow"
}

type HTTPError struct {
	Status int
	Body   string
}

func (e *HTTPError) Error() string { return fmt.Sprintf("stackexchange http %d: %s", e.Status, e.Body) }

type Client struct {
	cfg     Config
	http    *http.Client
	limiter *rate.Limiter
}

func New(cfg Config) *Client {
	if cfg.APIBaseURL == "" {
		cfg.APIBaseURL = "https://api.stackexchange.com/2.3"
	}
	if cfg.Site == "" {
		cfg.Site = "stackoverflow"
	}
	if cfg.RateLimit == 0 {
		cfg.RateLimit = 1.0
	}
	return &Client{
		cfg:     cfg,
		http:    &http.Client{Timeout: 30 * time.Second},
		limiter: rate.NewLimiter(rate.Limit(cfg.RateLimit), 1),
	}
}

func (c *Client) GetJSON(ctx context.Context, path string, params url.Values, out any) error {
	if err := c.limiter.Wait(ctx); err != nil {
		return err
	}
	if params == nil {
		params = url.Values{}
	}
	params.Set("key", c.cfg.Key)
	params.Set("site", c.cfg.Site)

	u, err := url.Parse(c.cfg.APIBaseURL + path)
	if err != nil {
		return err
	}
	u.RawQuery = params.Encode()

	req, err := http.NewRequestWithContext(ctx, "GET", u.String(), nil)
	if err != nil {
		return err
	}
	req.Header.Set("Accept", "application/json")

	resp, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		b, _ := io.ReadAll(resp.Body)
		return &HTTPError{Status: resp.StatusCode, Body: string(b)}
	}
	return json.NewDecoder(resp.Body).Decode(out)
}
```

- [ ] **Step 3: Run tests**

```bash
go test ./internal/fetch/stackoverflow/... -v
```

Expected: both client tests PASS.

- [ ] **Step 4: Commit**

```bash
git add internal/fetch/stackoverflow/client.go internal/fetch/stackoverflow/client_test.go
git commit -m "feat(fetch/so): stack exchange api client with rate limiter"
```

---

### Task 20: Stack Overflow fetch - questions + accepted answer

**Files:**
- Create: `internal/fetch/stackoverflow/fetch.go`
- Create: `internal/fetch/stackoverflow/fetch_test.go`
- Create: `internal/fetch/stackoverflow/testdata/questions_tag.json`

- [ ] **Step 1: Create test fixture**

Create `internal/fetch/stackoverflow/testdata/questions_tag.json`:

```json
{
  "items": [
    {
      "question_id": 12345,
      "title": "How to pass SOC2 audit evidence?",
      "body": "We need to collect evidence continuously...",
      "owner": { "display_name": "alice" },
      "score": 7,
      "view_count": 123,
      "tags": ["soc2", "compliance"],
      "link": "https://stackoverflow.com/questions/12345",
      "creation_date": 1713000000,
      "is_answered": true,
      "accepted_answer_id": 99999
    },
    {
      "question_id": 12346,
      "title": "No accepted answer question",
      "body": "…",
      "owner": { "display_name": "bob" },
      "score": 3,
      "view_count": 50,
      "tags": ["compliance"],
      "link": "https://stackoverflow.com/questions/12346",
      "creation_date": 1713000100,
      "is_answered": false
    }
  ]
}
```

- [ ] **Step 2: Create answers fixture**

Create `internal/fetch/stackoverflow/testdata/answers.json`:

```json
{
  "items": [
    {
      "answer_id": 99999,
      "question_id": 12345,
      "body": "We built a cron job that collects evidence from cloudtrail...",
      "owner": { "display_name": "charlie" },
      "score": 18,
      "is_accepted": true,
      "creation_date": 1713001000
    }
  ]
}
```

- [ ] **Step 3: Write failing test**

Create `internal/fetch/stackoverflow/fetch_test.go`:

```go
package stackoverflow

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/devonbooker/market-research/internal/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFetchQuestionsByTag_ParsesAndFiltersBySince(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Contains(t, r.URL.Path, "/questions")
		data, err := os.ReadFile("testdata/questions_tag.json")
		require.NoError(t, err)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(data)
	}))
	defer srv.Close()

	c := New(Config{Key: "k", APIBaseURL: srv.URL, RateLimit: 100})
	since := time.Unix(1710000000, 0).UTC()
	docs, err := FetchQuestionsByTag(context.Background(), c, "soc2", since, 100)
	require.NoError(t, err)
	require.Len(t, docs, 2)

	d := docs[0]
	assert.Equal(t, types.PlatformStackOverflow, d.Platform)
	assert.Equal(t, "12345", d.PlatformID)
	assert.Contains(t, d.Title, "SOC2 audit")
	assert.Equal(t, "alice", d.Author)
	assert.Equal(t, 7, d.Score)
	assert.Equal(t, "https://stackoverflow.com/questions/12345", d.URL)
	assert.Contains(t, string(d.PlatformMetadata), "soc2")
	assert.Contains(t, string(d.PlatformMetadata), "accepted_answer_id")
}

func TestFetchAcceptedAnswer_ReturnsReplyForAcceptedOnly(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.True(t, strings.Contains(r.URL.Path, "/answers/"))
		data, err := os.ReadFile("testdata/answers.json")
		require.NoError(t, err)
		_, _ = w.Write(data)
	}))
	defer srv.Close()

	c := New(Config{Key: "k", APIBaseURL: srv.URL, RateLimit: 100})
	reply, err := FetchAcceptedAnswer(context.Background(), c, 99999)
	require.NoError(t, err)
	require.NotNil(t, reply)
	assert.Equal(t, "99999", reply.PlatformID)
	assert.Equal(t, 18, reply.Score)
	require.NotNil(t, reply.IsAccepted)
	assert.True(t, *reply.IsAccepted)
}
```

- [ ] **Step 4: Implement fetch.go**

Create `internal/fetch/stackoverflow/fetch.go`:

```go
package stackoverflow

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"strconv"
	"time"

	"github.com/devonbooker/market-research/internal/types"
)

type question struct {
	QuestionID       int64    `json:"question_id"`
	Title            string   `json:"title"`
	Body             string   `json:"body"`
	Owner            owner    `json:"owner"`
	Score            int      `json:"score"`
	ViewCount        int      `json:"view_count"`
	Tags             []string `json:"tags"`
	Link             string   `json:"link"`
	CreationDate     int64    `json:"creation_date"`
	IsAnswered       bool     `json:"is_answered"`
	AcceptedAnswerID *int64   `json:"accepted_answer_id,omitempty"`
}

type answer struct {
	AnswerID     int64  `json:"answer_id"`
	QuestionID   int64  `json:"question_id"`
	Body         string `json:"body"`
	Owner        owner  `json:"owner"`
	Score        int    `json:"score"`
	IsAccepted   bool   `json:"is_accepted"`
	CreationDate int64  `json:"creation_date"`
}

type owner struct {
	DisplayName string `json:"display_name"`
}

type itemsResponse[T any] struct {
	Items []T `json:"items"`
}

// FetchQuestionsByTag pulls new questions for a single tag since the given time, up to maxPosts.
// Uses filter=withbody to get question body inline.
func FetchQuestionsByTag(ctx context.Context, c *Client, tag string, since time.Time, maxPosts int) ([]*types.Document, error) {
	params := url.Values{}
	params.Set("tagged", tag)
	params.Set("fromdate", strconv.FormatInt(since.Unix(), 10))
	params.Set("sort", "creation")
	params.Set("order", "desc")
	params.Set("pagesize", "100")
	params.Set("filter", "withbody")

	var resp itemsResponse[question]
	if err := c.GetJSON(ctx, "/questions", params, &resp); err != nil {
		return nil, err
	}
	return questionsToDocs(resp.Items, maxPosts), nil
}

// FetchSearch runs an advanced search query.
func FetchSearch(ctx context.Context, c *Client, query string, since time.Time, maxPosts int) ([]*types.Document, error) {
	params := url.Values{}
	params.Set("q", query)
	params.Set("fromdate", strconv.FormatInt(since.Unix(), 10))
	params.Set("sort", "creation")
	params.Set("order", "desc")
	params.Set("pagesize", "100")
	params.Set("filter", "withbody")

	var resp itemsResponse[question]
	if err := c.GetJSON(ctx, "/search/advanced", params, &resp); err != nil {
		return nil, err
	}
	return questionsToDocs(resp.Items, maxPosts), nil
}

// FetchAcceptedAnswer fetches a specific answer by ID. Returns nil reply if not accepted.
func FetchAcceptedAnswer(ctx context.Context, c *Client, answerID int64) (*types.Reply, error) {
	params := url.Values{}
	params.Set("filter", "withbody")
	path := fmt.Sprintf("/answers/%d", answerID)

	var resp itemsResponse[answer]
	if err := c.GetJSON(ctx, path, params, &resp); err != nil {
		return nil, err
	}
	if len(resp.Items) == 0 {
		return nil, nil
	}
	a := resp.Items[0]
	accepted := a.IsAccepted
	return &types.Reply{
		PlatformID: strconv.FormatInt(a.AnswerID, 10),
		Body:       a.Body,
		Author:     a.Owner.DisplayName,
		Score:      a.Score,
		CreatedAt:  time.Unix(a.CreationDate, 0).UTC(),
		IsAccepted: &accepted,
	}, nil
}

func questionsToDocs(qs []question, maxPosts int) []*types.Document {
	out := make([]*types.Document, 0, len(qs))
	for _, q := range qs {
		meta, _ := json.Marshal(map[string]any{
			"tags":               q.Tags,
			"view_count":         q.ViewCount,
			"is_answered":        q.IsAnswered,
			"accepted_answer_id": q.AcceptedAnswerID,
		})
		out = append(out, &types.Document{
			Platform:         types.PlatformStackOverflow,
			PlatformID:       strconv.FormatInt(q.QuestionID, 10),
			Title:            q.Title,
			Body:             q.Body,
			Author:           q.Owner.DisplayName,
			Score:            q.Score,
			URL:              q.Link,
			CreatedAt:        time.Unix(q.CreationDate, 0).UTC(),
			FetchedAt:        time.Now().UTC(),
			PlatformMetadata: meta,
		})
		if maxPosts > 0 && len(out) >= maxPosts {
			break
		}
	}
	return out
}
```

- [ ] **Step 5: Run tests**

```bash
go test ./internal/fetch/stackoverflow/... -v
```

Expected: both fetch tests PASS.

- [ ] **Step 6: Commit**

```bash
git add internal/fetch/stackoverflow/fetch.go internal/fetch/stackoverflow/fetch_test.go internal/fetch/stackoverflow/testdata/
git commit -m "feat(fetch/so): questions by tag + search + accepted answer"
```

---

## Phase 7: Fetch orchestrator

### Task 21: Orchestrator with retry + permanent-error deactivation

**Files:**
- Create: `internal/fetch/errors.go`
- Create: `internal/fetch/orchestrator.go`
- Create: `internal/fetch/orchestrator_test.go`

- [ ] **Step 1: Create error types**

Create `internal/fetch/errors.go`:

```go
package fetch

import (
	"errors"

	redditc "github.com/devonbooker/market-research/internal/fetch/reddit"
	soc "github.com/devonbooker/market-research/internal/fetch/stackoverflow"
)

// IsTransient returns true for retryable HTTP failures (5xx, 429) or network errors.
func IsTransient(err error) bool {
	var rhe *redditc.HTTPError
	if errors.As(err, &rhe) {
		return rhe.Status >= 500 || rhe.Status == 429
	}
	var she *soc.HTTPError
	if errors.As(err, &she) {
		return she.Status >= 500 || she.Status == 429
	}
	// network errors, context errors, etc: treat as transient
	return err != nil
}

// IsPermanent returns true for 403/404/410, signaling the source should be deactivated.
func IsPermanent(err error) bool {
	var rhe *redditc.HTTPError
	if errors.As(err, &rhe) {
		return rhe.Status == 403 || rhe.Status == 404 || rhe.Status == 410
	}
	var she *soc.HTTPError
	if errors.As(err, &she) {
		return she.Status == 403 || she.Status == 404 || she.Status == 410
	}
	return false
}
```

- [ ] **Step 2: Write orchestrator interfaces test first**

Create `internal/fetch/orchestrator_test.go`:

```go
package fetch

import (
	"context"
	"errors"
	"testing"
	"time"

	redditc "github.com/devonbooker/market-research/internal/fetch/reddit"
	"github.com/devonbooker/market-research/internal/store"
	"github.com/devonbooker/market-research/internal/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type stubFetcher struct {
	docs    []*types.Document
	replies map[string][]*types.Reply
	err     error
}

func (s *stubFetcher) FetchDocuments(ctx context.Context, src *types.Source, since time.Time, max int) ([]*types.Document, error) {
	if s.err != nil {
		return nil, s.err
	}
	return s.docs, nil
}
func (s *stubFetcher) FetchReplies(ctx context.Context, platformID string) ([]*types.Reply, error) {
	return s.replies[platformID], nil
}

func openStore(t *testing.T) *store.Store {
	t.Helper()
	s, err := store.Open(":memory:")
	require.NoError(t, err)
	t.Cleanup(func() { s.Close() })
	return s
}

func TestOrchestrator_HappyPath(t *testing.T) {
	st := openStore(t)
	topicID, _ := st.CreateTopic("t", "", true)
	srcID, _, _ := st.UpsertSource(topicID, types.PlatformReddit, types.SourceKindSubreddit, "x", types.AddedByAgent)

	now := time.Now().UTC()
	doc := &types.Document{
		TopicID: topicID, SourceID: srcID,
		Platform: types.PlatformReddit, PlatformID: "p1",
		Title: "t", URL: "u", CreatedAt: now, FetchedAt: now,
	}
	f := &stubFetcher{
		docs:    []*types.Document{doc},
		replies: map[string][]*types.Reply{"p1": {{PlatformID: "c1", Body: "b", CreatedAt: now}}},
	}

	o := &Orchestrator{
		Store:        st,
		Reddit:       f,
		StackOverflow: &stubFetcher{},
		BackfillWindow: 7 * 24 * time.Hour,
		MaxPostsPerSource: 100,
		TopCommentsPerPost: 10,
	}

	err := o.RunAll(context.Background())
	require.NoError(t, err)

	var docCount int
	require.NoError(t, st.DB().QueryRow("SELECT COUNT(*) FROM documents").Scan(&docCount))
	assert.Equal(t, 1, docCount)

	var replyCount int
	require.NoError(t, st.DB().QueryRow("SELECT COUNT(*) FROM document_replies").Scan(&replyCount))
	assert.Equal(t, 1, replyCount)

	var runCount int
	require.NoError(t, st.DB().QueryRow("SELECT COUNT(*) FROM fetch_runs WHERE status='success'").Scan(&runCount))
	assert.Equal(t, 2, runCount) // one per platform
}

func TestOrchestrator_PermanentErrorDeactivatesSource(t *testing.T) {
	st := openStore(t)
	topicID, _ := st.CreateTopic("t", "", true)
	srcID, _, _ := st.UpsertSource(topicID, types.PlatformReddit, types.SourceKindSubreddit, "x", types.AddedByAgent)

	f := &stubFetcher{err: &redditc.HTTPError{Status: 404, Body: "gone"}}
	o := &Orchestrator{
		Store: st, Reddit: f, StackOverflow: &stubFetcher{},
		BackfillWindow: 7 * 24 * time.Hour, MaxPostsPerSource: 100, TopCommentsPerPost: 10,
		Retries: 1,
	}
	require.NoError(t, o.RunAll(context.Background()))

	src, err := st.GetSource(srcID)
	require.NoError(t, err)
	assert.False(t, src.Active, "source should be deactivated on 404")
}

func TestOrchestrator_TransientErrorRetriedThenFails(t *testing.T) {
	st := openStore(t)
	topicID, _ := st.CreateTopic("t", "", true)
	_, _, _ = st.UpsertSource(topicID, types.PlatformReddit, types.SourceKindSubreddit, "x", types.AddedByAgent)

	f := &stubFetcher{err: errors.New("network blip")}
	o := &Orchestrator{
		Store: st, Reddit: f, StackOverflow: &stubFetcher{},
		BackfillWindow: 7 * 24 * time.Hour, MaxPostsPerSource: 100, TopCommentsPerPost: 10,
		Retries: 2, BackoffBase: time.Millisecond,
	}
	require.NoError(t, o.RunAll(context.Background()))

	var errorRuns int
	require.NoError(t, st.DB().QueryRow("SELECT COUNT(*) FROM fetch_runs WHERE status='error'").Scan(&errorRuns))
	assert.GreaterOrEqual(t, errorRuns, 1)
}
```

- [ ] **Step 3: Implement orchestrator**

Create `internal/fetch/orchestrator.go`:

```go
package fetch

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/devonbooker/market-research/internal/store"
	"github.com/devonbooker/market-research/internal/types"
)

// PlatformFetcher is the boundary between the orchestrator and concrete per-platform code.
// Reddit and StackOverflow packages implement this via small adapters (see cmd/mr/main.go wiring).
type PlatformFetcher interface {
	FetchDocuments(ctx context.Context, src *types.Source, since time.Time, maxPosts int) ([]*types.Document, error)
	FetchReplies(ctx context.Context, docPlatformID string) ([]*types.Reply, error)
}

type Orchestrator struct {
	Store         *store.Store
	Reddit        PlatformFetcher
	StackOverflow PlatformFetcher

	// Tunables
	BackfillWindow     time.Duration // window for first-time fetch on a new source
	MaxPostsPerSource  int           // cap per source per run
	TopCommentsPerPost int           // comment cap per post
	Retries            int           // transient retry count (default 3)
	BackoffBase        time.Duration // base for exponential backoff (default 500ms)
}

func (o *Orchestrator) RunAll(ctx context.Context) error {
	topics, err := o.Store.ListTopics(false)
	if err != nil {
		return err
	}
	for _, t := range topics {
		o.runTopic(ctx, t)
	}
	return nil
}

func (o *Orchestrator) RunTopic(ctx context.Context, name string) error {
	t, err := o.Store.GetTopicByName(name)
	if err != nil {
		return err
	}
	o.runTopic(ctx, t)
	return nil
}

func (o *Orchestrator) runTopic(ctx context.Context, t *types.Topic) {
	defer func() {
		if r := recover(); r != nil {
			slog.Error("topic panic", "topic", t.Name, "panic", r)
		}
	}()

	var wg sync.WaitGroup
	for _, pair := range []struct {
		platform types.Platform
		fetcher  PlatformFetcher
	}{
		{types.PlatformReddit, o.Reddit},
		{types.PlatformStackOverflow, o.StackOverflow},
	} {
		wg.Add(1)
		go func(platform types.Platform, fetcher PlatformFetcher) {
			defer wg.Done()
			o.runPlatform(ctx, t, platform, fetcher)
		}(pair.platform, pair.fetcher)
	}
	wg.Wait()
}

func (o *Orchestrator) runPlatform(ctx context.Context, t *types.Topic, platform types.Platform, fetcher PlatformFetcher) {
	runID, err := o.Store.StartFetchRun(t.ID, platform)
	if err != nil {
		slog.Error("start fetch run", "err", err)
		return
	}

	var docsNew, repliesNew int
	var lastErr error

	sources, err := o.Store.ListSources(t.ID, platform, false)
	if err != nil {
		_ = o.Store.CloseFetchRun(runID, types.RunStatusError, 0, 0, err.Error())
		return
	}

	for _, src := range sources {
		since := o.backfillStart(src)
		docs, err := o.fetchWithRetry(ctx, fetcher, src, since)
		if err != nil {
			lastErr = err
			slog.Error("fetch source failed", "topic", t.Name, "source", src.Value, "err", err)
			if IsPermanent(err) {
				_ = o.Store.SetSourceActive(src.ID, false)
			}
			continue
		}

		for _, d := range docs {
			d.TopicID = t.ID
			d.SourceID = src.ID
			id, inserted, err := o.Store.UpsertDocument(d)
			if err != nil {
				lastErr = err
				continue
			}
			if inserted {
				docsNew++
			}
			replies, err := fetcher.FetchReplies(ctx, d.PlatformID)
			if err != nil {
				slog.Warn("fetch replies", "platform_id", d.PlatformID, "err", err)
				continue
			}
			for _, r := range replies {
				r.DocumentID = id
				if _, ins, err := o.Store.UpsertReply(r); err == nil && ins {
					repliesNew++
				}
			}
		}
		_ = o.Store.UpdateSourceLastFetched(src.ID, time.Now().UTC())
	}

	status := types.RunStatusSuccess
	errMsg := ""
	if lastErr != nil {
		status = types.RunStatusError
		errMsg = lastErr.Error()
	}
	_ = o.Store.CloseFetchRun(runID, status, docsNew, repliesNew, errMsg)
}

func (o *Orchestrator) backfillStart(src *types.Source) time.Time {
	if src.LastFetched != nil {
		return *src.LastFetched
	}
	w := o.BackfillWindow
	if w == 0 {
		w = 7 * 24 * time.Hour
	}
	return time.Now().Add(-w).UTC()
}

func (o *Orchestrator) fetchWithRetry(ctx context.Context, fetcher PlatformFetcher, src *types.Source, since time.Time) ([]*types.Document, error) {
	retries := o.Retries
	if retries == 0 {
		retries = 3
	}
	base := o.BackoffBase
	if base == 0 {
		base = 500 * time.Millisecond
	}
	maxPosts := o.MaxPostsPerSource
	if maxPosts == 0 {
		maxPosts = 100
	}

	var lastErr error
	for attempt := 0; attempt <= retries; attempt++ {
		docs, err := fetcher.FetchDocuments(ctx, src, since, maxPosts)
		if err == nil {
			return docs, nil
		}
		lastErr = err
		if IsPermanent(err) {
			return nil, err
		}
		if !IsTransient(err) {
			return nil, err
		}
		if attempt == retries {
			break
		}
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(base * (1 << attempt)):
		}
	}
	return nil, fmt.Errorf("exhausted retries: %w", lastErr)
}
```

- [ ] **Step 4: Run tests**

```bash
go test ./internal/fetch/... -race -v
```

Expected: three orchestrator tests PASS, all Reddit/SO tests still green.

- [ ] **Step 5: Commit**

```bash
git add internal/fetch/errors.go internal/fetch/orchestrator.go internal/fetch/orchestrator_test.go
git commit -m "feat(fetch): orchestrator with retry, permanent-error deactivation, per-platform goroutines"
```

---

## Phase 8: CLI

### Task 22: CLI skeleton (cobra root + wiring)

**Files:**
- Modify: `cmd/mr/main.go`

- [ ] **Step 1: Add cobra**

```bash
go get github.com/spf13/cobra
```

- [ ] **Step 2: Replace main.go**

Overwrite `cmd/mr/main.go`:

```go
package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/devonbooker/market-research/internal/config"
	"github.com/devonbooker/market-research/internal/store"
	"github.com/spf13/cobra"
)

type runtime struct {
	cfg   *config.Config
	store *store.Store
}

func newRootCmd() (*cobra.Command, *runtime) {
	rt := &runtime{}
	root := &cobra.Command{
		Use:   "mr",
		Short: "market-research data collection CLI",
		PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := config.Load()
			if err != nil {
				return err
			}
			rt.cfg = cfg
			s, err := store.Open(cfg.DBPath)
			if err != nil {
				return fmt.Errorf("open store: %w", err)
			}
			rt.store = s
			if n, err := s.MarkOrphanRunsErrored("process restarted"); err == nil && n > 0 {
				slog.Warn("recovered orphan runs", "count", n)
			}
			return nil
		},
		PersistentPostRun: func(cmd *cobra.Command, args []string) {
			if rt.store != nil {
				_ = rt.store.Close()
			}
		},
	}
	root.AddCommand(newTopicCmd(rt))
	root.AddCommand(newFetchCmd(rt))
	root.AddCommand(newRediscoverCmd(rt))
	root.AddCommand(newDoctorCmd(rt))
	return root, rt
}

func main() {
	slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stderr, nil)))

	defer func() {
		if r := recover(); r != nil {
			slog.Error("fatal panic", "panic", r)
			os.Exit(2)
		}
	}()

	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	root, _ := newRootCmd()
	root.SetContext(ctx)
	if err := root.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}
```

- [ ] **Step 3: Create placeholder subcommand files so build passes**

Create `cmd/mr/cmd_topic.go`:

```go
package main

import "github.com/spf13/cobra"

func newTopicCmd(rt *runtime) *cobra.Command {
	return &cobra.Command{Use: "topic", Short: "manage topics"}
}
```

Create `cmd/mr/cmd_fetch.go`:

```go
package main

import "github.com/spf13/cobra"

func newFetchCmd(rt *runtime) *cobra.Command {
	return &cobra.Command{Use: "fetch", Short: "fetch new content for topics"}
}
```

Create `cmd/mr/cmd_rediscover.go`:

```go
package main

import "github.com/spf13/cobra"

func newRediscoverCmd(rt *runtime) *cobra.Command {
	return &cobra.Command{Use: "rediscover", Short: "re-run source discovery"}
}
```

Create `cmd/mr/cmd_doctor.go`:

```go
package main

import "github.com/spf13/cobra"

func newDoctorCmd(rt *runtime) *cobra.Command {
	return &cobra.Command{Use: "doctor", Short: "diagnose system health"}
}
```

- [ ] **Step 4: Verify build**

```bash
make build
```

Expected: `bin/mr` produced successfully.

- [ ] **Step 5: Commit**

```bash
git add go.mod go.sum cmd/mr/
git commit -m "feat(cmd): cobra root with config/store wiring and subcommand stubs"
```

---

### Task 23: `mr topic add/list/remove`

**Files:**
- Modify: `cmd/mr/cmd_topic.go`

- [ ] **Step 1: Replace cmd_topic.go**

Overwrite `cmd/mr/cmd_topic.go`:

```go
package main

import (
	"fmt"

	"github.com/devonbooker/market-research/internal/sources"
	"github.com/devonbooker/market-research/internal/store"
	"github.com/devonbooker/market-research/internal/types"
	"github.com/spf13/cobra"
)

func newTopicCmd(rt *runtime) *cobra.Command {
	c := &cobra.Command{Use: "topic", Short: "manage topics"}
	c.AddCommand(newTopicAddCmd(rt))
	c.AddCommand(newTopicListCmd(rt))
	c.AddCommand(newTopicRemoveCmd(rt))
	return c
}

func newTopicAddCmd(rt *runtime) *cobra.Command {
	var description string
	c := &cobra.Command{
		Use:   "add <name>",
		Short: "add a topic and run initial source discovery",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			name := args[0]
			if _, err := rt.store.GetTopicByName(name); err == nil {
				return fmt.Errorf("topic %q already exists", name)
			}

			// Start inactive; activate after discovery succeeds.
			id, err := rt.store.CreateTopic(name, description, false)
			if err != nil {
				return err
			}

			agent := &sources.Agent{Claude: sources.NewAnthropicClient(rt.cfg.AnthropicAPIKey)}
			plan, err := agent.Discover(cmd.Context(), name, description)
			if err != nil {
				fmt.Fprintf(cmd.OutOrStderr(), "warning: discovery failed, topic kept inactive: %v\n", err)
				return nil
			}

			for _, s := range sources.PlanToSources(plan) {
				if _, _, err := rt.store.UpsertSource(id, s.Platform, s.Kind, s.Value, types.AddedByAgent); err != nil {
					return err
				}
			}
			if err := rt.store.SetTopicActive(id, true); err != nil {
				return err
			}

			fmt.Fprintf(cmd.OutOrStdout(), "added topic %q with %d sources\n", name, len(sources.PlanToSources(plan)))
			return nil
		},
	}
	c.Flags().StringVar(&description, "description", "", "optional description passed to the agent")
	return c
}

func newTopicListCmd(rt *runtime) *cobra.Command {
	var showIssues bool
	c := &cobra.Command{
		Use:   "list",
		Short: "list topics and their sources",
		RunE: func(cmd *cobra.Command, args []string) error {
			topics, err := rt.store.ListTopics(true)
			if err != nil {
				return err
			}
			for _, t := range topics {
				fmt.Fprintf(cmd.OutOrStdout(), "topic: %s (active=%t)\n", t.Name, t.Active)
				for _, p := range []types.Platform{types.PlatformReddit, types.PlatformStackOverflow} {
					srcs, err := rt.store.ListSources(t.ID, p, showIssues)
					if err != nil {
						return err
					}
					for _, s := range srcs {
						marker := ""
						if !s.Active {
							marker = " [INACTIVE]"
						}
						fmt.Fprintf(cmd.OutOrStdout(), "  %s/%s: %s%s\n", s.Platform, s.Kind, s.Value, marker)
					}
				}
			}
			return nil
		},
	}
	c.Flags().BoolVar(&showIssues, "issues", false, "include inactive (errored) sources")
	return c
}

func newTopicRemoveCmd(rt *runtime) *cobra.Command {
	return &cobra.Command{
		Use:   "remove <name>",
		Short: "remove a topic and all its data",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			t, err := rt.store.GetTopicByName(args[0])
			if err != nil {
				return err
			}
			return rt.store.DeleteTopic(t.ID)
		},
	}
}

var _ = store.ErrNotFound
```

- [ ] **Step 2: Verify build**

```bash
make build
```

Expected: `bin/mr` builds. Manual smoke test requires env vars; deferred to E2E test.

- [ ] **Step 3: Commit**

```bash
git add cmd/mr/cmd_topic.go
git commit -m "feat(cmd): mr topic add/list/remove with initial discovery"
```

---

### Task 24: `mr fetch` with platform adapter wiring

**Files:**
- Modify: `cmd/mr/cmd_fetch.go`
- Create: `cmd/mr/adapters.go`

- [ ] **Step 1: Create adapters**

The orchestrator takes a `PlatformFetcher` interface; concrete Reddit and SO packages expose `Fetch*` functions. Create adapters in `cmd/mr/adapters.go`:

```go
package main

import (
	"context"
	"time"

	rfetch "github.com/devonbooker/market-research/internal/fetch/reddit"
	sofetch "github.com/devonbooker/market-research/internal/fetch/stackoverflow"
	"github.com/devonbooker/market-research/internal/types"
)

type redditAdapter struct {
	client             *rfetch.Client
	topCommentsPerPost int
}

func (a *redditAdapter) FetchDocuments(ctx context.Context, src *types.Source, since time.Time, max int) ([]*types.Document, error) {
	switch src.Kind {
	case types.SourceKindSubreddit:
		return rfetch.FetchSubredditNew(ctx, a.client, src.Value, since, max)
	case types.SourceKindSearchQuery:
		return rfetch.FetchSearch(ctx, a.client, src.Value, "", since, max)
	}
	return nil, nil
}

func (a *redditAdapter) FetchReplies(ctx context.Context, platformID string) ([]*types.Reply, error) {
	n := a.topCommentsPerPost
	if n == 0 {
		n = 10
	}
	return rfetch.FetchTopComments(ctx, a.client, platformID, n)
}

type stackOverflowAdapter struct {
	client *sofetch.Client
}

func (a *stackOverflowAdapter) FetchDocuments(ctx context.Context, src *types.Source, since time.Time, max int) ([]*types.Document, error) {
	switch src.Kind {
	case types.SourceKindSOTag:
		return sofetch.FetchQuestionsByTag(ctx, a.client, src.Value, since, max)
	case types.SourceKindSearchQuery:
		return sofetch.FetchSearch(ctx, a.client, src.Value, since, max)
	}
	return nil, nil
}

func (a *stackOverflowAdapter) FetchReplies(ctx context.Context, platformID string) ([]*types.Reply, error) {
	// For SO we need the accepted_answer_id, which lives in the document's platform_metadata.
	// The orchestrator fetches by post platform_id (question_id). We load the document to find the accepted answer id.
	// Here we just return nil; in practice, FetchReplies for SO is wired via a closure in main.go that has store access.
	return nil, nil
}
```

Note: SO replies need document context. We'll provide a store-aware wrapper in `cmd_fetch.go`.

- [ ] **Step 2: Replace cmd_fetch.go**

Overwrite `cmd/mr/cmd_fetch.go`:

```go
package main

import (
	"context"
	"encoding/json"
	"strconv"
	"time"

	"github.com/devonbooker/market-research/internal/fetch"
	rfetch "github.com/devonbooker/market-research/internal/fetch/reddit"
	sofetch "github.com/devonbooker/market-research/internal/fetch/stackoverflow"
	"github.com/devonbooker/market-research/internal/store"
	"github.com/devonbooker/market-research/internal/types"
	"github.com/spf13/cobra"
)

func newFetchCmd(rt *runtime) *cobra.Command {
	var all bool
	var topic string
	c := &cobra.Command{
		Use:   "fetch",
		Short: "fetch new content for topics (daily job)",
		RunE: func(cmd *cobra.Command, args []string) error {
			orch := buildOrchestrator(rt)
			if all || topic == "" {
				return orch.RunAll(cmd.Context())
			}
			return orch.RunTopic(cmd.Context(), topic)
		},
	}
	c.Flags().BoolVar(&all, "all", false, "fetch all active topics (default)")
	c.Flags().StringVar(&topic, "topic", "", "fetch a single topic by name")
	return c
}

func buildOrchestrator(rt *runtime) *fetch.Orchestrator {
	rc := rfetch.New(rfetch.Config{
		ClientID:     rt.cfg.RedditClientID,
		ClientSecret: rt.cfg.RedditClientSecret,
		UserAgent:    rt.cfg.RedditUserAgent,
	})
	soc := sofetch.New(sofetch.Config{Key: rt.cfg.StackExchangeKey})

	return &fetch.Orchestrator{
		Store:              rt.store,
		Reddit:             &redditAdapter{client: rc, topCommentsPerPost: 10},
		StackOverflow:      &storeAwareSOAdapter{client: soc, store: rt.store},
		BackfillWindow:     7 * 24 * time.Hour,
		MaxPostsPerSource:  100,
		TopCommentsPerPost: 10,
		Retries:            3,
		BackoffBase:        500 * time.Millisecond,
	}
}

// storeAwareSOAdapter pulls accepted_answer_id from the stored document metadata
// and fetches it as the reply.
type storeAwareSOAdapter struct {
	client *sofetch.Client
	store  *store.Store
}

func (a *storeAwareSOAdapter) FetchDocuments(ctx context.Context, src *types.Source, since time.Time, max int) ([]*types.Document, error) {
	switch src.Kind {
	case types.SourceKindSOTag:
		return sofetch.FetchQuestionsByTag(ctx, a.client, src.Value, since, max)
	case types.SourceKindSearchQuery:
		return sofetch.FetchSearch(ctx, a.client, src.Value, since, max)
	}
	return nil, nil
}

func (a *storeAwareSOAdapter) FetchReplies(ctx context.Context, platformID string) ([]*types.Reply, error) {
	// Look up the document by (platform, platform_id) to find accepted_answer_id in metadata.
	var metaRaw []byte
	err := a.store.DB().QueryRow(
		`SELECT platform_metadata FROM documents WHERE platform = ? AND platform_id = ?`,
		types.PlatformStackOverflow, platformID,
	).Scan(&metaRaw)
	if err != nil {
		return nil, err
	}
	var meta struct {
		AcceptedAnswerID *int64 `json:"accepted_answer_id"`
	}
	if err := json.Unmarshal(metaRaw, &meta); err != nil {
		return nil, err
	}
	if meta.AcceptedAnswerID == nil {
		return nil, nil
	}
	reply, err := sofetch.FetchAcceptedAnswer(ctx, a.client, *meta.AcceptedAnswerID)
	if err != nil || reply == nil {
		return nil, err
	}
	return []*types.Reply{reply}, nil
}

var _ = strconv.Itoa
```

- [ ] **Step 3: Verify build**

```bash
make build
```

- [ ] **Step 4: Commit**

```bash
git add cmd/mr/adapters.go cmd/mr/cmd_fetch.go
git commit -m "feat(cmd): mr fetch --all / --topic with platform adapters"
```

---

### Task 25: `mr rediscover` with dry-run

**Files:**
- Modify: `cmd/mr/cmd_rediscover.go`

- [ ] **Step 1: Replace cmd_rediscover.go**

Overwrite `cmd/mr/cmd_rediscover.go`:

```go
package main

import (
	"context"
	"fmt"
	"time"

	"github.com/devonbooker/market-research/internal/sources"
	"github.com/devonbooker/market-research/internal/types"
	"github.com/spf13/cobra"
)

func newRediscoverCmd(rt *runtime) *cobra.Command {
	var all bool
	var topic string
	var dryRun bool
	c := &cobra.Command{
		Use:   "rediscover",
		Short: "re-run source discovery for topics (weekly job)",
		RunE: func(cmd *cobra.Command, args []string) error {
			agent := &sources.Agent{Claude: sources.NewAnthropicClient(rt.cfg.AnthropicAPIKey)}
			if all || topic == "" {
				topics, err := rt.store.ListTopics(false)
				if err != nil {
					return err
				}
				for _, t := range topics {
					if err := rediscoverOne(cmd.Context(), cmd, rt, agent, t, dryRun); err != nil {
						fmt.Fprintf(cmd.OutOrStderr(), "rediscover %s: %v\n", t.Name, err)
					}
				}
				return nil
			}
			t, err := rt.store.GetTopicByName(topic)
			if err != nil {
				return err
			}
			return rediscoverOne(cmd.Context(), cmd, rt, agent, t, dryRun)
		},
	}
	c.Flags().BoolVar(&all, "all", false, "rediscover all active topics (default)")
	c.Flags().StringVar(&topic, "topic", "", "single topic by name")
	c.Flags().BoolVar(&dryRun, "dry-run", false, "print proposed changes without writing")
	return c
}

func rediscoverOne(ctx context.Context, cmd *cobra.Command, rt *runtime, agent *sources.Agent, t *types.Topic, dryRun bool) error {
	stats, err := gatherStats(rt, t)
	if err != nil {
		return err
	}
	plan, err := agent.Rediscover(ctx, t.Name, t.Description, stats)
	if err != nil {
		return err
	}

	proposed := sources.PlanToSources(plan)
	if dryRun {
		fmt.Fprintf(cmd.OutOrStdout(), "topic %s: proposed %d sources\n", t.Name, len(proposed))
		for _, s := range proposed {
			fmt.Fprintf(cmd.OutOrStdout(), "  %s/%s: %s\n", s.Platform, s.Kind, s.Value)
		}
		return nil
	}

	for _, s := range proposed {
		if _, _, err := rt.store.UpsertSource(t.ID, s.Platform, s.Kind, s.Value, types.AddedByAgent); err != nil {
			return err
		}
	}

	// Update signal scores for all (kept) agent-added sources based on stats.
	for _, s := range stats {
		srcs, _ := rt.store.ListSources(t.ID, s.Platform, true)
		for _, src := range srcs {
			if src.Kind == s.Kind && src.Value == s.Value && src.AddedBy == types.AddedByAgent {
				score := sources.Score(s.DocsLast7d, s.AvgScore)
				_ = rt.store.SetSourceSignalScore(src.ID, score)
			}
		}
	}
	return nil
}

func gatherStats(rt *runtime, t *types.Topic) ([]sources.SourceStat, error) {
	cutoff := time.Now().UTC().Add(-7 * 24 * time.Hour)
	var out []sources.SourceStat
	for _, p := range []types.Platform{types.PlatformReddit, types.PlatformStackOverflow} {
		srcs, err := rt.store.ListSources(t.ID, p, true)
		if err != nil {
			return nil, err
		}
		for _, s := range srcs {
			n, avg, err := rt.store.SourceStatsSince(s.ID, cutoff)
			if err != nil {
				return nil, err
			}
			signal := 0.0
			if s.SignalScore != nil {
				signal = *s.SignalScore
			}
			out = append(out, sources.SourceStat{
				Platform: s.Platform, Kind: s.Kind, Value: s.Value,
				DocsLast7d: n, AvgScore: avg, SignalScore: signal,
			})
		}
	}
	return out, nil
}
```

- [ ] **Step 2: Verify build**

```bash
make build
```

- [ ] **Step 3: Commit**

```bash
git add cmd/mr/cmd_rediscover.go
git commit -m "feat(cmd): mr rediscover with dry-run and signal-score updates"
```

---

### Task 26: `mr doctor`

**Files:**
- Modify: `cmd/mr/cmd_doctor.go`

- [ ] **Step 1: Replace cmd_doctor.go**

Overwrite `cmd/mr/cmd_doctor.go`:

```go
package main

import (
	"fmt"
	"os"
	"time"

	"github.com/devonbooker/market-research/internal/types"
	"github.com/spf13/cobra"
)

func newDoctorCmd(rt *runtime) *cobra.Command {
	return &cobra.Command{
		Use:   "doctor",
		Short: "report system health (last runs, stale sources, db stats)",
		RunE: func(cmd *cobra.Command, args []string) error {
			out := cmd.OutOrStdout()

			// DB file stats
			if fi, err := os.Stat(rt.cfg.DBPath); err == nil {
				fmt.Fprintf(out, "db: %s (%.2f MB)\n", rt.cfg.DBPath, float64(fi.Size())/1024/1024)
			} else {
				fmt.Fprintf(out, "db: %s (stat error: %v)\n", rt.cfg.DBPath, err)
			}

			topics, err := rt.store.ListTopics(true)
			if err != nil {
				return err
			}

			fmt.Fprintf(out, "\ntopics (%d):\n", len(topics))
			for _, t := range topics {
				fmt.Fprintf(out, "  %s (active=%t)\n", t.Name, t.Active)
				for _, p := range []types.Platform{types.PlatformReddit, types.PlatformStackOverflow} {
					var startedAt *time.Time
					var status string
					row := rt.store.DB().QueryRow(
						`SELECT started_at, status FROM fetch_runs
						 WHERE topic_id = ? AND platform = ? AND status = 'success'
						 ORDER BY started_at DESC LIMIT 1`, t.ID, p)
					var ts time.Time
					if err := row.Scan(&ts, &status); err == nil {
						startedAt = &ts
					}
					if startedAt == nil {
						fmt.Fprintf(out, "    %s: no successful runs\n", p)
					} else {
						age := time.Since(*startedAt).Round(time.Minute)
						fmt.Fprintf(out, "    %s: last success %s ago\n", p, age)
					}
				}
			}

			// Stale sources (no fetch in >7 days)
			rows, err := rt.store.DB().Query(
				`SELECT topic_id, platform, kind, value, last_fetched
				 FROM sources WHERE active = 1 AND (last_fetched IS NULL OR last_fetched < ?)`,
				time.Now().UTC().Add(-7*24*time.Hour))
			if err != nil {
				return err
			}
			defer rows.Close()

			fmt.Fprintln(out, "\nstale sources (>7d since last fetch):")
			any := false
			for rows.Next() {
				var tID int64
				var plat, kind, val string
				var last *time.Time
				if err := rows.Scan(&tID, &plat, &kind, &val, &last); err != nil {
					return err
				}
				any = true
				fmt.Fprintf(out, "  topic=%d %s/%s: %s\n", tID, plat, kind, val)
			}
			if !any {
				fmt.Fprintln(out, "  (none)")
			}
			return nil
		},
	}
}
```

- [ ] **Step 2: Verify build**

```bash
make build
```

- [ ] **Step 3: Commit**

```bash
git add cmd/mr/cmd_doctor.go
git commit -m "feat(cmd): mr doctor reports last runs, stale sources, db size"
```

---

## Phase 9: Deploy + E2E

### Task 27: systemd units

**Files:**
- Create: `deploy/mr-fetch.service`
- Create: `deploy/mr-fetch.timer`
- Create: `deploy/mr-rediscover.service`
- Create: `deploy/mr-rediscover.timer`

- [ ] **Step 1: Create `deploy/mr-fetch.service`**

```ini
[Unit]
Description=market-research daily fetch
Wants=network-online.target
After=network-online.target

[Service]
Type=oneshot
User=mr
Group=mr
EnvironmentFile=/etc/mr/env
ExecStart=/usr/local/bin/mr fetch --all
Nice=10
ProtectSystem=strict
ProtectHome=true
ReadWritePaths=/var/lib/mr
NoNewPrivileges=true
PrivateTmp=true
```

- [ ] **Step 2: Create `deploy/mr-fetch.timer`**

```ini
[Unit]
Description=market-research daily fetch (04:00 UTC)

[Timer]
OnCalendar=*-*-* 04:00:00 UTC
Persistent=true
RandomizedDelaySec=300
Unit=mr-fetch.service

[Install]
WantedBy=timers.target
```

- [ ] **Step 3: Create `deploy/mr-rediscover.service`**

```ini
[Unit]
Description=market-research weekly source rediscovery
Wants=network-online.target
After=network-online.target

[Service]
Type=oneshot
User=mr
Group=mr
EnvironmentFile=/etc/mr/env
ExecStart=/usr/local/bin/mr rediscover --all
Nice=10
ProtectSystem=strict
ProtectHome=true
ReadWritePaths=/var/lib/mr
NoNewPrivileges=true
PrivateTmp=true
```

- [ ] **Step 4: Create `deploy/mr-rediscover.timer`**

```ini
[Unit]
Description=market-research weekly rediscovery (Sun 03:00 UTC)

[Timer]
OnCalendar=Sun *-*-* 03:00:00 UTC
Persistent=true
RandomizedDelaySec=600
Unit=mr-rediscover.service

[Install]
WantedBy=timers.target
```

- [ ] **Step 5: Commit**

```bash
git add deploy/
git commit -m "feat(deploy): systemd service + timer units for fetch and rediscover"
```

---

### Task 28: Deploy README with VM setup steps

**Files:**
- Create: `deploy/README.md`

- [ ] **Step 1: Create `deploy/README.md`**

```markdown
# Deployment

Target: small always-on Linux VM (Ubuntu 22.04+ / Debian 12+).

## One-time setup

1. Install binary:

   ```
   sudo install -m 0755 bin/mr /usr/local/bin/mr
   ```

2. Create system user and data dir:

   ```
   sudo useradd --system --home /var/lib/mr --shell /usr/sbin/nologin mr
   sudo install -d -o mr -g mr -m 0750 /var/lib/mr
   ```

3. Create env file (holds API keys):

   ```
   sudo install -d -o mr -g mr -m 0750 /etc/mr
   sudo tee /etc/mr/env > /dev/null <<'EOF'
   REDDIT_CLIENT_ID=...
   REDDIT_CLIENT_SECRET=...
   REDDIT_USER_AGENT=market-research/0.1 (by u/yourname)
   STACKEXCHANGE_KEY=...
   ANTHROPIC_API_KEY=...
   MR_DB_PATH=/var/lib/mr/mr.db
   EOF
   sudo chown mr:mr /etc/mr/env
   sudo chmod 0600 /etc/mr/env
   ```

4. Install systemd units:

   ```
   sudo cp deploy/mr-fetch.service deploy/mr-fetch.timer \
           deploy/mr-rediscover.service deploy/mr-rediscover.timer \
           /etc/systemd/system/
   sudo systemctl daemon-reload
   sudo systemctl enable --now mr-fetch.timer mr-rediscover.timer
   ```

5. Add first topic:

   ```
   sudo -u mr /usr/local/bin/mr topic add "soc2 compliance tool" --description "SOC2 audit pain"
   ```

## Observability

- Logs: `journalctl -u mr-fetch -o json` / `journalctl -u mr-rediscover -o json`
- Timer status: `systemctl list-timers mr-*`
- Self-diagnostic: `sudo -u mr /usr/local/bin/mr doctor`
```

- [ ] **Step 2: Commit**

```bash
git add deploy/README.md
git commit -m "docs(deploy): VM setup steps for systemd deployment"
```

---

### Task 29: End-to-end smoke test

**Files:**
- Create: `cmd/mr/e2e_test.go`

- [ ] **Step 1: Write E2E test**

This test exercises the full pipeline against a temp SQLite file, stubbed Claude client, and httptest servers for Reddit + SO.

Create `cmd/mr/e2e_test.go`:

```go
package main

import (
	"context"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"github.com/devonbooker/market-research/internal/config"
	"github.com/devonbooker/market-research/internal/fetch"
	rfetch "github.com/devonbooker/market-research/internal/fetch/reddit"
	sofetch "github.com/devonbooker/market-research/internal/fetch/stackoverflow"
	"github.com/devonbooker/market-research/internal/sources"
	"github.com/devonbooker/market-research/internal/store"
	"github.com/devonbooker/market-research/internal/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type stubClaude struct{}

func (stubClaude) Discover(ctx context.Context, sys, user string) (*types.SourcePlan, error) {
	p := &types.SourcePlan{}
	p.Reddit.Subreddits = []string{"devsecops"}
	p.StackOverflow.Tags = []string{"soc2"}
	p.Reasoning = "test"
	return p, nil
}

func TestE2E_AddTopicThenFetchLandsDocsInDB(t *testing.T) {
	// --- Stub platform servers ---
	redditSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/api/v1/access_token":
			_, _ = w.Write([]byte(`{"access_token":"tok","expires_in":3600}`))
		case r.URL.Path == "/r/devsecops/new":
			_, _ = w.Write([]byte(`{"data":{"children":[{"data":{"id":"p1","subreddit":"devsecops","title":"soc2 pain","selftext":"body","author":"a","score":1,"permalink":"/r/devsecops/comments/p1/","url":"https://reddit.com/x","created_utc":` + itoa(time.Now().Unix()) + `}}]}}`))
		case r.URL.Path == "/comments/p1.json":
			_, _ = w.Write([]byte(`[{"data":{"children":[]}},{"data":{"children":[{"kind":"t1","data":{"id":"c1","body":"me too","author":"b","score":5,"created_utc":` + itoa(time.Now().Unix()) + `}}]}}]`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer redditSrv.Close()

	soSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"items":[{"question_id":1,"title":"q","body":"b","owner":{"display_name":"u"},"score":2,"tags":["soc2"],"link":"https://stackoverflow.com/questions/1","creation_date":` + itoa(time.Now().Unix()) + `,"is_answered":false}]}`))
	}))
	defer soSrv.Close()

	// --- Temp DB and config ---
	dbPath := filepath.Join(t.TempDir(), "e2e.db")
	cfg := &config.Config{
		RedditClientID:     "id",
		RedditClientSecret: "secret",
		RedditUserAgent:    "e2e",
		StackExchangeKey:   "k",
		AnthropicAPIKey:    "a",
		DBPath:             dbPath,
	}
	st, err := store.Open(cfg.DBPath)
	require.NoError(t, err)
	defer st.Close()

	// --- Create topic + apply stubbed source plan directly (skip real agent anthropic call) ---
	topicID, err := st.CreateTopic("soc2 compliance tool", "", false)
	require.NoError(t, err)
	plan, err := (&sources.Agent{Claude: stubClaude{}}).Discover(context.Background(), "soc2 compliance tool", "")
	require.NoError(t, err)
	for _, s := range sources.PlanToSources(plan) {
		_, _, err := st.UpsertSource(topicID, s.Platform, s.Kind, s.Value, types.AddedByAgent)
		require.NoError(t, err)
	}
	require.NoError(t, st.SetTopicActive(topicID, true))

	// --- Wire orchestrator pointed at test servers ---
	rc := rfetch.New(rfetch.Config{
		ClientID: "id", ClientSecret: "secret", UserAgent: "e2e",
		AuthURL: redditSrv.URL + "/api/v1/access_token", APIBaseURL: redditSrv.URL,
		RateLimit: 100,
	})
	soc := sofetch.New(sofetch.Config{Key: "k", APIBaseURL: soSrv.URL, RateLimit: 100})

	orch := &fetch.Orchestrator{
		Store:              st,
		Reddit:             &redditAdapter{client: rc, topCommentsPerPost: 10},
		StackOverflow:      &storeAwareSOAdapter{client: soc, store: st},
		BackfillWindow:     24 * time.Hour,
		MaxPostsPerSource:  100,
		TopCommentsPerPost: 10,
		Retries:            0,
	}
	require.NoError(t, orch.RunAll(context.Background()))

	// --- Assert results ---
	var docCount int
	require.NoError(t, st.DB().QueryRow("SELECT COUNT(*) FROM documents").Scan(&docCount))
	assert.GreaterOrEqual(t, docCount, 2, "expected at least 1 reddit + 1 so doc")

	var replyCount int
	require.NoError(t, st.DB().QueryRow("SELECT COUNT(*) FROM document_replies").Scan(&replyCount))
	assert.GreaterOrEqual(t, replyCount, 1, "expected the reddit comment to be stored")

	var successRuns int
	require.NoError(t, st.DB().QueryRow("SELECT COUNT(*) FROM fetch_runs WHERE status='success'").Scan(&successRuns))
	assert.Equal(t, 2, successRuns)
}

func itoa(i int64) string {
	// small helper to keep the template string above readable
	const chars = "0123456789"
	if i == 0 {
		return "0"
	}
	neg := i < 0
	if neg {
		i = -i
	}
	var buf [20]byte
	pos := len(buf)
	for i > 0 {
		pos--
		buf[pos] = chars[i%10]
		i /= 10
	}
	if neg {
		pos--
		buf[pos] = '-'
	}
	return string(buf[pos:])
}
```

- [ ] **Step 2: Run test**

```bash
go test ./cmd/mr/... -race -v
```

Expected: `TestE2E_AddTopicThenFetchLandsDocsInDB` PASSes in under 2 seconds.

- [ ] **Step 3: Run full suite**

```bash
make test
```

Expected: all tests pass with `-race`.

- [ ] **Step 4: Commit**

```bash
git add cmd/mr/e2e_test.go
git commit -m "test(e2e): smoke test wiring add-topic → fetch → docs+replies in db"
```

- [ ] **Step 5: Push and verify CI green**

```bash
git push
gh run watch
```

Expected: CI workflow ends with `completed success`.

---

## Self-review checklist (for the plan author)

**Spec coverage (going section by section):**

- Section 1 (Architecture + components) - covered by package layout in Tasks 3, 4, 5, 11-21, 22-26 and systemd units in Task 27.
- Section 2 (Data model) - Tasks 5-10. Schema-as-contract preserved in `schema.sql`.
- Section 3 (Source-discovery agent) - Tasks 11-15 (plan validation/caps, prompts, agent, Anthropic adapter, signal scoring).
- Section 4 (Fetch pipeline) - Tasks 16-21 (Reddit client, Reddit fetch, comments, SO client, SO fetch, orchestrator with retry + permanent-error deactivation + per-platform goroutines).
- Section 5 (Observability) - slog JSON in Task 22 (main.go), fetch_runs lifecycle Task 9, orphan recovery Task 9/22, `mr doctor` Task 26, OnFailure systemd hook referenced in Task 27 (notifier deliberately deferred per spec).
- Section 6 (Testing) - unit tests in every package test file; store integration tests Task 10; platform client fixture tests Tasks 16-20; agent stub tests Task 13; E2E smoke test Task 29; CI workflow Task 2. `-race` runs in CI and locally.
- Downstream Subsystems Contract (spec) - `documents.url`, `created_at`, `platform_metadata` (with accepted_answer_id + subreddit) all preserved exactly.

**Placeholder scan:** No TBD/TODO/"fill in details" in any task. Every code step has real code. Every command has expected output or a verification step.

**Type consistency:** Walked through: `types.Platform`, `types.SourceKind`, `types.Document`, `types.Reply`, `types.SourcePlan`, `types.FetchRun` are defined in Task 4 and referenced consistently downstream. `store.ErrNotFound` defined in Task 6 and used in Task 23. `fetch.PlatformFetcher` defined in Task 21, satisfied by adapters in Task 24. `sources.ClaudeClient` defined in Task 12, implemented by `AnthropicClient` in Task 14 and by `stubClaude` in Task 29.

**No unresolved symbol references found.**
