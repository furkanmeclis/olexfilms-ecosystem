package docs

import (
	"bytes"
	"errors"
	"io/fs"
	"os"
	"testing"
)

// TestSpecInSync guards the embedded copy against drifting from the
// canonical repo-root spec (docs/openapi.yaml).
func TestSpecInSync(t *testing.T) {
	canonical, err := os.ReadFile("../../docs/openapi.yaml")
	if errors.Is(err, fs.ErrNotExist) {
		t.Skip("canonical spec not present (backend-only checkout)")
	}
	if err != nil {
		t.Fatal(err)
	}
	embedded, err := FS.ReadFile("openapi.yaml")
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(canonical, embedded) {
		t.Fatal("backend/docs/openapi.yaml differs from docs/openapi.yaml; run `make openapi-sync`")
	}
}
