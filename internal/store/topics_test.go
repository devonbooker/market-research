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
