// Package errtrack reports errors to the Sentry-compatible technowide
// errortracking receiver (K17). Every event carries a "module" tag from the
// fixed module list, plus request_id / organization_id where known. With an
// empty DSN every function is a no-op.
package errtrack

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"sync/atomic"
	"time"

	"github.com/getsentry/sentry-go"
)

// Release is the build version, injected at build time:
//
//	go build -ldflags "-X github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/errtrack.Release=v0.7"
//
// SENTRY_RELEASE at runtime overrides it.
var Release = ""

// Tag keys used on every event.
const (
	TagModule         = "module"
	TagRequestID      = "request_id"
	TagOrganizationID = "organization_id"
	TagBrandID        = "brand_id"
	TagTaskType       = "task_type"
	TagQueue          = "queue"
	TagComponent      = "component"
)

// Options configures the SDK.
type Options struct {
	DSN         string
	Environment string
	Release     string
	// ServiceName is set as the "component" tag (server, worker...).
	ServiceName string
	// Transport overrides the HTTP transport (tests).
	Transport sentry.Transport
	// Debug turns on SDK debug logging.
	Debug bool
}

// OptionsFromEnv reads SENTRY_DSN, SENTRY_ENVIRONMENT (default appEnv) and
// SENTRY_RELEASE (default the ldflags Release).
func OptionsFromEnv(appEnv, serviceName string) Options {
	env := strings.TrimSpace(os.Getenv("SENTRY_ENVIRONMENT"))
	if env == "" {
		env = appEnv
	}
	release := strings.TrimSpace(os.Getenv("SENTRY_RELEASE"))
	if release == "" {
		release = Release
	}
	return Options{
		DSN:         strings.TrimSpace(os.Getenv("SENTRY_DSN")),
		Environment: env,
		Release:     release,
		ServiceName: serviceName,
	}
}

var enabled atomic.Bool

// Enabled reports whether a DSN was configured and the client is live.
func Enabled() bool { return enabled.Load() }

// Init configures the global client. An empty DSN leaves the SDK disabled
// (all calls no-op) and returns (false, nil).
func Init(opts Options) (bool, error) {
	if strings.TrimSpace(opts.DSN) == "" {
		enabled.Store(false)
		return false, nil
	}
	tags := map[string]string{}
	if opts.ServiceName != "" {
		tags[TagComponent] = opts.ServiceName
	}
	err := sentry.Init(sentry.ClientOptions{
		Dsn:         opts.DSN,
		Environment: opts.Environment,
		Release:     opts.Release,
		// sendDefaultPii=false, stricter: DataCollection replaces the
		// deprecated SendDefaultPII flag. No user info, cookies, headers,
		// bodies or query params are collected; BeforeSend scrubs the rest.
		DataCollection:   noPII(),
		AttachStacktrace: true,
		// The receiver drops transactions/sessions; no tracing.
		EnableTracing:    false,
		TracesSampleRate: 0,
		BeforeSend:       beforeSend,
		BeforeBreadcrumb: func(bc *sentry.Breadcrumb, _ *sentry.BreadcrumbHint) *sentry.Breadcrumb {
			if bc != nil {
				bc.Message = ScrubString(bc.Message)
				bc.Data = scrubMap(bc.Data)
			}
			return bc
		},
		Transport:    opts.Transport,
		Tags:         tags,
		Debug:        opts.Debug,
		ServerName:   "-",
		IgnoreErrors: []string{context.Canceled.Error()},
	})
	if err != nil {
		enabled.Store(false)
		return false, fmt.Errorf("errtrack: init: %w", err)
	}
	enabled.Store(true)
	return true, nil
}

func noPII() *sentry.DataCollection {
	off := &sentry.KeyValueCollectionBehavior{Mode: sentry.CollectionOff}
	return &sentry.DataCollection{
		UserInfo:    sentry.Set(false),
		Cookies:     off,
		HTTPHeaders: &sentry.HeaderCollectionConfig{Request: off, Response: off},
		HTTPBodies:  []sentry.BodyType{},
		QueryParams: off,
	}
}

// Flush waits up to timeout for buffered events to be delivered.
func Flush(timeout time.Duration) bool {
	if !Enabled() {
		return true
	}
	return sentry.Flush(timeout)
}

// Tags are extra event tags.
type Tags map[string]string

func hubFrom(ctx context.Context) *sentry.Hub {
	if ctx != nil {
		if h := sentry.GetHubFromContext(ctx); h != nil {
			return h
		}
	}
	return sentry.CurrentHub().Clone()
}

// Capture reports err under module with optional extra tags. A module pinned
// on ctx with WithModule wins over the argument. No-op when disabled, when
// err is nil or when err is context.Canceled.
func Capture(ctx context.Context, module Module, err error, tags Tags) *sentry.EventID {
	if !Enabled() || err == nil || errors.Is(err, context.Canceled) {
		return nil
	}
	if ctx == nil {
		ctx = context.Background()
	}
	if m, ok := ModuleFrom(ctx); ok {
		module = m
	}
	module = module.Normalize()
	hub := hubFrom(ctx)
	var id *sentry.EventID
	hub.WithScope(func(scope *sentry.Scope) {
		applyContextTags(ctx, scope)
		scope.SetTag(TagModule, string(module))
		for k, v := range tags {
			if v != "" {
				scope.SetTag(k, v)
			}
		}
		// Group per module first, then the SDK's default grouping.
		scope.SetFingerprint([]string{string(module), "{{ default }}"})
		id = hub.CaptureException(err)
	})
	return id
}

// CaptureMessage reports a plain message (no error value) under module.
func CaptureMessage(ctx context.Context, module Module, msg string, tags Tags) *sentry.EventID {
	if !Enabled() || msg == "" {
		return nil
	}
	return Capture(ctx, module, errors.New(msg), tags)
}

// PanicError wraps a recovered panic value.
type PanicError struct {
	Value any
}

func (e *PanicError) Error() string { return fmt.Sprintf("panic: %v", e.Value) }

// Unwrap returns the panic value when it is an error.
func (e *PanicError) Unwrap() error {
	if err, ok := e.Value.(error); ok {
		return err
	}
	return nil
}
