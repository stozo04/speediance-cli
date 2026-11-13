package config

import (
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"slices"
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

// TestSkillDocAdvertisesFileAccess extends the §0 (advertised == actual) guard
// to the SKILL.md permissions block (CLAWHUB_STANDARDS §4): every file the
// binary actually reads or writes must appear in the corresponding files.read /
// files.write list, so the published skill never under-declares a capability.
// It asserts the negative — a real file access MISSING from the block fails
// loudly — the exact drift that previously left the `config set` write of
// config.json and the working-directory .env read unadvertised.
func TestSkillDocAdvertisesFileAccess(t *testing.T) {
	skill, err := os.ReadFile(filepath.Join(repoRoot(t), "SKILL.md"))
	if err != nil {
		t.Fatalf("read SKILL.md: %v", err)
	}
	doc := string(skill)

	// Slice one permissions list out of the frontmatter by its start key and the
	// key that follows it (order: network, files.read, files.write, requires).
	block := func(start, end string) string {
		t.Helper()
		i := strings.Index(doc, start)
		if i < 0 {
			t.Fatalf("SKILL.md permissions block missing %q", start)
		}
		rest := doc[i+len(start):]
		j := strings.Index(rest, end)
		if j < 0 {
			t.Fatalf("SKILL.md: no %q after %q — permissions block reordered?", end, start)
		}
		return rest[:j]
	}
	reads := block("files.read:", "files.write:")
	writes := block("files.write:", "requires:")

	// Everything the binary reads: config.json (internal/config discovery), the
	// token cache (auth.Load), plan JSON (template.LoadPlan), and the
	// working-directory .env (godotenv.Read in internal/config).
	for _, want := range []string{"config.json", "token", "plan", ".env"} {
		if !strings.Contains(reads, want) {
			t.Errorf("SKILL.md files.read does not mention %q, which the code reads — advertised != actual (CLAWHUB_STANDARDS §4)", want)
		}
	}
	// Everything the binary writes: the token cache (auth.Save), library.json
	// (library --out via writeJSONFile), and config.json (config set → setConfigKey).
	for _, want := range []string{"token", "library.json", "config.json"} {
		if !strings.Contains(writes, want) {
			t.Errorf("SKILL.md files.write does not mention %q, which the code writes — advertised != actual (CLAWHUB_STANDARDS §4)", want)
		}
	}
}

func TestSkillDocPromisesNoShellOut(t *testing.T) {
	root := repoRoot(t)

	skill, err := os.ReadFile(filepath.Join(root, "SKILL.md"))
	if err != nil {
		t.Fatalf("read SKILL.md: %v", err)
	}
	if !strings.Contains(string(skill), "      bins:\n        - speediance-cli\n      env:") {
		t.Error("SKILL.md requires.bins must list exactly speediance-cli: OpenClaw gates the skill on it, and the binary needs no other tool")
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
			t.Errorf("%s imports os/exec, but the skill declares no binary besides speediance-cli — advertised != actual", rel)
		}
		return nil
	})
	if walkErr != nil {
		t.Fatalf("walk source tree: %v", walkErr)
	}
}

func skillFrontmatter(t *testing.T) string {
	t.Helper()
	skill, err := os.ReadFile(filepath.Join(repoRoot(t), "SKILL.md"))
	if err != nil {
		t.Fatalf("read SKILL.md: %v", err)
	}
	doc := strings.TrimPrefix(string(skill), "---\n")
	end := strings.Index(doc, "\n---")
	if end < 0 {
		t.Fatal("SKILL.md has no closing frontmatter fence")
	}
	return doc[:end]
}

var agentSkillsFrontmatterFields = []string{"name", "description", "license", "compatibility", "metadata", "allowed-tools"}

var agentSkillsName = regexp.MustCompile(`^[a-z0-9]+(-[a-z0-9]+)*$`)

const agentSkillsMaxNameLen = 64

const agentSkillsMaxDescriptionLen = 1024

func TestSkillFrontmatterFollowsAgentSkillsSpec(t *testing.T) {
	fm := skillFrontmatter(t)
	var name string
	for _, line := range strings.Split(fm, "\n") {
		if line == "" || line[0] == ' ' || line[0] == '#' {
			continue
		}
		key, value, _ := strings.Cut(line, ":")
		if !slices.Contains(agentSkillsFrontmatterFields, key) {
			t.Errorf("SKILL.md top-level key %q is not an Agent Skills field %q; put runtime metadata under metadata.openclaw", key, agentSkillsFrontmatterFields)
		}
		if key == "name" {
			name = strings.TrimSpace(value)
		}
	}
	if len(name) == 0 || len(name) > agentSkillsMaxNameLen || !agentSkillsName.MatchString(name) {
		t.Errorf("SKILL.md name %q breaks the Agent Skills name rules", name)
	}
	for _, harness := range []string{".claude", ".codex", ".cursor"} {
		dir := filepath.Join(repoRoot(t), harness, "skills", name)
		if _, err := os.Stat(filepath.Join(dir, "SKILL.md")); err != nil {
			t.Errorf("no %s/skills/%s/SKILL.md: Agent Skills requires the directory to match the skill name", harness, name)
		}
	}
	description := skillDescription(t, fm)
	if n := len([]rune(description)); n == 0 || n > agentSkillsMaxDescriptionLen {
		t.Errorf("SKILL.md description is %d characters; Agent Skills allows 1-%d", n, agentSkillsMaxDescriptionLen)
	}
	if !strings.Contains(description, "Use when") {
		t.Error("SKILL.md description should say when to use the skill (Agent Skills spec)")
	}
}

func skillDescription(t *testing.T, fm string) string {
	t.Helper()
	start := strings.Index(fm, "description:")
	end := strings.Index(fm, "\nmetadata:")
	if start < 0 || end < start {
		t.Fatal("SKILL.md description must come right before metadata")
	}
	body := strings.TrimPrefix(fm[start:end], "description:")
	body = strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(body), ">"))
	return strings.Join(strings.Fields(body), " ")
}

func TestSkillDescriptionDisclosesLocalWrites(t *testing.T) {
	description := skillDescription(t, skillFrontmatter(t))
	for _, want := range []string{"token cache", "config set", "password", "library"} {
		if !strings.Contains(description, want) {
			t.Errorf("SKILL.md description does not disclose %q; SkillSpector TP4 compares the description with what the code writes", want)
		}
	}
}

func TestAgentDocsRequireExplicitRequestBeforeReadingWorkouts(t *testing.T) {
	root := repoRoot(t)
	for _, rel := range []string{"SKILL.md", filepath.Join("docs", "MACHINE_CONTRACT.md")} {
		data, err := os.ReadFile(filepath.Join(root, rel))
		if err != nil {
			t.Fatalf("read %s: %v", rel, err)
		}
		doc := strings.Join(strings.Fields(string(data)), " ")
		if !strings.Contains(doc, "A passing mention of exercising is not a request to read their account: ask first.") {
			t.Errorf("%s must tell agents to ask before calling today on a passing workout mention (SkillSpector SQP-1)", rel)
		}
		for _, broad := range []string{`just says "I did a workout," call`, "says they did a workout, call"} {
			if strings.Contains(doc, broad) {
				t.Errorf("%s still treats a passing workout mention as a trigger: %q", rel, broad)
			}
		}
	}
}
