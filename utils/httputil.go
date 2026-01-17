package utils

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"slices"
	"sync"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/metric"
	"go.opentelemetry.io/otel/trace"
)

var (
	// Metrics instruments (initialized via InitCacheMetrics after MeterProvider is set)
	cacheOperationsCounter metric.Int64Counter
)

// InitCacheMetrics initializes metrics instruments. Must be called after MeterProvider is set.
func InitCacheMetrics() {
	meter := otel.Meter("cloak-apps")
	var err error

	cacheOperationsCounter, err = meter.Int64Counter(
		"cache.operations.total",
		metric.WithDescription("Total number of cache operations"),
		metric.WithUnit("{operation}"),
	)
	if err != nil {
		slog.Error("Failed to create cache operations counter", "error", err)
	}
}

// BeforeCallHook is a hook (function) signature that should be called before applying the pattern
type BeforeCallHook func(ctx context.Context, req *http.Request) error

// Circuit is a function signature we want to apply the pattern on
type Circuit func(context.Context) ([]byte, error)

// CircuitWithKey is a function signature (with unique keys) we want to apply the pattern on
type CircuitWithKey func(context.Context, string) ([]byte, error)

// deepCopyRequest returns deep copy (body included) of the request and re-sets the original request
func deepCopyRequest(ctx context.Context, reqInit *http.Request) (*http.Request, error) {
	if reqInit.Body == nil {
		return reqInit.Clone(ctx), nil
	}

	body, err := io.ReadAll(reqInit.Body)
	if err != nil {
		return nil, fmt.Errorf("error while copying request body: %w", err)
	}

	reqClone := reqInit.Clone(ctx)
	reqInit.Body = io.NopCloser(bytes.NewReader(body))
	reqClone.Body = io.NopCloser(bytes.NewReader(body))

	return reqClone, nil
}

// SendRetryableRequest is a retryable call pattern, that is applied on specific retriableStatuses up to maxRetryTimes
// using specified http.Client (or a default one) with a prior call to BeforeCallHook (if provided)
//
// Note: initial http.Request can be later reused
func SendRetryableRequest(
	ctx context.Context,
	req *http.Request,
	retriableStatuses []int,
	maxRetryTimes int,
	hook BeforeCallHook,
	client *http.Client,
) ([]byte, error) {
	span := trace.SpanFromContext(ctx)

	if len(retriableStatuses) == 0 {
		return nil, fmt.Errorf("you must provide at least one retriable status code")
	}
	if slices.Contains(retriableStatuses, http.StatusOK) {
		return nil, fmt.Errorf("response status 200 cannot be used to trigger the retry")
	}

	var resBody []byte
	var retryNum = 0
	var statusCode = retriableStatuses[0]
	if client == nil {
		client = http.DefaultClient
	}
	for next := true; next; next = retryNum <= maxRetryTimes && slices.Contains(retriableStatuses, statusCode) {
		if retryNum > 0 {
			span.AddEvent("http.retry", trace.WithAttributes(
				attribute.Int("retry.attempt", retryNum),
				attribute.Int("http.response.status_code", statusCode),
			))
		}

		// Operate on a request (deep) copy
		reqCopy, err := deepCopyRequest(ctx, req)
		if err != nil {
			span.RecordError(err)
			span.SetStatus(codes.Error, "failed to copy request")
			return nil, err
		}

		if hook != nil {
			if err := hook(ctx, reqCopy); err != nil {
				span.RecordError(err)
				span.SetStatus(codes.Error, "hook failed")
				return nil, err
			}
		}

		resp, err := client.Do(reqCopy)
		if err != nil {
			span.RecordError(err)
			span.SetStatus(codes.Error, "request failed")
			return nil, fmt.Errorf("failed to execute request: %w", err)
		}

		statusCode = resp.StatusCode
		resBody, err = io.ReadAll(resp.Body)
		resp.Body.Close()
		if err != nil {
			span.RecordError(err)
			span.SetStatus(codes.Error, "failed to read response")
			return nil, fmt.Errorf("failed to read response body: %w", err)
		}

		if statusCode == http.StatusOK {
			span.SetAttributes(attribute.Int("http.retry_count", retryNum))
			return resBody, nil
		}

		retryNum++
	}

	err := fmt.Errorf("request failed after %d attempts with status %d: %s", retryNum, statusCode, string(resBody))
	span.RecordError(err)
	span.SetStatus(codes.Error, "max retries exceeded")
	span.SetAttributes(attribute.Int("http.retry_count", retryNum))
	return nil, err
}

