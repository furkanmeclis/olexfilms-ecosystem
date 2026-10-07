package fake_test

import (
	"context"
	"errors"
	"reflect"
	"testing"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/llm"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/llm/fake"
)

// runToolLoop is a minimal agent loop: stream, run requested tools, feed the
// results back until the model ends its turn.
func runToolLoop(t *testing.T, p llm.Provider) ([]llm.Event, llm.Usage) {
	t.Helper()
	req := llm.Request{
		Model:    "claude-sonnet-5-5",
		System:   []llm.SystemBlock{{Text: "asistan"}},
		Tools:    []llm.ToolDef{{Name: "get_stock", InputSchema: map[string]any{"type": "object"}}},
		Messages: []llm.Message{{Role: llm.RoleUser, Content: []llm.Block{llm.TextBlock("Stokta kaç rulo var?")}}},
	}
	var (
		all   []llm.Event
		total llm.Usage
	)
	for range 5 {
		ch, err := p.Stream(context.Background(), req)
		if err != nil {
			t.Fatalf("Stream: %v", err)
		}
		var stop llm.Event
		for ev := range ch {
			all = append(all, ev)
			switch ev.Type {
			case llm.EventUsage:
				total.Add(ev.Usage)
			case llm.EventMessageStop:
				stop = ev
			}
		}
		if stop.StopReason != llm.StopToolUse {
			return all, total
		}
		req.Messages = append(req.Messages, stop.Message)
		var results []llm.Block
		for _, tu := range stop.Message.ToolUses() {
			results = append(results, llm.ToolResultBlock(tu.ID, `{"rolls":42}`, false))
		}
		req.Messages = append(req.Messages, llm.Message{Role: llm.RoleUser, Content: results})
	}
	t.Fatal("tool loop did not end")
	return nil, llm.Usage{}
}

func scenario() *fake.Provider {
	return fake.New(
		fake.ToolCall("toolu_1", "get_stock", map[string]any{"sku": "OLX-75"}, llm.Usage{InputTokens: 100, OutputTokens: 20, CacheWriteTokens: 800}),
		fake.Text("Stokta 42 rulo var.", llm.Usage{InputTokens: 130, OutputTokens: 12, CacheReadTokens: 800}),
	)
}

func TestFakeTwoTurnToolScenarioIsDeterministic(t *testing.T) {
	p1 := scenario()
	events1, usage1 := runToolLoop(t, p1)
	events2, usage2 := runToolLoop(t, scenario())
	if !reflect.DeepEqual(events1, events2) || usage1 != usage2 {
		t.Fatal("same script produced different streams")
	}

	var types []string
	for _, ev := range events1 {
		types = append(types, ev.Type.String())
	}
	want := []string{
		"tool_use", "usage", "message_stop",
		"text_delta", "text_delta", "text_delta", "text_delta", "usage", "message_stop",
	}
	if !reflect.DeepEqual(types, want) {
		t.Fatalf("events = %v, want %v", types, want)
	}
	if string(events1[0].Block.Input) != `{"sku":"OLX-75"}` || events1[2].StopReason != llm.StopToolUse {
		t.Fatalf("turn 1 = %+v / %+v", events1[0], events1[2])
	}
	final := events1[len(events1)-1]
	if final.StopReason != llm.StopEndTurn || final.Message.Text() != "Stokta 42 rulo var." || final.Model != "claude-sonnet-5-5" {
		t.Fatalf("final = %+v", final)
	}
	if want := (llm.Usage{InputTokens: 230, OutputTokens: 32, CacheReadTokens: 800, CacheWriteTokens: 800}); usage1 != want {
		t.Fatalf("usage = %+v, want %+v", usage1, want)
	}

	reqs := p1.Requests()
	if len(reqs) != 2 || p1.Remaining() != 0 {
		t.Fatalf("requests = %d remaining = %d", len(reqs), p1.Remaining())
	}
	second := reqs[1].Messages
	if len(second) != 3 || second[2].Content[0].Type != llm.BlockToolResult || second[2].Content[0].ToolUseID != "toolu_1" {
		t.Fatalf("second request messages = %+v", second)
	}
	if _, err := p1.Stream(context.Background(), llm.Request{}); !errors.Is(err, fake.ErrScriptExhausted) {
		t.Fatalf("exhausted err = %v", err)
	}
}

func TestFakeErrors(t *testing.T) {
	boom := errors.New("boom")
	p := fake.New(
		fake.Turn{Err: llm.ErrUnavailable},
		fake.Turn{Content: []llm.Block{llm.TextBlock("yarım")}, StreamErr: boom},
	)
	if _, err := p.Stream(context.Background(), llm.Request{}); !errors.Is(err, llm.ErrUnavailable) {
		t.Fatalf("err = %v", err)
	}
	if _, err := llm.Complete(context.Background(), p, llm.Request{}); !errors.Is(err, boom) {
		t.Fatalf("stream err = %v", err)
	}
}
