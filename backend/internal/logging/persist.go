package logging

import (
	"context"
	"log/slog"
	"path"
	"runtime"
	"strings"
	"sync"
	"time"
)

const (
	maxMessageBytes = 8 << 10
	maxAttrBytes    = 16 << 10
	persistBuffer   = 1024
)

// Entry is a persistable log record (info is never stored).
type Entry struct {
	Time      time.Time
	Level     string
	Message   string
	Source    string
	Attrs     map[string]any
	RequestID string
}

// Writer stores persistable log entries.
type Writer interface {
	WriteLog(ctx context.Context, entry Entry) error
}

// Persist wraps a slog handler and asynchronously stores non-info records.
type Persist struct {
	handler slog.Handler
	logger  *slog.Logger
	ch      chan Entry
	writer  Writer
	stdout  slog.Handler
	stop    chan struct{}
	wg      sync.WaitGroup
}

// Attach returns a logger that writes to next and persists warn/error/debug.
func Attach(next *slog.Logger, writer Writer) *Persist {
	if next == nil {
		next = slog.Default()
	}
	stdout := next.Handler()
	p := &Persist{
		ch:     make(chan Entry, persistBuffer),
		writer: writer,
		stdout: stdout,
		stop:   make(chan struct{}),
	}
	h := &persistHandler{next: stdout, persist: p}
	p.handler = h
	p.logger = slog.New(h)
	p.wg.Add(1)
	go p.loop()
	return p
}

// Logger is the wrapped slog logger.
func (p *Persist) Logger() *slog.Logger {
	if p == nil || p.logger == nil {
		return slog.Default()
	}
	return p.logger
}

// Close drains the persist queue and stops the worker.
func (p *Persist) Close() {
	if p == nil {
		return
	}
	select {
	case <-p.stop:
		return
	default:
		close(p.stop)
	}
	p.wg.Wait()
}

func (p *Persist) loop() {
	defer p.wg.Done()
	for {
		select {
		case <-p.stop:
			for {
				select {
				case e := <-p.ch:
					p.flush(e)
				default:
					return
				}
			}
		case e := <-p.ch:
			p.flush(e)
		}
	}
}

