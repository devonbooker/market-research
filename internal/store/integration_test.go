package store

import (
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/devonbooker/market-research/internal/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCascade_DeleteTopic_WipesSourcesAndDocs(t *testing.T) {
	s := openTest(t)
	topicID, _ := s.CreateTopic("t", "", true)
	sourceID, _, _ := s.UpsertSource(topicID, types.PlatformReddit, types.SourceKindSubreddit, "x", types.AddedByAgent)
	_, _, _ = s.UpsertDocument(&types.Document{
		TopicID: topicID, SourceID: sourceID,
		Platform: types.PlatformReddit, PlatformID: "p1",
		Title: "t", URL: "u", CreatedAt: time.Now().UTC(), FetchedAt: time.Now().UTC(),
	})

	require.NoError(t, s.DeleteTopic(topicID))

	var docCount int
	require.NoError(t, s.DB().QueryRow("SELECT COUNT(*) FROM documents").Scan(&docCount))
	assert.Equal(t, 0, docCount)

	var srcCount int
	require.NoError(t, s.DB().QueryRow("SELECT COUNT(*) FROM sources").Scan(&srcCount))
	assert.Equal(t, 0, srcCount)
}

func TestConcurrentUpserts_NoDuplicates(t *testing.T) {
	dir := t.TempDir()
	s, err := Open(filepath.Join(dir, "t.db"))
	require.NoError(t, err)
	defer s.Close()

	topicID, _ := s.CreateTopic("t", "", true)
	sourceID, _, _ := s.UpsertSource(topicID, types.PlatformReddit, types.SourceKindSubreddit, "x", types.AddedByAgent)

	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, _, err := s.UpsertDocument(&types.Document{
				TopicID: topicID, SourceID: sourceID,
				Platform: types.PlatformReddit, PlatformID: "same",
				Title: "t", URL: "u",
				CreatedAt: time.Now().UTC(), FetchedAt: time.Now().UTC(),
			})
			assert.NoError(t, err)
		}()
	}
	wg.Wait()

	var n int
	require.NoError(t, s.DB().QueryRow("SELECT COUNT(*) FROM documents").Scan(&n))
	assert.Equal(t, 1, n)
}
