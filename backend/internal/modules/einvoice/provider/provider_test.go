package provider

import (
	"context"
	"errors"
	"testing"
)

func TestNoopIsNotConfigured(t *testing.T) {
	var p Provider = Noop{}
	ctx := context.Background()
	if _, err := p.Send(ctx, Envelope{UUID: "u"}); !errors.Is(err, ErrNotConfigured) {
		t.Fatalf("Send: %v", err)
	}
	if _, err := p.Status(ctx, "u"); !errors.Is(err, ErrNotConfigured) {
		t.Fatalf("Status: %v", err)
	}
	if _, err := p.Cancel(ctx, "u", "r"); !errors.Is(err, ErrNotConfigured) {
		t.Fatalf("Cancel: %v", err)
	}
}
