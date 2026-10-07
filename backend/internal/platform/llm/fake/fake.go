// Package fake is a scripted llm.Provider for tests: every Stream call plays
// the next Turn deterministically (text split into word deltas, tool_use
// blocks, usage, message_stop) and records the request.
package fake

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"sync"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/llm"
)

// ErrScriptExhausted is returned when Stream is called after the last turn.
var ErrScriptExhausted = errors.New("fake llm: script exhausted")

// Turn is one scripted model turn. Content is played in order: text blocks
// as word deltas, tool_use blocks as EventToolUse.
type Turn struct {
	Content []llm.Block
	// StopReason defaults to tool_use when Content has a tool call, else
	// end_turn.
	StopReason string
	Usage      llm.Usage
	// Err is returned by Stream itself (e.g. llm.ErrUnavailable).
	Err error
	// StreamErr is sent as EventError after Content instead of
	// usage/message_stop.
	StreamErr error
}

// Text builds a final text turn.
func Text(text string, usage llm.Usage) Turn {
	return Turn{Content: []llm.Block{llm.TextBlock(text)}, Usage: usage}
}

// ToolCall builds a turn calling one tool; input is marshalled to JSON.
func ToolCall(id, name string, input any, usage llm.Usage) Turn {
	raw, err := json.Marshal(input)
	if err != nil {
		panic(err)
	}
	return Turn{
		Content: []llm.Block{{Type: llm.BlockToolUse, ID: id, Name: name, Input: raw}},
		Usage:   usage,
	}
}

// Provider plays Turns in order.
type Provider struct {
	mu       sync.Mutex
	turns    []Turn
	requests []llm.Request
	// Model is reported on message_stop when the request has none.
	Model string
}

// New returns a provider scripted with turns.
func New(turns ...Turn) *Provider { return &Provider{turns: turns, Model: "fake-model"} }

// Push appends turns to the script.
func (p *Provider) Push(turns ...Turn) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.turns = append(p.turns, turns...)
}

// Requests returns copies of the recorded requests.
func (p *Provider) Requests() []llm.Request {
	p.mu.Lock()
	defer p.mu.Unlock()
	return append([]llm.Request(nil), p.requests...)
}

// Remaining is the number of unplayed turns.
func (p *Provider) Remaining() int {
	p.mu.Lock()
	defer p.mu.Unlock()
	return len(p.turns)
}

// Name implements llm.Provider.
func (*Provider) Name() string { return "fake" }

// Enabled implements llm.Provider.
func (*Provider) Enabled() bool { return true }

// Stream implements llm.Provider.
func (p *Provider) Stream(ctx context.Context, req llm.Request) (<-chan llm.Event, error) {
	p.mu.Lock()
	p.requests = append(p.requests, cloneRequest(req))
	if len(p.turns) == 0 {
		p.mu.Unlock()
		return nil, ErrScriptExhausted
	}
	turn := p.turns[0]
	p.turns = p.turns[1:]
	p.mu.Unlock()
	if turn.Err != nil {
		return nil, turn.Err
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	model := req.Model
	if model == "" {
		model = p.Model
	}
	events := script(turn, model)
	ch := make(chan llm.Event, len(events))
	for _, ev := range events {
		ch <- ev
	}
	close(ch)
	return ch, nil
}

func script(turn Turn, model string) []llm.Event {
	var (
		events []llm.Event
		msg    = llm.Message{Role: llm.RoleAssistant}
		tools  bool
	)
	for _, b := range turn.Content {
		switch b.Type {
		case llm.BlockText:
			for _, part := range strings.SplitAfter(b.Text, " ") {
				if part != "" {
					events = append(events, llm.Event{Type: llm.EventTextDelta, Text: part})
				}
			}
		case llm.BlockToolUse:
			tools = true
			if len(b.Input) == 0 {
				b.Input = json.RawMessage(`{}`)
			}
			events = append(events, llm.Event{Type: llm.EventToolUse, Block: b})
		}
		msg.Content = append(msg.Content, b)
	}
	if turn.StreamErr != nil {
		return append(events, llm.Event{Type: llm.EventError, Err: turn.StreamErr})
	}
	stop := turn.StopReason
	if stop == "" {
		stop = llm.StopEndTurn
		if tools {
			stop = llm.StopToolUse
		}
	}
	return append(events,
		llm.Event{Type: llm.EventUsage, Usage: turn.Usage},
		llm.Event{Type: llm.EventMessageStop, StopReason: stop, Model: model, Message: msg},
	)
}

func cloneRequest(r llm.Request) llm.Request {
	out := r
	out.System = append([]llm.SystemBlock(nil), r.System...)
	out.Messages = append([]llm.Message(nil), r.Messages...)
	out.Tools = append([]llm.ToolDef(nil), r.Tools...)
	return out
}
