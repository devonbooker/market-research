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
