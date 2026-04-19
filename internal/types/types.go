package types

import (
	"encoding/json"
	"time"
)

type Platform string

const (
	PlatformReddit        Platform = "reddit"
	PlatformStackOverflow Platform = "stackoverflow"
)

type SourceKind string

const (
	SourceKindSubreddit   SourceKind = "subreddit"
	SourceKindSOTag       SourceKind = "so_tag"
	SourceKindSearchQuery SourceKind = "search_query"
)

type AddedBy string

const (
	AddedByAgent  AddedBy = "agent"
	AddedByManual AddedBy = "manual"
)

type RunStatus string

const (
	RunStatusRunning RunStatus = "running"
	RunStatusSuccess RunStatus = "success"
	RunStatusError   RunStatus = "error"
)

type Topic struct {
	ID          int64
	Name        string
	Description string
	CreatedAt   time.Time
	Active      bool
}

type Source struct {
	ID           int64
	TopicID      int64
	Platform     Platform
	Kind         SourceKind
	Value        string
	AddedAt      time.Time
	AddedBy      AddedBy
	LastFetched  *time.Time
	SignalScore  *float64
	Active       bool
}

type Document struct {
	ID               int64
	TopicID          int64
	SourceID         int64
	Platform         Platform
	PlatformID       string
	Title            string
	Body             string
	Author           string
	Score            int
	URL              string
	CreatedAt        time.Time
	FetchedAt        time.Time
	PlatformMetadata json.RawMessage
}

type Reply struct {
	ID         int64
	DocumentID int64
	PlatformID string
	Body       string
	Author     string
	Score      int
	CreatedAt  time.Time
	IsAccepted *bool
}

type FetchRun struct {
	ID            int64
	TopicID       int64
	Platform      Platform
	StartedAt     time.Time
	EndedAt       *time.Time
	Status        RunStatus
	DocumentsNew  int
	RepliesNew    int
	ErrorMessage  string
}

type SourcePlan struct {
	Reddit struct {
		Subreddits     []string `json:"subreddits"`
		SearchQueries  []string `json:"search_queries"`
	} `json:"reddit"`
	StackOverflow struct {
		Tags          []string `json:"tags"`
		SearchQueries []string `json:"search_queries"`
	} `json:"stackoverflow"`
	Reasoning string `json:"reasoning"`
}
