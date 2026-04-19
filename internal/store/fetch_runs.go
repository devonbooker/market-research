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
