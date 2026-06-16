// Package api is the single patch point for the (unofficial) Speediance cloud
// API. Every request URL, query param, JSON body, and header is frozen to match
// the Android app byte-for-byte (GOAL.md §2, §8); the brittle server rejects
// anything else. If the app updates and something breaks, this package is where
// to fix it.
package api

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/hashicorp/go-retryablehttp"

	"github.com/stozo04/speediance-cli/internal/auth"
)

// maxResponseBytes caps how much of a response body we read, guarding against a
// hostile or broken server streaming forever (GOAL.md §8 decode discipline).
const maxResponseBytes = 16 << 20 // 16 MiB

// defaultTimeout matches the Python client's ~20–30s per-request budget.
const defaultTimeout = 30 * time.Second

// Client talks to the Speediance API for one account/region.
type Client struct {
	base     string // e.g. https://api2.speediance.com/api
	host     string // Host header, derived from base
	email    string
	password string

	token  string
	userID string

	// retry handles transient failures on idempotent GETs only; plain is the
	// underlying single-shot client used for POSTs so logins and program
	// creation are never auto-retried at the transport layer (GOAL.md §8).
	retry *retryablehttp.Client
	plain *http.Client

	logger *slog.Logger
	now    func() time.Time // injectable clock for the Timestamp header (tests).
}

// Config constructs a Client. Token/UserID may be empty (a fresh login happens
// lazily on first use). BaseURL overrides region resolution for tests.
type Config struct {
	Region   string
	Email    string
	Password string
	Token    auth.Token
	Logger   *slog.Logger
	Timeout  time.Duration
	BaseURL  string           // test override; empty → derive from Region
	Now      func() time.Time // test override; nil → time.Now
}

// New builds a Client with a custom *http.Client (never http.DefaultClient) and
// a retry layer restricted to safe methods.
func New(cfg Config) *Client {
	base := cfg.BaseURL
	if base == "" {
		base = baseURLFor(cfg.Region)
	}
	base = strings.TrimRight(base, "/")

	timeout := cfg.Timeout
	if timeout == 0 {
		timeout = defaultTimeout
	}
	logger := cfg.Logger
	if logger == nil {
		logger = slog.New(slog.NewTextHandler(io.Discard, nil))
	}
	nowFn := cfg.Now
	if nowFn == nil {
		nowFn = time.Now
	}

	retry := retryablehttp.NewClient()
	retry.RetryMax = 3
	retry.HTTPClient.Timeout = timeout
	// Route retry-layer logs to slog on stderr so retries never touch stdout
	// (GOAL.md §8). DefaultBackoff already honors Retry-After on 429/503.
	retry.Logger = leveledLogger{logger}

	return &Client{
		base:     base,
		host:     hostOf(base),
		email:    cfg.Email,
		password: cfg.Password,
		token:    cfg.Token.Token,
		userID:   cfg.Token.UserID,
		retry:    retry,
		plain:    retry.HTTPClient, // share the transport/timeout; no retry wrapper.
		logger:   logger,
		now:      nowFn,
	}
}

// Token returns the current session token and user id, so the CLI can write the
// (possibly refreshed) values back to the cache after a command (GOAL.md §7).
func (c *Client) Token() auth.Token {
	return auth.Token{Token: c.token, UserID: c.userID}
}

func hostOf(base string) string {
	if u, err := url.Parse(base); err == nil && u.Host != "" {
		return u.Host
	}
	return ""
}

// setHeaders applies the frozen header set. Keys are assigned directly into the
// map (not via Header.Set) so they reach the wire byte-for-byte, exactly as the
// Android app / Python client send them (GOAL.md §8). Token and App_user_id are
// included only when present.
func (c *Client) setHeaders(req *http.Request) {
	ts := strconv.FormatInt(c.now().UnixMilli(), 10)
	req.Host = c.host // Host header comes from req.Host, not req.Header.
	req.Header = http.Header{
		"User-Agent":      {"Dart/3.9 (dart:io)"},
		"Content-Type":    {"application/json"},
		"Timestamp":       {ts},
		"Utc_offset":      {"+0000"},
		"Timezone":        {"GMT"},
		"Versioncode":     {"40304"},
		"Accept-Language": {"en"},
		"App_type":        {"SOFTWARE"},
		"Mobiledevices":   {mobileDevices},
	}
	if c.token != "" {
		req.Header["Token"] = []string{c.token}
	}
	if c.userID != "" {
		req.Header["App_user_id"] = []string{c.userID}
	}
}

