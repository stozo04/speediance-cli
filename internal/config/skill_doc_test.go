package config

import (
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// repoRoot locates the repository root from this test file's own path, so the
// SKILL.md / source-tree checks below run regardless of the working directory.
// This file lives at <root>/internal/config/skill_doc_test.go.
func repoRoot(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("cannot resolve caller path for repo root")
	}
	return filepath.Join(filepath.Dir(file), "..", "..")
}

// TestSkillDocAdvertisesEveryEnvVar is the automated §0 (advertised == actual)
// guard promised in CLI_CONVENTIONS.md: every SPEEDIANCE_* environment variable
// the loader actually reads must be documented in SKILL.md, so the published
// skill never hides a real input from the agent driving it. It asserts the
// negative — a code-read env var MISSING from SKILL.md fails loudly — which is
// exactly the drift that let `token_cache_path` be advertised-but-unwired before
// (issue #17). Add a new SPEEDIANCE_* knob to the loader and you must document it.
func TestSkillDocAdvertisesEveryEnvVar(t *testing.T) {
	skill, err := os.ReadFile(filepath.Join(repoRoot(t), "SKILL.md"))
	if err != nil {
		t.Fatalf("read SKILL.md: %v", err)
	}
	doc := string(skill)

	// The complete set of env vars the code consults (the frozen contract).
	for _, env := range []string{
		EnvEmail, EnvPassword, EnvRegion, EnvDeviceType, EnvConfig, EnvTokenCache,
	} {
		if !strings.Contains(doc, env) {
			t.Errorf("SKILL.md does not document env var %q that the loader reads — advertised != actual (CLI_CONVENTIONS.md §0)", env)
		}
	}
}

// TestSkillDocPromisesNoShellOut backs the same §0 cornerstone (and
// CLAWHUB_STANDARDS §4/§5): the skill advertises `requires.bins: []` — a single
// static binary that never shells out — so no production Go source may import
// os/exec. Asserting the negative across the tree fails the build the moment a
// shell-out is introduced without updating the advertised permissions.
func TestSkillDocPromisesNoShellOut(t *testing.T) {
	root := repoRoot(t)

	skill, err := os.ReadFile(filepath.Join(root, "SKILL.md"))
	if err != nil {
		t.Fatalf("read SKILL.md: %v", err)
	}
	if !strings.Contains(string(skill), "bins: []") {
		t.Error("SKILL.md must declare requires.bins: [] (single static binary, no external tools)")
	}

	walkErr := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if d.Name() == ".git" {
				return filepath.SkipDir
			}
			return nil
		}
		// Production code only: test files don't ship in the binary, and skipping
		// them also avoids this guard matching its own import string.
		if !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		src, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		if strings.Contains(string(src), `"os/exec"`) {
			rel, _ := filepath.Rel(root, path)
			t.Errorf("%s imports os/exec, but the skill advertises no shell-out (requires.bins: []) — advertised != actual", rel)
		}
		return nil
	})
	if walkErr != nil {
		t.Fatalf("walk source tree: %v", walkErr)
	}
}
