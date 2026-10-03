package migrations

import (
	"os"
	"regexp"
	"strconv"
	"testing"
	"testing/fstest"
)

// The embedded version is the highest up file of the directory on disk.
func TestLatestVersionMatchesDirectory(t *testing.T) {
	got, err := LatestVersion()
	if err != nil {
		t.Fatal(err)
	}
	entries, err := os.ReadDir(".")
	if err != nil {
		t.Fatal(err)
	}
	re := regexp.MustCompile(`^([0-9]+)_.+\.up\.sql$`)
	var want uint64
	for _, e := range entries {
		if m := re.FindStringSubmatch(e.Name()); m != nil {
			v, _ := strconv.ParseUint(m[1], 10, 64)
			want = max(want, v)
		}
	}
	if got != want || got == 0 {
		t.Fatalf("LatestVersion = %d, want %d", got, want)
	}
}

func TestLatestVersionIgnoresOtherFiles(t *testing.T) {
	fsys := fstest.MapFS{
		"000002_b.up.sql":   {},
		"000010_c.down.sql": {},
		"000003_c.up.sql":   {},
		"README.md":         {},
	}
	if v, err := latestVersion(fsys); err != nil || v != 3 {
		t.Fatalf("latestVersion = %d, %v", v, err)
	}
	if _, err := latestVersion(fstest.MapFS{}); err == nil {
		t.Fatal("empty fs accepted")
	}
}