// Login performs the two-step email/password login (GOAL.md §8) and stores the
// resulting token/user id on the client.
func (c *Client) Login(ctx context.Context) error {
	// Step 1: verifyIdentity.
	env, err := c.send(ctx, http.MethodPost, "/app/v2/login/verifyIdentity",
		verifyIdentityReq{Type: 2, UserIdentity: c.email})
	if err != nil {
		return err
	}
	if env.Code != 0 {
		return authErrorf("verifyIdentity failed: %s", env.Message)
	}
	var v verifyIdentityData
	_ = json.Unmarshal(env.Data, &v)
	if !v.IsExist {
		return &AuthError{Msg: "Account does not exist. Register in the Speediance app first."}
	}
	if !v.HasPwd {
		return &AuthError{Msg: "Account has no password set. Set one in the Speediance app."}
	}

	// Step 2: byPass (password login).
	env, err = c.send(ctx, http.MethodPost, "/app/v2/login/byPass",
		byPassReq{UserIdentity: c.email, Password: c.password, Type: 2})
	if err != nil {
		return err
	}
	if env.Code != 0 {
		return authErrorf("Login failed: %s", env.Message)
	}
	var d byPassData
	if err := json.Unmarshal(env.Data, &d); err != nil {
		return authErrorf("Login failed: malformed response: %v", err)
	}
	c.token = d.Token
	c.userID = d.AppUserID.String()
	c.logger.Info("logged in", "user_id", c.userID)
	return nil
}

// GetJSON issues a GET and returns the decoded envelope, logging in lazily and
// retrying once on an expired-token (code 91) response (GOAL.md §8). path
// includes any query string, already encoded to match the Python client.
func (c *Client) GetJSON(ctx context.Context, path string) (Envelope, error) {
	return c.request(ctx, http.MethodGet, path, nil)
}

// PostJSON issues a POST with a JSON body, with the same lazy-login and code-91
// behavior as GetJSON. POSTs are never retried at the transport layer.
func (c *Client) PostJSON(ctx context.Context, path string, body any) (Envelope, error) {
	return c.request(ctx, http.MethodPost, path, body)
}

// request implements lazy login + the single application-level code-91 retry,
// shared by GET and POST (GOAL.md §8).
func (c *Client) request(ctx context.Context, method, path string, body any) (Envelope, error) {
	if c.token == "" {
		if err := c.Login(ctx); err != nil {
			return Envelope{}, err
		}
	}
	env, err := c.send(ctx, method, path, body)
	if err != nil {
		return Envelope{}, err
	}
	if env.Code == codeTokenExpired {
		c.logger.Info("token expired, re-authenticating")
		if err := c.Login(ctx); err != nil {
			return Envelope{}, err
		}
		env, err = c.send(ctx, method, path, body)
		if err != nil {
			return Envelope{}, err
		}
	}
	return env, nil
}

// send performs a single HTTP round-trip and decodes the envelope. GETs go
// through the retry client; everything else uses the plain client.
func (c *Client) send(ctx context.Context, method, path string, body any) (Envelope, error) {
	var bodyBytes []byte
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return Envelope{}, fmt.Errorf("encode request body: %w", err)
		}
		bodyBytes = b
	}

	urlStr := c.base + path

	var resp *http.Response
	var err error
	if method == http.MethodGet || method == http.MethodHead {
		var req *retryablehttp.Request
		req, err = retryablehttp.NewRequestWithContext(ctx, method, urlStr, bodyBytes)
		if err != nil {
			return Envelope{}, fmt.Errorf("build request: %w", err)
		}
		c.setHeaders(req.Request)
		resp, err = c.retry.Do(req)
	} else {
		var req *http.Request
		var reader io.Reader
		if bodyBytes != nil {
			reader = bytes.NewReader(bodyBytes)
		}
		req, err = http.NewRequestWithContext(ctx, method, urlStr, reader)
		if err != nil {
			return Envelope{}, fmt.Errorf("build request: %w", err)
		}
		c.setHeaders(req)
		resp, err = c.plain.Do(req)
	}
	if err != nil {
		return Envelope{}, fmt.Errorf("%s %s: %w", method, path, err)
	}
	defer func() { _ = resp.Body.Close() }()

	data, err := io.ReadAll(io.LimitReader(resp.Body, maxResponseBytes))
	if err != nil {
		return Envelope{}, fmt.Errorf("read response: %w", err)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return Envelope{}, fmt.Errorf("%s %s: HTTP %d", method, path, resp.StatusCode)
	}

	var env Envelope
	if err := json.Unmarshal(data, &env); err != nil {
		return Envelope{}, fmt.Errorf("decode response from %s: %w", path, err)
	}
	return env, nil
}

// leveledLogger adapts *slog.Logger to retryablehttp.LeveledLogger so retry
// diagnostics flow to stderr at debug/warn level instead of stdout.
type leveledLogger struct{ l *slog.Logger }

func (a leveledLogger) Error(msg string, kv ...any) { a.l.Error(msg, kv...) }
func (a leveledLogger) Warn(msg string, kv ...any)  { a.l.Warn(msg, kv...) }
func (a leveledLogger) Info(msg string, kv ...any)  { a.l.Debug(msg, kv...) }
func (a leveledLogger) Debug(msg string, kv ...any) { a.l.Debug(msg, kv...) }
