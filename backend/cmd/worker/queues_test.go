package main

import (
	"reflect"
	"testing"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/queue"
)

func TestParseWorkerQueues(t *testing.T) {
	all, err := parseWorkerQueues("")
	if err != nil || !reflect.DeepEqual(all, queue.DefaultQueues()) {
		t.Fatalf("empty = %v, %v; want every queue", all, err)
	}

	core, err := parseWorkerQueues("critical, default ,low")
	if err != nil {
		t.Fatal(err)
	}
	for _, q := range []string{queue.QueueNotifications, queue.QueueWhatsApp, "default", queue.QueueImports, queue.QueueBulk, queue.QueueSearch, queue.QueueMaintenance, queue.QueueLow} {
		if core[q] == 0 {
			t.Errorf("core worker misses queue %q", q)
		}
	}
	for _, q := range []string{queue.QueueExports, queue.QueueDocs} {
		if _, ok := core[q]; ok {
			t.Errorf("core worker must not consume the docs group queue %q", q)
		}
	}
	if core[queue.QueueNotifications] <= core["default"] || core["default"] <= core[queue.QueueMaintenance] {
		t.Errorf("weights must be critical > default > low: %v", core)
	}

	docs, err := parseWorkerQueues("docs")
	if err != nil || !reflect.DeepEqual(docs, map[string]int{queue.QueueDocs: 4, queue.QueueExports: 2}) {
		t.Fatalf("docs = %v, %v", docs, err)
	}

	// Core + docs together cover every queue the backend enqueues on.
	for q := range queue.DefaultQueues() {
		if core[q] == 0 && docs[q] == 0 {
			t.Errorf("queue %q is consumed by no production worker", q)
		}
	}

	raw, err := parseWorkerQueues("exports")
	if err != nil || raw[queue.QueueExports] != 1 {
		t.Fatalf("raw queue name = %v, %v", raw, err)
	}

	for _, bad := range []string{"nope", " , "} {
		if _, err := parseWorkerQueues(bad); err == nil {
			t.Errorf("%q should fail", bad)
		}
	}
}
