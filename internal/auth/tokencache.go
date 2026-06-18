// Package auth manages the on-disk token cache (.token.json).
//
// The file holds the session token and user id returned by login so a user with
// a valid cached token isn't forced to re-authenticate (GOAL.md §7, §20.4). It
// is written with owner-only permissions because it is a credential.
package auth

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

// Token is the cache file's contents. The JSON shape — {"token","user_id"} — is
// frozen for drop-in compatibility with the Python tool (GOAL.md §7).
type Token struct {
	Token  string `json:"token"`
	UserID string `json:"user_id"`
}

// Empty reports whether the token holds no usable session.
func (t Token) Empty() bool { return t.Token == "" }

// Load reads the token cache. A missing file is not an error — it returns a
// zero Token and false so callers can fall back to a fresh login.
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
		// A corrupt cache shouldn't be fatal; treat it as "no token" so the
		// next call logs in fresh and overwrites it.
		return Token{}, false, nil
	}
	return tok, !tok.Empty(), nil
}

// Save writes the token cache with owner-only (0600) permissions.
//
// It uses os.OpenFile with O_TRUNC rather than os.WriteFile because WriteFile
// will NOT re-restrict permissions on a file that already exists (GOAL.md §7):
// passing the mode to OpenFile applies it to a freshly created file, and we
// additionally Chmod to tighten an existing one.
//
// On Windows, Unix permission bits are largely advisory; a Chmod failure there
// must not fail the command, so chmod errors are ignored (best-effort), again
// matching the Python tool's try/except.
func Save(path string, tok Token) error {
	if dir := filepath.Dir(path); dir != "" && dir != "." {
		// A non-CWD location may need its parent created; 0700 keeps the
		// directory owner-only too.
		if err := os.MkdirAll(dir, 0o700); err != nil {
			return fmt.Errorf("create token cache dir %s: %w", dir, err)
		}
	}

	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o600)
	if err != nil {
		return fmt.Errorf("open token cache %s: %w", path, err)
	}
	defer func() { _ = f.Close() }()

	// Best-effort re-tighten in case the file pre-existed with looser perms.
	// Ignored on platforms (Windows) where chmod is unsupported.
	_ = f.Chmod(0o600)

	enc := json.NewEncoder(f)
	enc.SetIndent("", "  ")
	if err := enc.Encode(tok); err != nil {
		return fmt.Errorf("write token cache %s: %w", path, err)
	}
	return nil
}

// MigrateLegacy relocates a token cache from a legacy working-directory path to
// the resolved per-user location, one time, on upgrade. It exists for issue #17:
// the old default cached the live session token as ".token.json" in the current
// working directory, where a routine `git add -A` could commit and push a working
// credential. Relocating it both removes the credential from the committable tree
// and spares the user a forced re-login.
//
// It is conservative and best-effort: it acts only when the resolved path differs
// from the legacy one, the legacy file holds a usable token, and the destination
// does not already exist (so a newer token is never clobbered). The legacy file
// is removed only after the new copy is safely written. Any failure is returned
// for the caller to log and is never fatal — the worst case is that the next call
// logs in fresh.
func MigrateLegacy(resolvedPath, legacyPath string) (migrated bool, err error) {
	if resolvedPath == "" || legacyPath == "" || resolvedPath == legacyPath {
		return false, nil
	}
	// Never overwrite a token already at the destination.
	if _, statErr := os.Stat(resolvedPath); statErr == nil {
		return false, nil
	}
	tok, ok, loadErr := Load(legacyPath)
	if loadErr != nil || !ok {
		// No usable legacy token (missing or corrupt) — nothing worth moving, and
		// a corrupt file carries no live credential to relocate.
		return false, nil
	}
	if err := Save(resolvedPath, tok); err != nil {
		return false, fmt.Errorf("migrate token cache to %s: %w", resolvedPath, err)
	}
	// The token now lives safely in the per-user dir; remove the old copy so the
	// credential no longer sits in the working directory.
	if err := os.Remove(legacyPath); err != nil {
		return true, fmt.Errorf("remove legacy token cache %s: %w", legacyPath, err)
	}
	return true, nil
}
