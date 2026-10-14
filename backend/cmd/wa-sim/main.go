// Command wa-sim talks to the WhatsApp AI pipeline without a phone (TEC-640).
// It posts a wuzapi-shaped "Message" webhook, signed with
// WUZAPI_WEBHOOK_SECRET, to the backend's POST /hooks/wuzapi as if -from had
// written to the business number, then waits for the AI run that message
// triggers and prints the replies and tool calls. -from must match
// WHATSAPP_SANDBOX_PREFIXES so the replies never leave the backend.
//
// Usage (from the repo root, backend running):
//
//	make wa-sim                                        # interactive
//	make wa-sim ARGS='-m "Merhaba, randevu almak istiyorum"'
//	make wa-sim ARGS='-from +905000000002 -name Ayşe'
package main

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/config"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/whatsapp/wuzapi"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

func main() {
	from := flag.String("from", "+905000000001", "simulated sender (E.164, must match WHATSAPP_SANDBOX_PREFIXES)")
	name := flag.String("name", "Simülatör", "WhatsApp push name of the sender")
	msg := flag.String("m", "", "send this one message and exit (default: interactive)")
	url := flag.String("url", "", "backend base URL (default: http://127.0.0.1 + APP_HTTP_ADDR port)")
	wait := flag.Duration("wait", 2*time.Minute, "how long to wait for the AI run of a message")
	flag.Parse()

	cfg, err := config.Load()
	if err != nil {
		fatal("config: %v", err)
	}
	if cfg.Wuzapi.WebhookSecret == "" {
		fatal("WUZAPI_WEBHOOK_SECRET is empty")
	}
	sim := &simulator{
		secret: cfg.Wuzapi.WebhookSecret,
		hook:   strings.TrimRight(baseURL(*url, cfg.HTTP.Addr), "/") + "/hooks/wuzapi",
		from:   digits(*from),
		name:   *name,
		wait:   *wait,
		http:   &http.Client{Timeout: 30 * time.Second},
	}
	if sim.from == "" {
		fatal("-from is empty")
	}
	if !sandboxed(sim.from, cfg.Wuzapi.SandboxPrefixes) {
		fmt.Fprintf(os.Stderr, "uyarı: +%s WHATSAPP_SANDBOX_PREFIXES ile eşleşmiyor; cevaplar gerçek gönderime gider (allowlist izin verirse)\n", sim.from)
	}

	ctx := context.Background()
	pool, err := database.NewPostgresPool(ctx, cfg.DB)
	if err != nil {
		fatal("database: %v", err)
	}
	defer pool.Close()
	sim.db = pool

	if *msg != "" {
		if err := sim.turn(ctx, *msg); err != nil {
			fatal("%v", err)
		}
		return
	}
	fmt.Printf("wa-sim: +%s → %s (çıkış: Ctrl+D)\n", sim.from, sim.hook)
	in := bufio.NewScanner(os.Stdin)
	for {
		fmt.Print("\nsen> ")
		if !in.Scan() {
			fmt.Println()
			return
		}
		text := strings.TrimSpace(in.Text())
		if text == "" {
			continue
		}
		if err := sim.turn(ctx, text); err != nil {
			fmt.Fprintf(os.Stderr, "hata: %v\n", err)
		}
	}
}

type simulator struct {
	secret string
	hook   string
	from   string
	name   string
	wait   time.Duration
	http   *http.Client
	db     *pgxpool.Pool
}

// turn sends one message and prints what the AI run it triggers produced.
func (s *simulator) turn(ctx context.Context, text string) error {
	id := fmt.Sprintf("SIM%d", time.Now().UnixNano())
	if err := s.post(ctx, id, text); err != nil {
		return err
	}
	msgID, convID, err := s.inbound(ctx, id)
	if err != nil {
		return err
	}
	run, err := s.awaitRun(ctx, convID, msgID)
	if err != nil {
		return err
	}
	// The reply is queued, then sent by the worker: give it a moment.
	replies, err := s.awaitReplies(ctx, convID, msgID, 10*time.Second)
	if err != nil {
		return err
	}
	for _, r := range replies {
		fmt.Printf("ai> %s\n", r.body)
		if r.media != "" {
			fmt.Printf("    [ek: %s]\n", r.media)
		}
		if r.status != "sent" && r.status != "delivered" && r.status != "read" {
			fmt.Printf("    [durum: %s %s]\n", r.status, r.failure)
		}
	}
	run.print()
	return nil
}

