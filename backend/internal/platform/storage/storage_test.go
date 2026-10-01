package storage

import (
	"context"
	"strings"
	"testing"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/config"
)

func TestSanitizePrefix(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name    string
		in      string
		want    string
		wantErr bool
	}{
		{name: "empty", in: "", want: ""},
		{name: "trim slash", in: "/projects/a", want: "projects/a"},
		{name: "space", in: "  docs/  ", want: "docs/"},
		{name: "dotdot", in: "a/../b", wantErr: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got, err := sanitizePrefix(tc.in)
			if tc.wantErr {
				if err == nil {
					t.Fatal("expected error")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tc.want {
				t.Fatalf("got %q want %q", got, tc.want)
			}
		})
	}
}

func TestSanitizePathRequiresKey(t *testing.T) {
	t.Parallel()
	if _, err := sanitizePath(""); err == nil {
		t.Fatal("expected error for empty path")
	}
	got, err := sanitizePath("/a/b.txt")
	if err != nil {
		t.Fatal(err)
	}
	if got != "a/b.txt" {
		t.Fatalf("got %q", got)
	}
}

func TestIsSystemKey(t *testing.T) {
	t.Parallel()
	if !IsSystemKey("__system/trash/x") {
		t.Fatal("expected system key")
	}
	if IsSystemKey("uploads/a.png") {
		t.Fatal("did not expect system key")
	}
}

func TestNewFromConfigRejectsMinIO(t *testing.T) {
	_, err := NewFromConfig(context.Background(), config.StorageConfig{Driver: "minio"})
	if err == nil || !strings.Contains(err.Error(), "STORAGE_DRIVER=s3") {
		t.Fatalf("expected minio removal error, got %v", err)
	}
}
