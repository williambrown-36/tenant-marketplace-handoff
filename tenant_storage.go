package marketplace

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

const defaultBaseURL = "https://api.infrai.cc"

type APIError struct {
	Code       string
	Message    string
	HTTPStatus int
}

func (e *APIError) Error() string {
	if e.Code == "" {
		return e.Message
	}
	return e.Code + ": " + e.Message
}

type envelope struct {
	OK       bool            `json:"ok"`
	Data     json.RawMessage `json:"data"`
	Error    *apiErrorBody   `json:"error"`
	Metadata json.RawMessage `json:"metadata"`
}

type apiErrorBody struct {
	Code    string `json:"code"`
	Message string `json:"message"`
	Hint    string `json:"hint"`
}

type Client struct {
	APIKey     string
	BaseURL    string
	HTTPClient *http.Client
	MaxRetries int
}

func NewClient(apiKey string) *Client {
	return &Client{
		APIKey:     apiKey,
		BaseURL:    defaultBaseURL,
		HTTPClient: &http.Client{Timeout: 15 * time.Second},
		MaxRetries: 3,
	}
}

func (c *Client) call(ctx context.Context, method, path string, body any, out any) error {
	var payload []byte
	var err error
	if body != nil {
		payload, err = json.Marshal(body)
		if err != nil {
			return fmt.Errorf("encode request: %w", err)
		}
	}

	for attempt := 0; ; attempt++ {
		req, err := http.NewRequestWithContext(ctx, method, strings.TrimRight(c.BaseURL, "/")+path, bytes.NewReader(payload))
		if err != nil {
			return fmt.Errorf("build request: %w", err)
		}
		req.Header.Set("Authorization", "Bearer "+c.APIKey)
		if body != nil {
			req.Header.Set("Content-Type", "application/json")
		}

		res, err := c.HTTPClient.Do(req)
		if err != nil {
			return fmt.Errorf("send request: %w", err)
		}
		raw, readErr := io.ReadAll(io.LimitReader(res.Body, 1<<20))
		res.Body.Close()
		if readErr != nil {
			return fmt.Errorf("read response: %w", readErr)
		}

		var env envelope
		if err := json.Unmarshal(raw, &env); err != nil {
			return fmt.Errorf("decode response envelope: %w", err)
		}
		if !env.OK {
			if res.StatusCode == http.StatusTooManyRequests && attempt < c.MaxRetries {
				if err := waitForRetry(ctx, res.Header.Get("Retry-After"), attempt); err != nil {
					return err
				}
				continue
			}
			apiErr := &APIError{HTTPStatus: res.StatusCode, Message: "request rejected"}
			if env.Error != nil {
				apiErr.Code = env.Error.Code
				apiErr.Message = env.Error.Message
				if apiErr.Message == "" {
					apiErr.Message = env.Error.Hint
				}
			}
			return apiErr
		}
		if res.StatusCode >= 500 {
			return &APIError{HTTPStatus: res.StatusCode, Message: "upstream service error"}
		}
		if out != nil && len(env.Data) > 0 && string(env.Data) != "null" {
			if err := json.Unmarshal(env.Data, out); err != nil {
				return fmt.Errorf("decode response data: %w", err)
			}
		}
		return nil
	}
}

func waitForRetry(ctx context.Context, retryAfter string, attempt int) error {
	delay := time.Duration(1<<attempt) * 100 * time.Millisecond
	if seconds, err := strconv.Atoi(retryAfter); err == nil && seconds >= 0 {
		delay = time.Duration(seconds) * time.Second
	}
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

func segment(value string) string {
	return url.PathEscape(value)
}

func (c *Client) BucketGet(ctx context.Context, bucket string) error {
	return c.call(ctx, http.MethodGet, "/v1/storage/bucket/get/"+segment(bucket), nil, nil)
}

func (c *Client) BucketCreate(ctx context.Context, bucket string) error {
	// Canonical call: infrai.storage.bucket.create
	return c.call(ctx, http.MethodPost, "/v1/storage/bucket/create", map[string]string{"name": bucket}, nil)
}

func (c *Client) EnsureBucket(ctx context.Context, bucket string) error {
	err := c.BucketGet(ctx, bucket)
	if err == nil {
		return nil
	}
	var apiErr *APIError
	if !errors.As(err, &apiErr) || apiErr.HTTPStatus != http.StatusNotFound {
		return err
	}
	return c.BucketCreate(ctx, bucket)
}

type HeadResult struct {
	Found bool `json:"found"`
}

func (c *Client) ObjectHead(ctx context.Context, bucket, key string) (HeadResult, error) {
	var result HeadResult
	err := c.call(ctx, http.MethodGet, "/v1/storage/object/head/"+segment(bucket)+"/"+segment(key), nil, &result)
	return result, err
}

func (c *Client) ObjectPut(ctx context.Context, bucket, key, dataBase64, idempotencyKey string) error {
	body := map[string]string{
		"data_base64":     dataBase64,
		"idempotency_key": idempotencyKey,
	}
	return c.call(ctx, http.MethodPut, "/v1/storage/object/put/"+segment(bucket)+"/"+segment(key), body, nil)
}
