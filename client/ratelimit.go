package client

import (
	"context"
	"fmt"
	"math"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"
)

const (
	rateLimitRetries     = 3
	rateLimitMaxWait     = 10 * time.Second
	rateLimitDefaultWait = 5 * time.Second
	rateLimitMinWait     = 1 * time.Second
	rateLimitMaxSeconds  = 24 * 60 * 60
)

// rateHold is a period during which requests to a host are held back,
// together with the status that caused it.
type rateHold struct {
	until  time.Time
	status int
}

// rateLimit stores the hold per host.
var rateLimit struct {
	holds map[string]rateHold
	mutex sync.Mutex
}

// RateLimitError is returned when a host asks to wait longer than the client waits.
// Status is 429 (rate limited) or 503 (temporarily unavailable).
type RateLimitError struct {
	Host       string
	Status     int
	RetryAfter time.Duration
}

// Error implements the error interface.
func (e *RateLimitError) Error() string {
	if e.Status == http.StatusServiceUnavailable {
		return fmt.Sprintf("%s is temporarily unavailable, please try again in %d seconds", e.Host, e.Seconds())
	}

	return fmt.Sprintf("Rate limited by %s, please try again in %d seconds", e.Host, e.Seconds())
}

// Seconds returns the remaining wait rounded up to whole seconds.
func (e *RateLimitError) Seconds() int {
	return int(math.Ceil(e.RetryAfter.Seconds()))
}

// isRetryable reports whether res asks the client to wait and retry:
// a 429, or a 503 that carries Retry-After.
func isRetryable(res *http.Response) bool {
	switch res.StatusCode {
	case http.StatusTooManyRequests:
		return true

	case http.StatusServiceUnavailable:
		return res.Header.Get("Retry-After") != ""
	}

	return false
}

// doWithRateLimit sends req with c. On a retryable response it waits as requested
// by Retry-After and retries, up to rateLimitRetries times and rateLimitMaxWait per wait.
// The hold is recorded for the host that answered, which may be a redirect target.
func doWithRateLimit(c *http.Client, req *http.Request) (*http.Response, error) {
	host := strings.ToLower(req.URL.Host)

	for attempt := 0; ; attempt++ {
		if err := waitRateLimit(req.Context(), host); err != nil {
			return nil, err
		}

		res, err := c.Do(req)
		if err != nil {
			return nil, err
		}
		if !isRetryable(res) {
			return res, nil
		}
		res.Body.Close()

		limited := host
		if res.Request != nil {
			limited = strings.ToLower(res.Request.URL.Host)
		}

		wait := parseRetryAfter(res.Header.Get("Retry-After"))

		// A 503 holds the host for at most rateLimitMaxWait.
		hold := wait
		if res.StatusCode == http.StatusServiceUnavailable && hold > rateLimitMaxWait {
			hold = rateLimitMaxWait
		}
		setRateLimit(limited, hold, res.StatusCode)

		if attempt >= rateLimitRetries || wait > rateLimitMaxWait ||
			(req.Body != nil && req.GetBody == nil) {
			return nil, &RateLimitError{Host: limited, Status: res.StatusCode, RetryAfter: wait}
		}

		if err := waitRateLimit(req.Context(), limited); err != nil {
			return nil, err
		}

		next := req.Clone(req.Context())
		if req.GetBody != nil {
			if next.Body, err = req.GetBody(); err != nil {
				return nil, err
			}
		}
		req = next
	}
}

// waitRateLimit blocks until host is no longer held or ctx is done.
// It fails at once if the remaining wait exceeds rateLimitMaxWait.
func waitRateLimit(ctx context.Context, host string) error {
	rateLimit.mutex.Lock()
	hold := rateLimit.holds[host]
	rateLimit.mutex.Unlock()

	wait := time.Until(hold.until)
	if wait <= 0 {
		return nil
	}
	if wait > rateLimitMaxWait {
		return &RateLimitError{Host: host, Status: hold.status, RetryAfter: wait}
	}

	timer := time.NewTimer(wait)
	defer timer.Stop()

	select {
	case <-ctx.Done():
		return ctx.Err()

	case <-timer.C:
		return nil
	}
}

// setRateLimit holds back requests to host for wait, keeping any longer hold.
func setRateLimit(host string, wait time.Duration, status int) {
	rateLimit.mutex.Lock()
	defer rateLimit.mutex.Unlock()

	if rateLimit.holds == nil {
		rateLimit.holds = make(map[string]rateHold)
	}

	until := time.Now().Add(wait)
	if until.After(rateLimit.holds[host].until) {
		rateLimit.holds[host] = rateHold{until: until, status: status}
	}
}

// ClearRateLimits removes all holds and reports whether any was active.
func ClearRateLimits() bool {
	rateLimit.mutex.Lock()
	defer rateLimit.mutex.Unlock()

	active := false
	for _, hold := range rateLimit.holds {
		active = active || time.Now().Before(hold.until)
	}
	rateLimit.holds = nil

	return active
}

// clearRateLimit removes the hold for host.
func clearRateLimit(host string) {
	rateLimit.mutex.Lock()
	defer rateLimit.mutex.Unlock()

	delete(rateLimit.holds, strings.ToLower(host))
}

// parseRetryAfter parses a Retry-After value given in seconds or as an HTTP date.
// Missing or invalid values yield rateLimitDefaultWait; the result is at least rateLimitMinWait.
func parseRetryAfter(value string) time.Duration {
	wait := rateLimitDefaultWait

	value = strings.TrimSpace(value)
	if seconds, err := strconv.Atoi(value); err == nil && seconds >= 0 {
		if seconds > rateLimitMaxSeconds {
			seconds = rateLimitMaxSeconds
		}
		wait = time.Duration(seconds) * time.Second
	} else if date, err := http.ParseTime(value); err == nil {
		wait = time.Until(date)
		if wait > rateLimitMaxSeconds*time.Second {
			wait = rateLimitMaxSeconds * time.Second
		}
	}

	if wait < rateLimitMinWait {
		wait = rateLimitMinWait
	}

	return wait
}
