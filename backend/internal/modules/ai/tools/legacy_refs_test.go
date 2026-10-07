package tools

import (
	"bytes"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"testing"
)

// legacyRefAllowed are the only files that may name the legacy AI layer or
// the legacy WhatsApp gateway: the design document, the cutover runbook and
// the editor's English autocomplete word list (plain English words there).
var legacyRefAllowed = map[string]bool{
	"docs/design.md":                 true,
	"docs/runbooks/f4-ai-cutover.md": true,
	"frontend/src/components/editor/plugins/auto-complete-plugin.tsx": true,
}

// legacyRefSkipDirs are dependency, build and tool output directories.
var legacyRefSkipDirs = map[string]bool{
	".git": true, "node_modules": true, ".next": true, ".turbo": true, "coverage": true,
	"test-results": true, "playwright-report": true, "dist": true, "tmp": true, ".cache": true,
}

// TEC-409 acceptance: a case-insensitive search for the legacy WhatsApp gateway and the
// legacy AI layer's repo slug finds them only in the runbook and design.md
// (plus the word list). The pattern is split so this file does not match
// itself.
func TestNoLegacyAIReferences(t *testing.T) {
	root, err := filepath.Abs(filepath.Join("..", "..", "..", "..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(root, "docs", "design.md")); err != nil {
		t.Fatalf("repo root not found at %s: %v", root, err)
	}
	pattern := regexp.MustCompile("(?i)" + "evol" + "ution|ai-" + "layer")
	var hits []string
	err = filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if legacyRefSkipDirs[d.Name()] {
				return filepath.SkipDir
			}
			return nil
		}
		if !d.Type().IsRegular() {
			return nil
		}
		rel, _ := filepath.Rel(root, path)
		rel = filepath.ToSlash(rel)
		if legacyRefAllowed[rel] {
			return nil
		}
		raw, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		if bytes.IndexByte(raw[:min(len(raw), 8000)], 0) >= 0 {
			return nil // binary
		}
		if pattern.Match(raw) {
			hits = append(hits, rel)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(hits) > 0 {
		t.Errorf("legacy AI layer / WhatsApp gateway references outside the runbook and design.md: %v", hits)
	}
}
