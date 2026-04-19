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
