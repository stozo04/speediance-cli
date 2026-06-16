package cli

import (
	"bytes"
	"encoding/json"
	"io"
	"os"
)

// writeJSON emits v as machine-readable JSON matching Python's
// json.dumps(indent=2): two-space indent, a trailing newline, and — crucially —
// HTML escaping OFF so characters like &, <, > survive byte-for-byte. The
// Python tool never escapes these, and agents parse our stdout, so the
// `--json` output must match exactly (GOAL.md §2, §9).
//
// Callers pass cmd.OutOrStdout() so tests can capture output (GOAL.md §13).
func writeJSON(w io.Writer, v any) error {
	enc := json.NewEncoder(w)
	enc.SetEscapeHTML(false)
	enc.SetIndent("", "  ")
	return enc.Encode(v) // Encode already appends the trailing newline.
}

// writeJSONFile writes v to a file with the same formatting as Python's
// json.dump(..., ensure_ascii=False, indent=2): two-space indent, HTML escaping
// off, and NO trailing newline (json.dump adds none). Used for `library --out`.
func writeJSONFile(path string, v any) error {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	enc.SetIndent("", "  ")
	if err := enc.Encode(v); err != nil {
		return err
	}
	// Encode appends a trailing newline; strip it to match json.dump exactly.
	out := bytes.TrimRight(buf.Bytes(), "\n")
	return os.WriteFile(path, out, 0o644)
}
