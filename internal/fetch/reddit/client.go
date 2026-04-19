package reddit

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"golang.org/x/time/rate"
)

type Config struct {
	ClientID     string
	ClientSecret string
	UserAgent    string
	AuthURL      string // defaults to Reddit production
	APIBaseURL   string // defaults to Reddit production
	RateLimit    float64 // requests per second
}

type HTTPError struct {
	Status int
	Body   string
}

func (e *HTTPError) Error() string { return fmt.Sprintf("reddit http %d: %s", e.Status, e.Body) }

type Client struct {
	cfg     Config
	http    *http.Client
	limiter *rate.Limiter

	mu         sync.Mutex
	token      string
	tokenExp   time.Time
}

func New(cfg Config) *Client {
	if cfg.AuthURL == "" {
		cfg.AuthURL = "https://www.reddit.com/api/v1/access_token"
	}
	if cfg.APIBaseURL == "" {
		cfg.APIBaseURL = "https://oauth.reddit.com"
	}
	if cfg.RateLimit == 0 {
		cfg.RateLimit = 50.0 / 60.0 // 50 req/min = ~0.83 req/s
	}
	return &Client{
		cfg:     cfg,
		http:    &http.Client{Timeout: 30 * time.Second},
		limiter: rate.NewLimiter(rate.Limit(cfg.RateLimit), 1),
	}
}

func (c *Client) ensureToken(ctx context.Context) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.token != "" && time.Now().Before(c.tokenExp.Add(-30*time.Second)) {
		return nil
	}
	body := strings.NewReader("grant_type=client_credentials")
	req, err := http.NewRequestWithContext(ctx, "POST", c.cfg.AuthURL, body)
	if err != nil {
		return err
	}
	req.SetBasicAuth(c.cfg.ClientID, c.cfg.ClientSecret)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("User-Agent", c.cfg.UserAgent)

	resp, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		b, _ := io.ReadAll(resp.Body)
		return &HTTPError{Status: resp.StatusCode, Body: string(b)}
	}
	var tok struct {
		AccessToken string `json:"access_token"`
		ExpiresIn   int    `json:"expires_in"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&tok); err != nil {
		return err
	}
	c.token = tok.AccessToken
	c.tokenExp = time.Now().Add(time.Duration(tok.ExpiresIn) * time.Second)
	return nil
}

func (c *Client) GetJSON(ctx context.Context, path string, out any) error {
	if err := c.limiter.Wait(ctx); err != nil {
		return err
	}
	if err := c.ensureToken(ctx); err != nil {
		return err
	}

	u, err := url.Parse(c.cfg.APIBaseURL + path)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, "GET", u.String(), nil)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+c.token)
	req.Header.Set("User-Agent", c.cfg.UserAgent)

	resp, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		b, _ := io.ReadAll(resp.Body)
		return &HTTPError{Status: resp.StatusCode, Body: string(b)}
	}
	return json.NewDecoder(resp.Body).Decode(out)
}
