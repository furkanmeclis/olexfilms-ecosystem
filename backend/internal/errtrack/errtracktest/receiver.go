// Package errtracktest provides a fake Sentry receiver (envelope endpoint
// like technowide errortracking) for tests.
package errtracktest

import (
	"bufio"
	"bytes"
	"compress/gzip"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"
)

// ProjectID is the source id used in the receiver DSN.
const ProjectID = "42"

// Key is the public key used in the receiver DSN.
const Key = "0123456789abcdef0123456789abcdef"

// Exception is the subset of an exception value the tests inspect.
type Exception struct {
	Type  string `json:"type"`
	Value string `json:"value"`
}

// Event is the subset of an event payload the tests inspect.
type Event struct {
	EventID     string            `json:"event_id"`
	Message     string            `json:"message"`
	Environment string            `json:"environment"`
	Release     string            `json:"release"`
	ServerName  string            `json:"server_name"`
	Tags        map[string]string `json:"tags"`
	Fingerprint []string          `json:"fingerprint"`
	User        map[string]any    `json:"user"`
	Request     map[string]any    `json:"request"`
	// ExceptionRaw is either a list or {"values": [...]} (both are valid).
	ExceptionRaw json.RawMessage `json:"exception"`
	Raw          json.RawMessage `json:"-"`
}

// Exceptions returns the exception chain (outermost last).
func (e Event) Exceptions() []Exception {
	var list []Exception
	if json.Unmarshal(e.ExceptionRaw, &list) == nil {
		return list
	}
	var wrapped struct {
		Values []Exception `json:"values"`
	}
	_ = json.Unmarshal(e.ExceptionRaw, &wrapped)
	return wrapped.Values
}

// Receiver is an httptest server accepting POST /api/{id}/envelope/.
type Receiver struct {
	srv    *httptest.Server
	mu     sync.Mutex
	events []Event
	paths  []string
}

// NewReceiver starts the fake receiver; it is closed with t.Cleanup.
func NewReceiver(t testing.TB) *Receiver {
	t.Helper()
	rcv := &Receiver{}
	rcv.srv = httptest.NewServer(http.HandlerFunc(rcv.handle))
	t.Cleanup(rcv.srv.Close)
	return rcv
}

// DSN returns a DSN pointing to the receiver.
func (r *Receiver) DSN() string {
	host := strings.TrimPrefix(r.srv.URL, "http://")
	return fmt.Sprintf("http://%s@%s/%s", Key, host, ProjectID)
}

// Events returns the events received so far.
func (r *Receiver) Events() []Event {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]Event, len(r.events))
	copy(out, r.events)
	return out
}

// Paths returns the request paths received so far.
func (r *Receiver) Paths() []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]string(nil), r.paths...)
}

// WaitEvents polls until at least n events arrived or timeout passes.
func (r *Receiver) WaitEvents(n int, timeout time.Duration) []Event {
	deadline := time.Now().Add(timeout)
	for {
		ev := r.Events()
		if len(ev) >= n || time.Now().After(deadline) {
			return ev
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func (r *Receiver) handle(w http.ResponseWriter, req *http.Request) {
	want := "/api/" + ProjectID + "/envelope/"
	if req.Method != http.MethodPost || strings.TrimSuffix(req.URL.Path, "/")+"/" != want {
		http.NotFound(w, req)
		return
	}
	var body io.Reader = req.Body
	if req.Header.Get("Content-Encoding") == "gzip" {
		gz, err := gzip.NewReader(req.Body)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		defer func() { _ = gz.Close() }()
		body = gz
	}
	raw, err := io.ReadAll(body)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	events, err := ParseEnvelope(raw)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	r.mu.Lock()
	r.paths = append(r.paths, req.URL.Path)
	r.events = append(r.events, events...)
	r.mu.Unlock()
	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write([]byte(`{"id":"ok"}`))
}

// ParseEnvelope decodes the event items of a Sentry envelope:
// header line, then (item header line, payload) pairs; payloads use the
// item "length" when present, otherwise run to the next newline.
func ParseEnvelope(raw []byte) ([]Event, error) {
	rd := bufio.NewReader(bytes.NewReader(raw))
	if _, err := readLine(rd); err != nil {
		return nil, fmt.Errorf("envelope header: %w", err)
	}
	var events []Event
	for {
		line, err := readLine(rd)
		if err == io.EOF && len(line) == 0 {
			return events, nil
		}
		if err != nil && err != io.EOF {
			return nil, err
		}
		if len(bytes.TrimSpace(line)) == 0 {
			if err == io.EOF {
				return events, nil
			}
			continue
		}
		var hdr struct {
			Type   string          `json:"type"`
			Length json.RawMessage `json:"length"`
		}
		if jerr := json.Unmarshal(line, &hdr); jerr != nil {
			return nil, fmt.Errorf("item header: %w", jerr)
		}
		var payload []byte
		if n, perr := strconv.Atoi(string(hdr.Length)); perr == nil && len(hdr.Length) > 0 {
			payload = make([]byte, n)
			if _, rerr := io.ReadFull(rd, payload); rerr != nil {
				return nil, fmt.Errorf("item payload: %w", rerr)
			}
			_, _ = rd.ReadByte() // trailing newline
		} else {
			payload, err = readLine(rd)
			if err != nil && err != io.EOF {
				return nil, err
			}
		}
		if hdr.Type != "event" {
			continue
		}
		var ev Event
		if jerr := json.Unmarshal(payload, &ev); jerr != nil {
			return nil, fmt.Errorf("event payload: %w", jerr)
		}
		ev.Raw = append(json.RawMessage(nil), payload...)
		events = append(events, ev)
	}
}

func readLine(rd *bufio.Reader) ([]byte, error) {
	line, err := rd.ReadBytes('\n')
	return bytes.TrimRight(line, "\n"), err
}
