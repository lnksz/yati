package toggl

import (
	"fmt"
	"net/http"
	"strconv"
	"time"
)

// APIError describes a rejected request, including when a quota allows retrying.
type APIError struct {
	StatusCode int
	Body       string
	RetryAfter time.Duration
}

func (e *APIError) Error() string {
	return fmt.Sprintf("toggl API error: status %d, body: %s", e.StatusCode, e.Body)
}

func responseError(resp *http.Response, body []byte) error {
	err := &APIError{StatusCode: resp.StatusCode, Body: string(body)}
	if seconds, parseErr := strconv.Atoi(resp.Header.Get("X-Toggl-Quota-Resets-In")); parseErr == nil && seconds > 0 {
		err.RetryAfter = time.Duration(seconds) * time.Second
	} else if seconds, parseErr := strconv.Atoi(resp.Header.Get("Retry-After")); parseErr == nil && seconds > 0 {
		err.RetryAfter = time.Duration(seconds) * time.Second
	} else if deadline, parseErr := http.ParseTime(resp.Header.Get("Retry-After")); parseErr == nil {
		err.RetryAfter = max(0, time.Until(deadline))
	}
	return err
}
