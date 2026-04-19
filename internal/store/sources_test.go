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
