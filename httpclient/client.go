package httpclient

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/rs/zerolog"
)

// Client is an HTTP client wrapper with logging and retry support
type Client struct {
	httpClient *http.Client
	logger     zerolog.Logger
	baseURL    string
	headers    map[string]string
}

// Config holds HTTP client configuration
type Config struct {
	BaseURL    string
	Timeout    time.Duration
	Logger     zerolog.Logger
	Headers    map[string]string
	MaxRetries int
}

// New creates a new HTTP client
func New(cfg Config) *Client {
	timeout := cfg.Timeout
	if timeout == 0 {
		timeout = 30 * time.Second
	}

	return &Client{
		httpClient: &http.Client{
			Timeout: timeout,
		},
		logger:  cfg.Logger.With().Str("component", "http-client").Logger(),
		baseURL: cfg.BaseURL,
		headers: cfg.Headers,
	}
}

// Response represents an HTTP response
type Response struct {
	StatusCode int
	Body       []byte
	Headers    http.Header
}

// Get performs a GET request
func (c *Client) Get(ctx context.Context, path string, headers map[string]string) (*Response, error) {
	return c.do(ctx, http.MethodGet, path, nil, headers)
}

// Post performs a POST request with JSON body
func (c *Client) Post(ctx context.Context, path string, body interface{}, headers map[string]string) (*Response, error) {
	return c.do(ctx, http.MethodPost, path, body, headers)
}

// Put performs a PUT request with JSON body
func (c *Client) Put(ctx context.Context, path string, body interface{}, headers map[string]string) (*Response, error) {
	return c.do(ctx, http.MethodPut, path, body, headers)
}

// Patch performs a PATCH request with JSON body
func (c *Client) Patch(ctx context.Context, path string, body interface{}, headers map[string]string) (*Response, error) {
	return c.do(ctx, http.MethodPatch, path, body, headers)
}

// Delete performs a DELETE request
func (c *Client) Delete(ctx context.Context, path string, headers map[string]string) (*Response, error) {
	return c.do(ctx, http.MethodDelete, path, nil, headers)
}

func (c *Client) do(ctx context.Context, method, path string, body interface{}, headers map[string]string) (*Response, error) {
	url := c.baseURL + path

	var reqBody io.Reader
	if body != nil {
		jsonBody, err := json.Marshal(body)
		if err != nil {
			return nil, fmt.Errorf("failed to marshal request body: %w", err)
		}
		reqBody = bytes.NewReader(jsonBody)
	}

	req, err := http.NewRequestWithContext(ctx, method, url, reqBody)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	// Set default headers
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")

	// Set client-level headers
	for k, v := range c.headers {
		req.Header.Set(k, v)
	}

	// Set request-level headers
	for k, v := range headers {
		req.Header.Set(k, v)
	}

	// Log request
	startTime := time.Now()
	c.logger.Debug().
		Str("method", method).
		Str("url", url).
		Msg("HTTP request started")

	// Execute request
	resp, err := c.httpClient.Do(req)
	if err != nil {
		c.logger.Error().
			Err(err).
			Str("method", method).
			Str("url", url).
			Dur("duration", time.Since(startTime)).
			Msg("HTTP request failed")
		return nil, fmt.Errorf("request failed: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	// Read response body
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %w", err)
	}

	// Log response
	c.logger.Debug().
		Str("method", method).
		Str("url", url).
		Int("status", resp.StatusCode).
		Dur("duration", time.Since(startTime)).
		Msg("HTTP request completed")

	return &Response{
		StatusCode: resp.StatusCode,
		Body:       respBody,
		Headers:    resp.Header,
	}, nil
}

// GetJSON performs a GET request and unmarshals the response
func (c *Client) GetJSON(ctx context.Context, path string, headers map[string]string, dest interface{}) error {
	resp, err := c.Get(ctx, path, headers)
	if err != nil {
		return err
	}

	if resp.StatusCode >= 400 {
		return fmt.Errorf("HTTP error %d: %s", resp.StatusCode, string(resp.Body))
	}

	if err := json.Unmarshal(resp.Body, dest); err != nil {
		return fmt.Errorf("failed to unmarshal response: %w", err)
	}

	return nil
}

// PostJSON performs a POST request and unmarshals the response
func (c *Client) PostJSON(ctx context.Context, path string, body interface{}, headers map[string]string, dest interface{}) error {
	resp, err := c.Post(ctx, path, body, headers)
	if err != nil {
		return err
	}

	if resp.StatusCode >= 400 {
		return fmt.Errorf("HTTP error %d: %s", resp.StatusCode, string(resp.Body))
	}

	if dest != nil {
		if err := json.Unmarshal(resp.Body, dest); err != nil {
			return fmt.Errorf("failed to unmarshal response: %w", err)
		}
	}

	return nil
}

// SetHeader sets a default header for all requests
func (c *Client) SetHeader(key, value string) {
	if c.headers == nil {
		c.headers = make(map[string]string)
	}
	c.headers[key] = value
}

// SetBaseURL sets the base URL for all requests
func (c *Client) SetBaseURL(baseURL string) {
	c.baseURL = baseURL
}

// IsSuccess checks if the response indicates success
func (r *Response) IsSuccess() bool {
	return r.StatusCode >= 200 && r.StatusCode < 300
}

// Unmarshal unmarshals the response body into dest
func (r *Response) Unmarshal(dest interface{}) error {
	return json.Unmarshal(r.Body, dest)
}


