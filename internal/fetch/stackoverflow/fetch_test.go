package stackoverflow

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/devonbooker/market-research/internal/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFetchQuestionsByTag_ParsesAndFiltersBySince(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Contains(t, r.URL.Path, "/questions")
		data, err := os.ReadFile("testdata/questions_tag.json")
		require.NoError(t, err)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(data)
	}))
	defer srv.Close()

	c := New(Config{Key: "k", APIBaseURL: srv.URL, RateLimit: 100})
	since := time.Unix(1710000000, 0).UTC()
	docs, err := FetchQuestionsByTag(context.Background(), c, "soc2", since, 100)
	require.NoError(t, err)
	require.Len(t, docs, 2)

	d := docs[0]
	assert.Equal(t, types.PlatformStackOverflow, d.Platform)
	assert.Equal(t, "12345", d.PlatformID)
	assert.Contains(t, d.Title, "SOC2 audit")
	assert.Equal(t, "alice", d.Author)
	assert.Equal(t, 7, d.Score)
	assert.Equal(t, "https://stackoverflow.com/questions/12345", d.URL)
	assert.Contains(t, string(d.PlatformMetadata), "soc2")
	assert.Contains(t, string(d.PlatformMetadata), "accepted_answer_id")
}

func TestFetchAcceptedAnswer_ReturnsReplyForAcceptedOnly(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.True(t, strings.Contains(r.URL.Path, "/answers/"))
		data, err := os.ReadFile("testdata/answers.json")
		require.NoError(t, err)
		_, _ = w.Write(data)
	}))
	defer srv.Close()

	c := New(Config{Key: "k", APIBaseURL: srv.URL, RateLimit: 100})
	reply, err := FetchAcceptedAnswer(context.Background(), c, 99999)
	require.NoError(t, err)
	require.NotNil(t, reply)
	assert.Equal(t, "99999", reply.PlatformID)
	assert.Equal(t, 18, reply.Score)
	require.NotNil(t, reply.IsAccepted)
	assert.True(t, *reply.IsAccepted)
}