// CacheFirstTTL tracks last time it was called and returns a cached result until ttl expires
func CacheFirstTTL(circuit Circuit, ttl time.Duration) Circuit {
	var expires time.Time
	var result []byte
	var err error
	var m sync.Mutex

	return func(ctx context.Context) ([]byte, error) {
		span := trace.SpanFromContext(ctx)

		m.Lock()
		defer m.Unlock()

		if time.Now().After(expires) {
			span.SetAttributes(attribute.String("cache.status", "miss"))
			cacheOperationsCounter.Add(ctx, 1, metric.WithAttributes(
				attribute.String("cache.status", "miss"),
				attribute.String("cache.type", "ttl"),
			))
			result, err = circuit(ctx)
			if err == nil {
				// Move expiration only if no error
				expires = time.Now().Add(ttl)
			}
			return result, err
		}

		span.SetAttributes(attribute.String("cache.status", "hit"))
		cacheOperationsCounter.Add(ctx, 1, metric.WithAttributes(
			attribute.String("cache.status", "hit"),
			attribute.String("cache.type", "ttl"),
		))
		return result, err
	}
}

type resultWithTTL struct {
	expires time.Time
	result  []byte
	err     error
}

// hashKey creates a SHA-256 hash of the key for use as cache key (fixed-size)
func hashKey(token string) string {
	hash := sha256.Sum256([]byte(token))
	return hex.EncodeToString(hash[:])
}

// CacheFirstForKeyTTL tracks last call for each key and returns a cached result until ttl expires
func CacheFirstForKeyTTL(done chan struct{}, circuit CircuitWithKey, ttl time.Duration, hashKeys bool) CircuitWithKey {
	results := map[string]*resultWithTTL{}
	cleanupTicker := time.NewTicker(2 * ttl)
	var m sync.Mutex

	go func() {
		for {
			select {
			case <-done:
				cleanupTicker.Stop()
				return
			case <-cleanupTicker.C:
				now := time.Now()
				m.Lock()
				for key, res := range results {
					if now.After(res.expires) {
						delete(results, key)
					}
				}
				m.Unlock()
			}
		}
	}()

	return func(ctx context.Context, key string) ([]byte, error) {
		span := trace.SpanFromContext(ctx)
		_key := key
		if hashKeys {
			_key = hashKey(key)
		}

		m.Lock()
		defer m.Unlock()
		cache := results[_key]

		if cache == nil || time.Now().After(cache.expires) {
			span.SetAttributes(attribute.String("cache.status", "miss"))
			cacheOperationsCounter.Add(ctx, 1, metric.WithAttributes(
				attribute.String("cache.status", "miss"),
				attribute.String("cache.type", "key_ttl"),
			))
			var expires time.Time

			res, err := circuit(ctx, key)
			if err == nil {
				// Set expiration only if no error
				expires = time.Now().Add(ttl)
			}

			results[_key] = &resultWithTTL{
				expires: expires,
				result:  res,
				err:     err,
			}
			return res, err
		}

		span.SetAttributes(attribute.String("cache.status", "hit"))
		cacheOperationsCounter.Add(ctx, 1, metric.WithAttributes(
			attribute.String("cache.status", "hit"),
			attribute.String("cache.type", "key_ttl"),
		))
		return cache.result, cache.err
	}
}
