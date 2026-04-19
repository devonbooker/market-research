package config

import (
	"fmt"
	"os"
)

type Config struct {
	RedditClientID     string
	RedditClientSecret string
	RedditUserAgent    string
	StackExchangeKey   string
	AnthropicAPIKey    string
	DBPath             string
}

func Load() (*Config, error) {
	cfg := &Config{
		RedditClientID:     os.Getenv("REDDIT_CLIENT_ID"),
		RedditClientSecret: os.Getenv("REDDIT_CLIENT_SECRET"),
		RedditUserAgent:    os.Getenv("REDDIT_USER_AGENT"),
		StackExchangeKey:   os.Getenv("STACKEXCHANGE_KEY"),
		AnthropicAPIKey:    os.Getenv("ANTHROPIC_API_KEY"),
		DBPath:             os.Getenv("MR_DB_PATH"),
	}

	required := map[string]string{
		"REDDIT_CLIENT_ID":     cfg.RedditClientID,
		"REDDIT_CLIENT_SECRET": cfg.RedditClientSecret,
		"STACKEXCHANGE_KEY":    cfg.StackExchangeKey,
		"ANTHROPIC_API_KEY":    cfg.AnthropicAPIKey,
	}
	for name, val := range required {
		if val == "" {
			return nil, fmt.Errorf("%s is required", name)
		}
	}

	if cfg.DBPath == "" {
		cfg.DBPath = "/var/lib/mr/mr.db"
	}
	if cfg.RedditUserAgent == "" {
		cfg.RedditUserAgent = "market-research/0.1 (by u/unknown)"
	}

	return cfg, nil
}
