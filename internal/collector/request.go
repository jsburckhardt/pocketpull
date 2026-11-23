package collector

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"strconv"
	"syscall"
	"time"
)

const (
	requestTimeout = 30 * time.Second
	maxAttempts    = 4
	maxRetryAfter  = 10 * time.Second
)

func request(ctx context.Context, options Options, route, id string) ([]byte, error) {
	started := time.Now()
	for attempt := 1; ; attempt++ {
		if ctx.Err() != nil {
			return nil, errInterrupted
		}
		body, retry, retryAfter, err := requestAttempt(ctx, options, route)
		if ctx.Err() != nil {
			return nil, errInterrupted
		}
		if err == nil || !retry || attempt == maxAttempts {
			return body, err
		}
		delay := 500 * time.Millisecond * time.Duration(1<<(attempt-1))
		if retryAfter > delay {
			delay = retryAfter
		}
		attrs := []any{"event", "request_retry", "attempt", attempt, "max_attempts", maxAttempts,
			"delay_ms", delay.Milliseconds(), "elapsed_ms", time.Since(started).Milliseconds(), "reason", err.Error()}
		if id != "" {
			attrs = append(attrs, "recording_id", id, "operation", "detail")
		} else {
			attrs = append(attrs, "operation", "list")
		}
		options.Log.Warn("retrying request", attrs...)
		if err := options.Sleep(ctx, delay); err != nil {
			return nil, errInterrupted
		}
		if ctx.Err() != nil {
			return nil, errInterrupted
		}
		(*options.retries)++
	}
}

func requestAttempt(ctx context.Context, options Options, route string) ([]byte, bool, time.Duration, error) {
	ctx, cancel := context.WithTimeout(ctx, requestTimeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, options.BaseURL+route, nil)
	if err != nil {
		return nil, false, 0, errors.New("request setup failed")
	}
	req.Header.Set("Authorization", "Bearer "+options.APIKey)
	req.Header.Set("Accept", "application/json")
	response, err := options.Client.Do(req)
	if err != nil {
		return nil, transientNetwork(err), 0, errors.New("transport failed")
	}
	defer response.Body.Close()
	if response.StatusCode == http.StatusUnauthorized || response.StatusCode == http.StatusForbidden {
		return nil, false, 0, errAuth
	}
	if response.StatusCode != http.StatusOK {
		retry := response.StatusCode == http.StatusTooManyRequests ||
			(response.StatusCode >= 500 && response.StatusCode <= 599)
		return nil, retry, retryDelay(response.Header.Get("Retry-After"), options.Now()),
			fmt.Errorf("http status %d", response.StatusCode)
	}
	const maxResponseBytes = 32 << 20
	body, err := io.ReadAll(io.LimitReader(response.Body, maxResponseBytes+1))
	if err != nil {
		return nil, transientNetwork(err), 0, errors.New("response read failed")
	}
	if len(body) > maxResponseBytes {
		return nil, false, 0, errors.New("response too large")
	}
	return body, false, 0, nil
}

func transientNetwork(err error) bool {
	var network net.Error
	if errors.As(err, &network) && (network.Timeout() || network.Temporary()) {
		return true
	}
	for _, transient := range []error{io.EOF, io.ErrUnexpectedEOF, syscall.ECONNRESET,
		syscall.ECONNREFUSED, syscall.EPIPE, syscall.ENETUNREACH, syscall.EHOSTUNREACH} {
		if errors.Is(err, transient) {
			return true
		}
	}
	return false
}

func retryDelay(value string, now time.Time) time.Duration {
	if seconds, err := strconv.ParseUint(value, 10, 64); err == nil {
		if seconds >= uint64(maxRetryAfter/time.Second) {
			return maxRetryAfter
		}
		return time.Duration(seconds) * time.Second
	}
	if date, err := http.ParseTime(value); err == nil {
		return min(max(date.Sub(now), 0), maxRetryAfter)
	}
	return 0
}

func sleep(ctx context.Context, delay time.Duration) error {
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}
