package api

import (
	"encoding/json"
	"fmt"
)

// Region base URLs (frozen — GOAL.md §8). An unknown region falls back to
// Global, mirroring the Python client's BASE_URLS.get(region, Global).
var baseURLs = map[string]string{
	"Global": "https://api2.speediance.com/api",
	"EU":     "https://euapi.speediance.com/api",
}

// baseURLFor resolves a region to its API base, defaulting to Global.
func baseURLFor(region string) string {
	if u, ok := baseURLs[region]; ok {
		return u
	}
	return baseURLs["Global"]
}

// mobileDevices is the spoofed device fingerprint header value, byte-for-byte
// from the Android app (GOAL.md §8). Declared once so it can't drift.
const mobileDevices = `{"brand":"google","device":"emulator64","deviceType":"sdk_gphone64","os":"","os_version":"31","manufacturer":"Google"}`

// Envelope is the standard API response wrapper: {code, message, data}. data is
// left raw because its shape depends on the endpoint; callers decode it into the
// appropriate type. Responses are decoded loosely (no DisallowUnknownFields) so
// new server fields don't break us (GOAL.md §8).
type Envelope struct {
	Code    int             `json:"code"`
	Message string          `json:"message"`
	Data    json.RawMessage `json:"data"`
}

// codeTokenExpired is the application-level "token expired" signal. On seeing
// it the client re-logs-in once and retries the same request (GOAL.md §8).
const codeTokenExpired = 91

// AuthError marks an authentication failure. The CLI maps it to exit code 2 to
// preserve the Python tool's behavior (GOAL.md §12). It is a distinct type so
// callers can match it with errors.As.
type AuthError struct {
	Msg string
}

func (e *AuthError) Error() string { return e.Msg }

func authErrorf(format string, args ...any) *AuthError {
	return &AuthError{Msg: fmt.Sprintf(format, args...)}
}

// --- login wire types (frozen bodies — GOAL.md §8) ---

type verifyIdentityReq struct {
	Type         int    `json:"type"`
	UserIdentity string `json:"userIdentity"`
}

type verifyIdentityData struct {
	IsExist bool `json:"isExist"`
	HasPwd  bool `json:"hasPwd"`
}

type byPassReq struct {
	UserIdentity string `json:"userIdentity"`
	Password     string `json:"password"`
	Type         int    `json:"type"`
}

type byPassData struct {
	Token string `json:"token"`
	// appUserId arrives as a JSON number; json.Number preserves it exactly so
	// str(appUserId) parity holds (no float rounding of a big id).
	AppUserID json.Number `json:"appUserId"`
}
