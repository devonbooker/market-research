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