func (s *simulator) post(ctx context.Context, id, text string) error {
	jid := s.from + "@s.whatsapp.net"
	body, err := json.Marshal(map[string]any{
		"type": "Message",
		"event": map[string]any{
			"Info": map[string]any{
				"ID": id, "Chat": jid, "Sender": jid, "IsFromMe": false, "IsGroup": false,
				"PushName": s.name, "Timestamp": time.Now().UTC().Format(time.RFC3339), "Type": "text",
			},
			"Message": map[string]any{"conversation": text},
		},
	})
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, s.hook, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set(wuzapi.SignatureHeader, wuzapi.Sign(s.secret, body))
	resp, err := s.http.Do(req)
	if err != nil {
		return fmt.Errorf("webhook: %w (backend çalışıyor mu?)", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode/100 != 2 {
		b, _ := io.ReadAll(io.LimitReader(resp.Body, 2048))
		return fmt.Errorf("webhook: HTTP %d: %s", resp.StatusCode, strings.TrimSpace(string(b)))
	}
	return nil
}

// inbound finds the stored inbound message of external id.
func (s *simulator) inbound(ctx context.Context, externalID string) (msgID, convID int64, err error) {
	err = s.db.QueryRow(ctx,
		`SELECT id, conversation_id FROM messages WHERE external_id = $1 AND direction = 'in'`,
		externalID).Scan(&msgID, &convID)
	if errors.Is(err, pgx.ErrNoRows) {
		return 0, 0, fmt.Errorf("mesaj kaydedilmedi (gönderen çözümlenemedi ya da webhook yok sayıldı; backend loguna bak)")
	}
	return msgID, convID, err
}

type aiRun struct {
	status    string
	model     string
	toolCalls json.RawMessage
	stages    json.RawMessage
	errText   *string
	duration  *int32
	inTokens  int64
	outTokens int64
}

// awaitRun waits until an AI run of the conversation that covers msgID (the
// debounce may fold several messages into one later trigger) has finished.
func (s *simulator) awaitRun(ctx context.Context, convID, msgID int64) (*aiRun, error) {
	deadline := time.Now().Add(s.wait)
	for time.Now().Before(deadline) {
		var r aiRun
		err := s.db.QueryRow(ctx, `
			SELECT status, model, tool_calls, stages, error, duration_ms, input_tokens, output_tokens
			FROM conversation_ai_runs
			WHERE conversation_id = $1 AND trigger_message_id >= $2
			ORDER BY id DESC LIMIT 1`, convID, msgID).
			Scan(&r.status, &r.model, &r.toolCalls, &r.stages, &r.errText, &r.duration, &r.inTokens, &r.outTokens)
		if err != nil && !errors.Is(err, pgx.ErrNoRows) {
			return nil, err
		}
		if err == nil && r.status != "running" {
			return &r, nil
		}
		time.Sleep(700 * time.Millisecond)
	}
	return nil, fmt.Errorf("%s içinde AI çalışması bitmedi (AI kapalı, konuşma panelde devralınmış ya da worker çalışmıyor olabilir)", s.wait)
}

type reply struct {
	body, status, failure, media string
}

// awaitReplies returns the outbound messages after msgID, waiting up to max
// for the queued ones to be sent.
func (s *simulator) awaitReplies(ctx context.Context, convID, msgID int64, max time.Duration) ([]reply, error) {
	deadline := time.Now().Add(max)
	for {
		rows, err := s.db.Query(ctx, `
			SELECT COALESCE(body, ''), status, COALESCE(failure_reason, ''), COALESCE(media->>'file_name', '')
			FROM messages
			WHERE conversation_id = $1 AND id > $2 AND direction = 'out'
			ORDER BY id`, convID, msgID)
		if err != nil {
			return nil, err
		}
		var out []reply
		pending := false
		for rows.Next() {
			var r reply
			if err := rows.Scan(&r.body, &r.status, &r.failure, &r.media); err != nil {
				rows.Close()
				return nil, err
			}
			if r.status == "queued" {
				pending = true
			}
			out = append(out, r)
		}
		rows.Close()
		if err := rows.Err(); err != nil {
			return nil, err
		}
		if !pending || time.Now().After(deadline) {
			return out, nil
		}
		time.Sleep(500 * time.Millisecond)
	}
}

func (r *aiRun) print() {
	line := fmt.Sprintf("    [run: %s, model %s, %d/%d token", r.status, r.model, r.inTokens, r.outTokens)
	if r.duration != nil {
		line += fmt.Sprintf(", %d ms", *r.duration)
	}
	fmt.Println(line + "]")
	var calls []map[string]any
	if json.Unmarshal(r.toolCalls, &calls) == nil {
		for _, c := range calls {
			b, _ := json.Marshal(c)
			fmt.Printf("    [tool: %s]\n", b)
		}
	}
	if r.errText != nil && *r.errText != "" {
		fmt.Printf("    [hata: %s]\n", *r.errText)
	}
	if r.status == "skipped" {
		fmt.Printf("    [aşamalar: %s]\n", r.stages)
	}
}

func baseURL(flagURL, addr string) string {
	if flagURL != "" {
		return flagURL
	}
	port := "8080"
	if i := strings.LastIndex(addr, ":"); i >= 0 && addr[i+1:] != "" {
		port = addr[i+1:]
	}
	return "http://127.0.0.1:" + port
}

func sandboxed(d string, prefixes []string) bool {
	for _, p := range prefixes {
		if p = digits(p); p != "" && strings.HasPrefix(d, p) {
			return true
		}
	}
	return false
}

func digits(raw string) string {
	var b strings.Builder
	for _, r := range raw {
		if r >= '0' && r <= '9' {
			b.WriteRune(r)
		}
	}
	return b.String()
}

func fatal(format string, args ...any) {
	fmt.Fprintf(os.Stderr, "wa-sim: "+format+"\n", args...)
	os.Exit(1)
}
