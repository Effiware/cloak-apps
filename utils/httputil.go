package utils

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"slices"
)

type BeforeCallHook func(ctx context.Context, req *http.Request) error

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

func SendRetryableRequest(
	ctx context.Context,
	req *http.Request,
	retriableStatuses []int,
	maxRetryTimes int,
	hook BeforeCallHook,
	client *http.Client,
) ([]byte, error) {
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
		// Operate on a request (deep) copy
		reqCopy, err := deepCopyRequest(ctx, req)
		if err != nil {
			return nil, err
		}

		if hook != nil {
			if err := hook(ctx, reqCopy); err != nil {
				return nil, err
			}
		}

		resp, err := client.Do(reqCopy)
		if err != nil {
			return nil, fmt.Errorf("failed to execute request: %w", err)
		}

		statusCode = resp.StatusCode
		resBody, err = io.ReadAll(resp.Body)
		resp.Body.Close()
		if err != nil {
			return nil, fmt.Errorf("failed to read response body: %w", err)
		}

		if statusCode == http.StatusOK {
			return resBody, nil
		}

		retryNum++
	}

	return nil, fmt.Errorf("request failed after %d attempts with status %d: %s", retryNum, statusCode, string(resBody))
}
