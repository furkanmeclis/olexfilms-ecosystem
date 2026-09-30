package events

import (
	"context"
	"errors"
	"log/slog"
	"testing"
	"time"
)

func TestBus_PublishInvokesSubscribers(t *testing.T) {
	t.Parallel()
	bus := NewBus(slog.Default())
	called := 0
	tenantID := int64(3)
	bus.Subscribe(CustomersCreated, func(_ context.Context, event Event) error {
		called++
		if event.Name != CustomersCreated {
			t.Fatalf("name = %s", event.Name)
		}
		if event.TenantID == nil || *event.TenantID != tenantID {
			t.Fatalf("tenant = %#v", event.TenantID)
		}
		if event.EventID.String() == "" {
			t.Fatal("expected event id")
		}
		return nil
	})
	err := bus.Publish(context.Background(), New(CustomersCreated).WithTenant(tenantID))
	if err != nil {
		t.Fatal(err)
	}
	if called != 1 {
		t.Fatalf("called = %d", called)
	}
}

func TestBus_PublishPrefixSubscribe(t *testing.T) {
	t.Parallel()
	bus := NewBus(nil)
	called := 0
	bus.Subscribe("customers.*", func(_ context.Context, event Event) error {
		called++
		if event.Name != CustomersFlagAdded {
			t.Fatalf("name = %s", event.Name)
		}
		return nil
	})
	if err := bus.Publish(context.Background(), New(CustomersFlagAdded)); err != nil {
		t.Fatal(err)
	}
	if called != 1 {
		t.Fatalf("called = %d", called)
	}
}

func TestBus_PublishRejectsInvalidName(t *testing.T) {
	t.Parallel()
	bus := NewBus(nil)
	if err := bus.Publish(context.Background(), Event{Name: "Bad.Name"}); err == nil {
		t.Fatal("expected error")
	}
	if err := bus.Publish(context.Background(), Event{Name: "nodot"}); err == nil {
		t.Fatal("expected error")
	}
	if err := bus.Publish(context.Background(), Event{}); err == nil {
		t.Fatal("expected error")
	}
}

func TestBus_HandlerErrorIsFailSoft(t *testing.T) {
	t.Parallel()
	bus := NewBus(slog.Default())
	second := false
	bus.Subscribe("demo.ping", func(context.Context, Event) error {
		return errors.New("boom")
	})
	bus.Subscribe("demo.ping", func(context.Context, Event) error {
		second = true
		return nil
	})
	if err := bus.Publish(context.Background(), New("demo.ping")); err != nil {
		t.Fatal(err)
	}
	if !second {
		t.Fatal("expected second handler to run after first error")
	}
}

func TestEvent_OccurredAtDefaults(t *testing.T) {
	t.Parallel()
	before := time.Now().UTC().Add(-time.Second)
	event := New(CustomersUpdated).WithActor(1)
	if event.Name != CustomersUpdated {
		t.Fatalf("name = %s", event.Name)
	}
	if event.OccurredAt.Before(before) {
		t.Fatal("expected recent OccurredAt")
	}
	if event.EventID.String() == "00000000-0000-0000-0000-000000000000" {
		t.Fatal("expected non-nil EventID")
	}
}
