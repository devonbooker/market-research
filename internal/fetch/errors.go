package fetch

import (
	"errors"

	redditc "github.com/devonbooker/market-research/internal/fetch/reddit"
	soc "github.com/devonbooker/market-research/internal/fetch/stackoverflow"
)

// IsTransient returns true for retryable HTTP failures (5xx, 429) or network errors.
func IsTransient(err error) bool {
	var rhe *redditc.HTTPError
	if errors.As(err, &rhe) {
		return rhe.Status >= 500 || rhe.Status == 429
	}
	var she *soc.HTTPError
	if errors.As(err, &she) {
		return she.Status >= 500 || she.Status == 429
	}
	// network errors, context errors, etc: treat as transient
	return err != nil
}

// IsPermanent returns true for 403/404/410, signaling the source should be deactivated.
func IsPermanent(err error) bool {
	var rhe *redditc.HTTPError
	if errors.As(err, &rhe) {
		return rhe.Status == 403 || rhe.Status == 404 || rhe.Status == 410
	}
	var she *soc.HTTPError
	if errors.As(err, &she) {
		return she.Status == 403 || she.Status == 404 || she.Status == 410
	}
	return false
}
