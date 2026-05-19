package vidpickr

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"
)

// DefaultBaseURL is the production API host. Override via WithBaseURL
// for self-hosted deployments or local dev.
const DefaultBaseURL = "https://api.vidpickr.com/v1"

const apiKeyHeader = "X-API-Key"

// Client is the low-level HTTP client. Most callers use the higher-level
// VidPickr type, which embeds a Client. Reach for Client directly when
// you need fine-grained control (custom retry policies, piping bytes
// into your own pipeline, etc.).
type Client struct {
	apiKey  string
	baseURL string
	httpc   *http.Client
}

// NewClient constructs a low-level HTTP client.
func NewClient(apiKey string, opts ...Option) *Client {
	if apiKey == "" {
		panic("vidpickr: apiKey is required")
	}
	cfg := &config{
		baseURL: DefaultBaseURL,
		httpc:   &http.Client{Timeout: 90 * time.Second},
	}
	for _, o := range opts {
		o(cfg)
	}
	return &Client{
		apiKey:  apiKey,
		baseURL: strings.TrimRight(cfg.baseURL, "/"),
		httpc:   cfg.httpc,
	}
}

// Info resolves a YouTube URL into the full format list.
func (c *Client) Info(ctx context.Context, target string) (*VideoInfo, error) {
	u := c.baseURL + "/info?url=" + url.QueryEscape(target)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set(apiKeyHeader, c.apiKey)
	res, err := c.httpc.Do(req)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()
	if !ok(res.StatusCode) {
		return nil, toAPIError(res)
	}
	var info VideoInfo
	if err := json.NewDecoder(res.Body).Decode(&info); err != nil {
		return nil, err
	}
	return &info, nil
}

// SplitToken exchanges a merge token (bundled video+audio) for two
// single-source tokens you can stream separately.
func (c *Client) SplitToken(ctx context.Context, mergeToken string) (*SplitTokenResult, error) {
	u := c.baseURL + "/split_token?token=" + url.QueryEscape(mergeToken)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set(apiKeyHeader, c.apiKey)
	res, err := c.httpc.Do(req)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()
	if !ok(res.StatusCode) {
		return nil, toAPIError(res)
	}
	var split SplitTokenResult
	if err := json.NewDecoder(res.Body).Decode(&split); err != nil {
		return nil, err
	}
	return &split, nil
}

// OpenStream returns a response whose body streams the requested track.
// Caller MUST close res.Body. Use this when piping bytes directly into
// another consumer; for "stream to file" use StreamToFile.
func (c *Client) OpenStream(ctx context.Context, token string) (*http.Response, error) {
	u := c.baseURL + "/stream?token=" + url.QueryEscape(token)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set(apiKeyHeader, c.apiKey)
	res, err := c.httpc.Do(req)
	if err != nil {
		return nil, err
	}
	if !ok(res.StatusCode) {
		defer res.Body.Close()
		return nil, toAPIError(res)
	}
	return res, nil
}

// StreamToFile downloads a single track to disk. Returns total bytes
// written and any error.
func (c *Client) StreamToFile(ctx context.Context, token, path string) (int64, error) {
	res, err := c.OpenStream(ctx, token)
	if err != nil {
		return 0, err
	}
	defer res.Body.Close()

	f, err := createFile(path)
	if err != nil {
		return 0, err
	}
	defer f.Close()
	n, err := io.Copy(f, res.Body)
	return n, err
}

// Subtitle fetches a caption track in the requested format (srt|vtt|txt).
// Returns the track as a string.
func (c *Client) Subtitle(ctx context.Context, token, format string) (string, error) {
	if format == "" {
		format = "srt"
	}
	u := c.baseURL + "/subtitle?token=" + url.QueryEscape(token) + "&format=" + url.QueryEscape(format)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set(apiKeyHeader, c.apiKey)
	res, err := c.httpc.Do(req)
	if err != nil {
		return "", err
	}
	defer res.Body.Close()
	if !ok(res.StatusCode) {
		return "", toAPIError(res)
	}
	body, err := io.ReadAll(res.Body)
	if err != nil {
		return "", err
	}
	return string(body), nil
}

// ─────────────────────────────────────────────────────────────────────

type config struct {
	baseURL string
	httpc   *http.Client
}

// Option tweaks the Client at construction.
type Option func(*config)

// WithBaseURL overrides the default API host. Useful for staging,
// self-hosted deployments, or pointing at a local Go server in dev.
func WithBaseURL(u string) Option { return func(c *config) { c.baseURL = u } }

// WithHTTPClient lets the caller supply a custom *http.Client (custom
// transport, retries, instrumentation, etc.).
func WithHTTPClient(h *http.Client) Option { return func(c *config) { c.httpc = h } }

// ─────────────────────────────────────────────────────────────────────

func ok(status int) bool { return status >= 200 && status < 300 }

func toAPIError(res *http.Response) error {
	apiErr := &APIError{
		Code:    fmt.Sprintf("http_%d", res.StatusCode),
		Message: fmt.Sprintf("HTTP %d %s", res.StatusCode, res.Status),
		Status:  res.StatusCode,
	}
	var body struct {
		Error struct {
			Code    string `json:"code"`
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.NewDecoder(res.Body).Decode(&body); err == nil {
		if body.Error.Code != "" {
			apiErr.Code = body.Error.Code
		}
		if body.Error.Message != "" {
			apiErr.Message = body.Error.Message
		}
	}
	if retry := res.Header.Get("Retry-After"); retry != "" {
		if n, err := strconv.Atoi(retry); err == nil {
			apiErr.RetryAfter = n
		}
	}
	return apiErr
}

// createFile is the indirection layer the tests swap to capture writes
// without hitting disk. Production points it at os.Create.
var createFile = func(path string) (io.WriteCloser, error) {
	f, err := os.Create(path)
	if err != nil {
		return nil, fmt.Errorf("create %s: %w", path, err)
	}
	return f, nil
}
