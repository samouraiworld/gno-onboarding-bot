package sheet

import (
	"context"
	"errors"
	"io"
	"net"
	"net/http"
	"net/url"
	"syscall"
	"testing"
	"time"

	"google.golang.org/api/googleapi"
)

// timeoutErr is a net.Error reporting Timeout() == true, like a transport read deadline.
type timeoutErr struct{}

func (timeoutErr) Error() string   { return "i/o timeout" }
func (timeoutErr) Timeout() bool   { return true }
func (timeoutErr) Temporary() bool { return true }

func TestIsRetryable(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want bool
	}{
		{"nil", nil, false},
		{"429", &googleapi.Error{Code: 429}, true},
		{"500", &googleapi.Error{Code: 500}, true},
		{"503", &googleapi.Error{Code: 503}, true},
		{"400", &googleapi.Error{Code: 400}, false},
		{"403", &googleapi.Error{Code: 403}, false},
		{"404", &googleapi.Error{Code: 404}, false},
		{"wrapped 429", errors.Join(errors.New("update Z47"), &googleapi.Error{Code: 429}), true},
		{"plain error", errors.New("boom"), false},
		{"timeout net.Error", &net.OpError{Op: "read", Err: timeoutErr{}}, true},
		{"connection reset", &net.OpError{Op: "read", Err: syscall.ECONNRESET}, true},
		{"unexpected EOF", &url.Error{Op: "Get", Err: io.ErrUnexpectedEOF}, true},
		{"context canceled", context.Canceled, false},
		{"context deadline (timeout)", &url.Error{Op: "Get", Err: context.DeadlineExceeded}, false},
	}
	for _, tt := range tests {
		if got := isRetryable(tt.err); got != tt.want {
			t.Errorf("isRetryable(%s) = %v, want %v", tt.name, got, tt.want)
		}
	}
}

func TestRetryAfter(t *testing.T) {
	withHeader := func(v string) error {
		h := http.Header{}
		h.Set("Retry-After", v)
		return &googleapi.Error{Code: 429, Header: h}
	}
	if got := retryAfter(withHeader("30")); got != 30*time.Second {
		t.Errorf("integer seconds: got %s, want 30s", got)
	}
	if got := retryAfter(withHeader("0")); got != 0 {
		t.Errorf("zero seconds: got %s, want 0", got)
	}
	if got := retryAfter(withHeader("-5")); got != 0 {
		t.Errorf("negative seconds: got %s, want 0", got)
	}
	if got := retryAfter(withHeader("garbage")); got != 0 {
		t.Errorf("garbage: got %s, want 0", got)
	}
	future := time.Now().Add(45 * time.Second).UTC().Format(http.TimeFormat)
	if got := retryAfter(withHeader(future)); got <= 0 {
		t.Errorf("future HTTP date: got %s, want positive", got)
	}
	past := time.Now().Add(-time.Minute).UTC().Format(http.TimeFormat)
	if got := retryAfter(withHeader(past)); got != 0 {
		t.Errorf("past HTTP date: got %s, want 0", got)
	}
	if got := retryAfter(&googleapi.Error{Code: 429}); got != 0 {
		t.Errorf("no header: got %s, want 0", got)
	}
	if got := retryAfter(errors.New("boom")); got != 0 {
		t.Errorf("plain error: got %s, want 0", got)
	}
}

func TestRetryPolicyNormalized(t *testing.T) {
	got := RetryPolicy{}.normalized()
	if got != DefaultRetryPolicy {
		t.Errorf("zero policy normalized = %+v, want default %+v", got, DefaultRetryPolicy)
	}

	// MaxDelay below BaseDelay is bumped up to BaseDelay.
	p := RetryPolicy{MaxAttempts: 3, BaseDelay: 2 * time.Second, MaxDelay: time.Second}.normalized()
	if p.MaxDelay != p.BaseDelay {
		t.Errorf("MaxDelay = %s, want clamped to BaseDelay %s", p.MaxDelay, p.BaseDelay)
	}
}

func TestDoRetriesThenSucceeds(t *testing.T) {
	c := &GoogleSheetsClient{retry: RetryPolicy{MaxAttempts: 4, BaseDelay: time.Millisecond, MaxDelay: time.Millisecond}}
	calls := 0
	err := c.do(context.Background(), func() error {
		calls++
		if calls < 3 {
			return &googleapi.Error{Code: 429}
		}
		return nil
	})
	if err != nil {
		t.Fatalf("do: unexpected error: %v", err)
	}
	if calls != 3 {
		t.Errorf("calls = %d, want 3 (two 429s then success)", calls)
	}
}

func TestDoExhaustsAttempts(t *testing.T) {
	c := &GoogleSheetsClient{retry: RetryPolicy{MaxAttempts: 3, BaseDelay: time.Millisecond, MaxDelay: time.Millisecond}}
	calls := 0
	err := c.do(context.Background(), func() error {
		calls++
		return &googleapi.Error{Code: 429}
	})
	if err == nil {
		t.Fatal("do: expected error after exhausting attempts")
	}
	if calls != 3 {
		t.Errorf("calls = %d, want 3 (MaxAttempts)", calls)
	}
}

func TestDoDoesNotRetryNonRetryable(t *testing.T) {
	c := &GoogleSheetsClient{retry: DefaultRetryPolicy}
	calls := 0
	err := c.do(context.Background(), func() error {
		calls++
		return &googleapi.Error{Code: 400}
	})
	if err == nil {
		t.Fatal("do: expected the 400 error to propagate")
	}
	if calls != 1 {
		t.Errorf("calls = %d, want 1 (no retry on 400)", calls)
	}
}

func TestDoStopsOnContextCancel(t *testing.T) {
	c := &GoogleSheetsClient{retry: RetryPolicy{MaxAttempts: 10, BaseDelay: time.Hour, MaxDelay: time.Hour}}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	calls := 0
	err := c.do(ctx, func() error {
		calls++
		return &googleapi.Error{Code: 429}
	})
	if !errors.Is(err, context.Canceled) {
		t.Errorf("err = %v, want context.Canceled", err)
	}
	if calls != 1 {
		t.Errorf("calls = %d, want 1 (cancel breaks the backoff wait)", calls)
	}
}
