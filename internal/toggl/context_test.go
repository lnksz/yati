package toggl

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestClientContextCancellation(t *testing.T) {
	client := NewClient("test-token")
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := client.WithContext(ctx).GetMe()
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("request did not cancel: %v", err)
	}
	if client.ctx != nil {
		t.Fatal("WithContext mutated the shared client")
	}
}

func TestQuotaErrorPreservesRetryHeader(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Toggl-Quota-Resets-In", "120")
		w.WriteHeader(http.StatusPaymentRequired)
	}))
	defer server.Close()
	client := NewClient("test-token")
	client.baseURL = server.URL
	_, err := client.GetMe()
	var apiErr *APIError
	if !errors.As(err, &apiErr) || apiErr.RetryAfter != 2*time.Minute {
		t.Fatalf("lost quota reset: %v", err)
	}
}
