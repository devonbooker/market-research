package config

import (
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLoad_RequiresRedditClientID(t *testing.T) {
	t.Setenv("REDDIT_CLIENT_ID", "")
	_, err := Load()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "REDDIT_CLIENT_ID")
}

func TestLoad_AppliesDefaults(t *testing.T) {
	t.Setenv("REDDIT_CLIENT_ID", "id")
	t.Setenv("REDDIT_CLIENT_SECRET", "secret")
	t.Setenv("STACKEXCHANGE_KEY", "k")
	t.Setenv("ANTHROPIC_API_KEY", "a")
	t.Setenv("MR_DB_PATH", "")
	cfg, err := Load()
	require.NoError(t, err)
	assert.Equal(t, "/var/lib/mr/mr.db", cfg.DBPath)
	assert.Equal(t, "market-research/0.1 (by u/unknown)", cfg.RedditUserAgent)
}

func TestLoad_RespectsDBPathOverride(t *testing.T) {
	t.Setenv("REDDIT_CLIENT_ID", "id")
	t.Setenv("REDDIT_CLIENT_SECRET", "secret")
	t.Setenv("STACKEXCHANGE_KEY", "k")
	t.Setenv("ANTHROPIC_API_KEY", "a")
	t.Setenv("MR_DB_PATH", "/tmp/test.db")
	cfg, err := Load()
	require.NoError(t, err)
	assert.Equal(t, "/tmp/test.db", cfg.DBPath)
}

func TestMain(m *testing.M) {
	os.Exit(m.Run())
}
