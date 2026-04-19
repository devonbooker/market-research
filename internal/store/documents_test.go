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
