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
