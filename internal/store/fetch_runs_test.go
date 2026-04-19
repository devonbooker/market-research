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
