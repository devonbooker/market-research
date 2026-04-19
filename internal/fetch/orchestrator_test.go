package fetch

import (
	"context"
	"errors"
	"testing"
	"time"

	redditc "github.com/devonbooker/market-research/internal/fetch/reddit"
	"github.com/devonbooker/market-research/internal/store"
	"github.com/devonbooker/market-research/internal/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type stubFetcher struct {
	docs    []*types.Document
	replies map[string][]*types.Reply
	err     error
}

func (s *stubFetcher) FetchDocuments(ctx context.Context, src *types.Source, since time.Time, max int) ([]*types.Document, error) {
	if s.err != nil {
		return nil, s.err
	}
	return s.docs, nil
}
func (s *stubFetcher) FetchReplies(ctx context.Context, platformID string) ([]*types.Reply, error) {
	return s.replies[platformID], nil
}

func openStore(t *testing.T) *store.Store {
	t.Helper()
	s, err := store.Open(":memory:")
	require.NoError(t, err)
	t.Cleanup(func() { s.Close() })
	return s
}

func TestOrchestrator_HappyPath(t *testing.T) {
	st := openStore(t)
	topicID, _ := st.CreateTopic("t", "", true)
	srcID, _, _ := st.UpsertSource(topicID, types.PlatformReddit, types.SourceKindSubreddit, "x", types.AddedByAgent)

	now := time.Now().UTC()
	doc := &types.Document{
		TopicID: topicID, SourceID: srcID,
		Platform: types.PlatformReddit, PlatformID: "p1",
		Title: "t", URL: "u", CreatedAt: now, FetchedAt: now,
	}
	f := &stubFetcher{
		docs:    []*types.Document{doc},
		replies: map[string][]*types.Reply{"p1": {{PlatformID: "c1", Body: "b", CreatedAt: now}}},
	}

	o := &Orchestrator{
		Store:        st,
		Reddit:       f,
		StackOverflow: &stubFetcher{},
		BackfillWindow: 7 * 24 * time.Hour,
		MaxPostsPerSource: 100,
		TopCommentsPerPost: 10,
	}

	err := o.RunAll(context.Background())
	require.NoError(t, err)

	var docCount int
	require.NoError(t, st.DB().QueryRow("SELECT COUNT(*) FROM documents").Scan(&docCount))
	assert.Equal(t, 1, docCount)

	var replyCount int
	require.NoError(t, st.DB().QueryRow("SELECT COUNT(*) FROM document_replies").Scan(&replyCount))
	assert.Equal(t, 1, replyCount)

	var runCount int
	require.NoError(t, st.DB().QueryRow("SELECT COUNT(*) FROM fetch_runs WHERE status='success'").Scan(&runCount))
	assert.Equal(t, 2, runCount) // one per platform
}

func TestOrchestrator_PermanentErrorDeactivatesSource(t *testing.T) {
	st := openStore(t)
	topicID, _ := st.CreateTopic("t", "", true)
	srcID, _, _ := st.UpsertSource(topicID, types.PlatformReddit, types.SourceKindSubreddit, "x", types.AddedByAgent)

	f := &stubFetcher{err: &redditc.HTTPError{Status: 404, Body: "gone"}}
	o := &Orchestrator{
		Store: st, Reddit: f, StackOverflow: &stubFetcher{},
		BackfillWindow: 7 * 24 * time.Hour, MaxPostsPerSource: 100, TopCommentsPerPost: 10,
		Retries: 1,
	}
	require.NoError(t, o.RunAll(context.Background()))

	src, err := st.GetSource(srcID)
	require.NoError(t, err)
	assert.False(t, src.Active, "source should be deactivated on 404")
}

func TestOrchestrator_TransientErrorRetriedThenFails(t *testing.T) {
	st := openStore(t)
	topicID, _ := st.CreateTopic("t", "", true)
	_, _, _ = st.UpsertSource(topicID, types.PlatformReddit, types.SourceKindSubreddit, "x", types.AddedByAgent)

	f := &stubFetcher{err: errors.New("network blip")}
	o := &Orchestrator{
		Store: st, Reddit: f, StackOverflow: &stubFetcher{},
		BackfillWindow: 7 * 24 * time.Hour, MaxPostsPerSource: 100, TopCommentsPerPost: 10,
		Retries: 2, BackoffBase: time.Millisecond,
	}
	require.NoError(t, o.RunAll(context.Background()))

	var errorRuns int
	require.NoError(t, st.DB().QueryRow("SELECT COUNT(*) FROM fetch_runs WHERE status='error'").Scan(&errorRuns))
	assert.GreaterOrEqual(t, errorRuns, 1)
}
