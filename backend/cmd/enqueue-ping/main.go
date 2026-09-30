package main

import (
	"fmt"
	"os"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/config"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/queue"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		_, _ = os.Stderr.WriteString("config load failed: " + err.Error() + "\n")
		os.Exit(1)
	}

	message := "hello from enqueue-ping"
	if len(os.Args) > 1 && os.Args[1] != "" {
		message = os.Args[1]
	}

	client := queue.NewClient(cfg.Redis)
	defer func() { _ = client.Close() }()

	task, err := queue.NewPingTask(message)
	if err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "build ping task: %v\n", err)
		os.Exit(1)
	}

	info, err := client.Enqueue(task)
	if err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "enqueue ping: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("enqueued %s id=%s queue=%s\n", info.Type, info.ID, info.Queue)
}
