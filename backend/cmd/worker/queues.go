package main

import (
	"fmt"
	"sort"
	"strings"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/queue"
)

// queueGroups maps the WORKER_QUEUES groups used by compose.prod.yml to the
// Asynq queues the backend enqueues on (queue name → priority weight).
//
//	critical  user-facing delivery (notifications: OTP, WhatsApp, e-mail)
//	default   imports, bulk actions, search indexing, untagged tasks
//	low       maintenance sweeps (log purge)
//	docs      PDF documents (Gotenberg) and exports, run by worker-docs
var queueGroups = map[string]map[string]int{
	"critical": {queue.QueueNotifications: 6},
	"default": {
		"default":          3,
		queue.QueueImports: 3,
		queue.QueueBulk:    3,
		queue.QueueSearch:  3,
	},
	"low":  {queue.QueueMaintenance: 1},
	"docs": {queue.QueueDocs: 4, queue.QueueExports: 2},
}

// parseWorkerQueues turns WORKER_QUEUES ("critical,default,low") into the
// Asynq queue map. Empty means every queue (queue.DefaultQueues). A raw
// queue name (e.g. "exports") is accepted too, with weight 1.
func parseWorkerQueues(raw string) (map[string]int, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return queue.DefaultQueues(), nil
	}
	known := queue.DefaultQueues()
	out := map[string]int{}
	for _, part := range strings.Split(raw, ",") {
		name := strings.ToLower(strings.TrimSpace(part))
		if name == "" {
			continue
		}
		if group, ok := queueGroups[name]; ok {
			for q, w := range group {
				out[q] = w
			}
			continue
		}
		if _, ok := known[name]; ok {
			if _, set := out[name]; !set {
				out[name] = 1
			}
			continue
		}
		return nil, fmt.Errorf("WORKER_QUEUES: unknown queue or group %q (groups: %s)", name, strings.Join(groupNames(), ", "))
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("WORKER_QUEUES: no queue selected")
	}
	return out, nil
}

func groupNames() []string {
	names := make([]string, 0, len(queueGroups))
	for n := range queueGroups {
		names = append(names, n)
	}
	sort.Strings(names)
	return names
}
