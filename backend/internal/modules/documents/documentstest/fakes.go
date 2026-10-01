// Package documentstest provides test doubles for the documents module: a
// fake Gotenberg server and a configurable SourceLoader. F1 modules can use
// them to test their document flows without Chromium.
package documentstest

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/documents/model"
)

// PDF is the body the fake Gotenberg returns.
const PDF = "%PDF-1.7\n%fake document\n"

// Gotenberg is an httptest server that answers every conversion with PDF,
// counts calls and records the submitted index.html documents.
type Gotenberg struct {
	Calls atomic.Int32
	Delay time.Duration
	URL   string
	mu    sync.Mutex
	html  []string
}

// NewGotenberg starts a fake Gotenberg (closed on test cleanup).
func NewGotenberg(t testing.TB, delay time.Duration) *Gotenberg {
	t.Helper()
	g := &Gotenberg{Delay: delay}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		g.Calls.Add(1)
		if err := r.ParseMultipartForm(32 << 20); err == nil {
			for _, fh := range r.MultipartForm.File["files"] {
				if fh.Filename != "index.html" {
					continue
				}
				file, err := fh.Open()
				if err != nil {
					continue
				}
				b, _ := io.ReadAll(file)
				_ = file.Close()
				g.mu.Lock()
				g.html = append(g.html, string(b))
				g.mu.Unlock()
			}
		}
		if g.Delay > 0 {
			time.Sleep(g.Delay)
		}
		w.Header().Set("Content-Type", "application/pdf")
		_, _ = io.WriteString(w, PDF)
	}))
	t.Cleanup(srv.Close)
	g.URL = srv.URL
	return g
}

// LastHTML returns the most recent submitted document.
func (g *Gotenberg) LastHTML() string {
	g.mu.Lock()
	defer g.mu.Unlock()
	if len(g.html) == 0 {
		return ""
	}
	return g.html[len(g.html)-1]
}

// Loader is a SourceLoader over in-memory records of one organization.
type Loader struct {
	Type           string
	OrganizationID int64
	BrandID        int64
	mu             sync.Mutex
	versions       map[string]string
	vars           map[string]string
}

// NewLoader builds a loader whose sources belong to one organization.
func NewLoader(orgID, brandID int64) *Loader {
	return &Loader{Type: "test_source", OrganizationID: orgID, BrandID: brandID, versions: map[string]string{}, vars: map[string]string{}}
}

// SourceType implements model.SourceLoader.
func (l *Loader) SourceType() string { return l.Type }

// SetVar sets a variable value for every source.
func (l *Loader) SetVar(key, value string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.vars[key] = value
}

// Bump changes a source's version (simulates an edit of the record).
func (l *Loader) Bump(id string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.versions[id] = fmt.Sprintf("%d", time.Now().UnixNano())
}

// Load implements model.SourceLoader: other organizations' viewers get
// ErrSourceNotFound.
func (l *Loader) Load(_ context.Context, viewer model.Viewer, id, _ string) (model.Source, error) {
	if !viewer.System && viewer.OrganizationID != l.OrganizationID {
		return model.Source{}, model.ErrSourceNotFound
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	v := l.versions[id]
	if v == "" {
		v = "1"
	}
	vars := map[string]string{"document_number": "HZM-" + id, "plate": "34 TEST " + id}
	for k, val := range l.vars {
		vars[k] = val
	}
	return model.Source{OrganizationID: l.OrganizationID, BrandID: l.BrandID, Version: v, Vars: vars, Title: "Hizmet " + id}, nil
}
