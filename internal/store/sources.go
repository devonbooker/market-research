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