func (p *Persist) flush(e Entry) {
	if p.writer == nil {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	if err := p.writer.WriteLog(ctx, e); err != nil && p.stdout != nil {
		rec := slog.NewRecord(time.Now(), slog.LevelError, "app_log_persist_failed", 0)
		rec.Add("error", err.Error())
		_ = p.stdout.Handle(ctx, rec)
	}
}

func (p *Persist) enqueue(e Entry) {
	select {
	case p.ch <- e:
	default:
		// Drop rather than blocking request/worker goroutines.
	}
}

type persistHandler struct {
	next    slog.Handler
	persist *Persist
	attrs   []slog.Attr
	groups  []string
}

func (h *persistHandler) Enabled(ctx context.Context, level slog.Level) bool {
	return h.next.Enabled(ctx, level)
}

func (h *persistHandler) Handle(ctx context.Context, rec slog.Record) error {
	if ShouldPersist(rec.Level) && h.persist != nil {
		h.persist.enqueue(h.toEntry(ctx, rec))
	}
	return h.next.Handle(ctx, rec)
}

func (h *persistHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	cp := *h
	cp.next = h.next.WithAttrs(attrs)
	cp.attrs = cloneAttrs(h.attrs, attrs)
	return &cp
}

func (h *persistHandler) WithGroup(name string) slog.Handler {
	cp := *h
	cp.next = h.next.WithGroup(name)
	if name != "" {
		cp.groups = append(append([]string{}, h.groups...), name)
	}
	return &cp
}

func (h *persistHandler) toEntry(ctx context.Context, rec slog.Record) Entry {
	attrs := flattenAttrs(h.groups, h.attrs, rec)
	reqID := RequestIDFromContext(ctx)
	if reqID == "" {
		if v, ok := attrs["request_id"].(string); ok {
			reqID = v
		}
	}
	msg := rec.Message
	if len(msg) > maxMessageBytes {
		msg = msg[:maxMessageBytes]
	}
	return Entry{
		Time:      rec.Time.UTC(),
		Level:     PersistLevel(rec.Level),
		Message:   msg,
		Source:    sourceFrom(rec, attrs),
		Attrs:     attrs,
		RequestID: reqID,
	}
}

// ShouldPersist reports whether a slog level is stored in the log viewer.
// Info (including HTTP access-style records) is excluded.
func ShouldPersist(level slog.Level) bool {
	return level != slog.LevelInfo
}

// PersistLevel maps a slog level onto the stored enum.
func PersistLevel(level slog.Level) string {
	switch {
	case level >= slog.LevelError:
		return "error"
	case level >= slog.LevelWarn:
		return "warn"
	default:
		return "debug"
	}
}

func sourceFrom(rec slog.Record, attrs map[string]any) string {
	for _, key := range []string{"source", "logger", "component"} {
		if v, ok := attrs[key].(string); ok {
			v = strings.TrimSpace(v)
			if v != "" {
				return truncate(v, 200)
			}
		}
	}
	if rec.PC != 0 {
		fs := runtime.CallersFrames([]uintptr{rec.PC})
		f, _ := fs.Next()
		if pkg := packageSource(f.Function); pkg != "" {
			return truncate(pkg, 200)
		}
		if f.File != "" {
			return truncate(path.Base(f.File), 200)
		}
	}
	return "app"
}

func packageSource(fn string) string {
	if fn == "" {
		return ""
	}
	// github.com/org/repo/backend/internal/modules/auth/usecase.(*AuthUseCase).Login
	const marker = "/internal/"
	i := strings.Index(fn, marker)
	if i < 0 {
		if dot := strings.LastIndex(fn, "."); dot > 0 {
			fn = fn[:dot]
		}
		return strings.TrimSuffix(fn, ".*")
	}
	rest := fn[i+len(marker):]
	if slash := strings.LastIndex(rest, "/"); slash >= 0 {
		rest = rest[:slash]
	}
	if paren := strings.Index(rest, "."); paren >= 0 {
		rest = rest[:paren]
	}
	return rest
}

func flattenAttrs(groups []string, pre []slog.Attr, rec slog.Record) map[string]any {
	out := make(map[string]any, 8)
	prefix := strings.Join(groups, ".")
	add := func(a slog.Attr) {
		writeAttr(out, prefix, a)
	}
	for _, a := range pre {
		add(a)
	}
	rec.Attrs(func(a slog.Attr) bool {
		add(a)
		return true
	})
	return out
}

func writeAttr(out map[string]any, prefix string, a slog.Attr) {
	a.Value = a.Value.Resolve()
	if a.Equal(slog.Attr{}) {
		return
	}
	key := a.Key
	if prefix != "" {
		key = prefix + "." + key
	}
	if a.Value.Kind() == slog.KindGroup {
		gprefix := key
		if a.Key == "" {
			gprefix = prefix
		}
		for _, child := range a.Value.Group() {
			writeAttr(out, gprefix, child)
		}
		return
	}
	if key == "" {
		return
	}
	out[key] = attrValue(a.Value)
}

func attrValue(v slog.Value) any {
	switch v.Kind() {
	case slog.KindString:
		return truncate(v.String(), maxAttrBytes)
	case slog.KindInt64:
		return v.Int64()
	case slog.KindUint64:
		return v.Uint64()
	case slog.KindFloat64:
		return v.Float64()
	case slog.KindBool:
		return v.Bool()
	case slog.KindDuration:
		return v.Duration().String()
	case slog.KindTime:
		return v.Time().UTC().Format(time.RFC3339Nano)
	case slog.KindAny:
		if err, ok := v.Any().(error); ok {
			return err.Error()
		}
		return v.Any()
	default:
		return v.Any()
	}
}

func cloneAttrs(base, extra []slog.Attr) []slog.Attr {
	out := make([]slog.Attr, 0, len(base)+len(extra))
	out = append(out, base...)
	out = append(out, extra...)
	return out
}

func truncate(s string, n int) string {
	if n <= 0 || len(s) <= n {
		return s
	}
	return s[:n]
}
