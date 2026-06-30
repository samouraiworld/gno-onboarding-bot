package sheet

import (
	"context"
	"errors"
	"io"
	"log"
	"math/rand"
	"net"
	"net/http"
	"strconv"
	"syscall"
	"time"

	"google.golang.org/api/googleapi"
)

// RetryPolicy controls how the client retries rate-limited (HTTP 429) and transient (5xx) Sheets responses.
type RetryPolicy struct {
	MaxAttempts int           // total attempts including the first
	BaseDelay   time.Duration // first backoff delay
	MaxDelay    time.Duration // cap on the backoff delay
}

// DefaultRetryPolicy spans a few quota minutes (cumulative ~1.5-3min of backoff)
// so a busy harvest reliably outlasts the Sheets per-minute write quota window.
var DefaultRetryPolicy = RetryPolicy{
	MaxAttempts: 8,
	BaseDelay:   2 * time.Second,
	MaxDelay:    60 * time.Second,
}

// normalized fills any unset field from DefaultRetryPolicy and keeps MaxDelay >= BaseDelay.
func (p RetryPolicy) normalized() RetryPolicy {
	if p.MaxAttempts < 1 {
		p.MaxAttempts = DefaultRetryPolicy.MaxAttempts
	}
	if p.BaseDelay <= 0 {
		p.BaseDelay = DefaultRetryPolicy.BaseDelay
	}
	if p.MaxDelay <= 0 {
		p.MaxDelay = DefaultRetryPolicy.MaxDelay
	}
	if p.MaxDelay < p.BaseDelay {
		p.MaxDelay = p.BaseDelay
	}
	return p
}

// do runs op, retrying retryable errors with exponential backoff plus jitter, until success, attempts run out, or ctx is cancelled.
func (c *GoogleSheetsClient) do(ctx context.Context, op func() error) error {
	policy := c.retry
	delay := policy.BaseDelay
	var err error
	for attempt := 1; attempt <= policy.MaxAttempts; attempt++ {
		err = op()
		if err == nil || !isRetryable(err) || attempt == policy.MaxAttempts {
			return err
		}
		wait := jitter(delay)
		// Honor the server's Retry-After hint when it asks for a longer wait,
		// then keep the whole backoff bounded by MaxDelay.
		if ra := retryAfter(err); ra > wait {
			wait = ra
		}
		if wait > policy.MaxDelay {
			wait = policy.MaxDelay
		}
		log.Printf("sheets: retryable error (attempt %d/%d), backing off %s: %v",
			attempt, policy.MaxAttempts, wait.Round(time.Millisecond), err)
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(wait):
		}
		delay *= 2
		if delay > policy.MaxDelay {
			delay = policy.MaxDelay
		}
	}
	return err
}

// jitter returns d scaled by a random factor in [0.5, 1.0], to avoid retry stampedes.
func jitter(d time.Duration) time.Duration {
	return d/2 + time.Duration(rand.Int63n(int64(d/2)+1))
}

// isRetryable reports whether err is a Sheets rate-limit (429), transient
// server error (5xx), or transient transport failure (timeout, connection
// reset, unexpected EOF).
func isRetryable(err error) bool {
	// A cancelled/expired context is terminal, not transient.
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return false
	}
	var gerr *googleapi.Error
	if errors.As(err, &gerr) {
		return gerr.Code == http.StatusTooManyRequests || gerr.Code >= 500
	}
	// Transport failures aren't *googleapi.Error, so match them directly.
	var nerr net.Error
	if errors.As(err, &nerr) && nerr.Timeout() {
		return true
	}
	return errors.Is(err, io.ErrUnexpectedEOF) || errors.Is(err, syscall.ECONNRESET)
}

// retryAfter reads the response's Retry-After header (integer seconds or an
// HTTP date) and returns it as a positive duration, or 0 when absent, in the
// past, or unparseable.
func retryAfter(err error) time.Duration {
	var gerr *googleapi.Error
	if !errors.As(err, &gerr) || gerr.Header == nil {
		return 0
	}
	v := gerr.Header.Get("Retry-After")
	if v == "" {
		return 0
	}
	if secs, e := strconv.Atoi(v); e == nil {
		if secs <= 0 {
			return 0
		}
		return time.Duration(secs) * time.Second
	}
	if t, e := http.ParseTime(v); e == nil {
		if d := time.Until(t); d > 0 {
			return d
		}
	}
	return 0
}
