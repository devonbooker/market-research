package stackoverflow

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"

	"golang.org/x/time/rate"
)

type Config struct {
	Key        string
	APIBaseURL string
	RateLimit  float64 // requests per second
	Site       string  // defaults to "stackoverflow"
}

type HTTPError struct {
	Status int
	Body   string
}

func (e *HTTPError) Error() string { return fmt.Sprintf("stackexchange http %d: %s", e.Status, e.Body) }

type Client struct {
	cfg     Config
	http    *http.Client
	limiter *rate.Limiter
}

func New(cfg Config) *Client {
	if cfg.APIBaseURL == "" {
		cfg.APIBaseURL = "https://api.stackexchange.com/2.3"
	}
	if cfg.Site == "" {
		cfg.Site = "stackoverflow"
	}
	if cfg.RateLimit == 0 {
		cfg.RateLimit = 1.0
	}
	return &Client{
		cfg:     cfg,
		http:    &http.Client{Timeout: 30 * time.Second},
		limiter: rate.NewLimiter(rate.Limit(cfg.RateLimit), 1),
	}
}

func (c *Client) GetJSON(ctx context.Context, path string, params url.Values, out any) error {
	if err := c.limiter.Wait(ctx); err != nil {
		return err
	}
	if params == nil {
		params = url.Values{}
	}
	params.Set("key", c.cfg.Key)
	params.Set("site", c.cfg.Site)

	u, err := url.Parse(c.cfg.APIBaseURL + path)
	if err != nil {
		return err
	}
	u.RawQuery = params.Encode()

	req, err := http.NewRequestWithContext(ctx, "GET", u.String(), nil)
	if err != nil {
		return err
	}
	req.Header.Set("Accept", "application/json")

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
