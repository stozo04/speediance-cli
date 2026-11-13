// Package auth reads and writes the on-disk session token cache.
package auth

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

// Token is the cached session token and user id.
type Token struct {
	Token  string `json:"token"`
	UserID string `json:"user_id"`
}

// Empty reports whether the token holds no usable session.
func (t Token) Empty() bool { return t.Token == "" }

// Load reads the token cache; a missing or corrupt file yields an empty Token.
func Load(path string) (Token, bool, error) {
	var tok Token
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return tok, false, nil
		}
		return tok, false, fmt.Errorf("read token cache %s: %w", path, err)
	}
	if err := json.Unmarshal(data, &tok); err != nil {
		return Token{}, false, nil
	}
	return tok, !tok.Empty(), nil
}

// Save writes the token cache with owner-only permissions.
func Save(path string, tok Token) error {
	if dir := filepath.Dir(path); dir != "" && dir != "." {
		if err := os.MkdirAll(dir, 0o700); err != nil {
			return fmt.Errorf("create token cache dir %s: %w", dir, err)
		}
	}

	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o600)
	if err != nil {
		return fmt.Errorf("open token cache %s: %w", path, err)
	}
	defer func() { _ = f.Close() }()

	_ = f.Chmod(0o600)

	enc := json.NewEncoder(f)
	enc.SetIndent("", "  ")
	if err := enc.Encode(tok); err != nil {
		return fmt.Errorf("write token cache %s: %w", path, err)
	}
	return nil
}
