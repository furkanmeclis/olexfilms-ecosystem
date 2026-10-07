package usecase

import (
	"encoding/json"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database/db"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/llm"
)

const (
	// keepFullTurns user turns keep their tool results verbatim; older tool
	// results are replaced by a placeholder. The boundary moves in steps of
	// keepFullTurns so the cached prefix only changes every few turns.
	keepFullTurns = 6
	// maxHistoryTurns caps replayed user turns (dropped in steps).
	maxHistoryTurns   = 40
	clearedToolResult = "[older tool result cleared to save context; call the tool again if needed]"
	notExecutedResult = "Not executed: the previous turn ended before this tool call ran."
)

// historyFromRows rebuilds the replayable API messages of stored rows.
func historyFromRows(rows []db.AiMessage) []llm.Message {
	var msgs []llm.Message
	for _, row := range rows {
		var stored []llm.Message
		if len(row.Content) == 0 || json.Unmarshal(row.Content, &stored) != nil {
			continue
		}
		msgs = append(msgs, stored...)
	}
	return msgs
}

func hasUserText(m llm.Message) bool {
	if m.Role != llm.RoleUser {
		return false
	}
	for _, b := range m.Content {
		if b.Type == llm.BlockText && b.Text != "" {
			return true
		}
	}
	return false
}

// orderUserBlocks puts tool_result blocks first (the API requires them to
// lead the user message that follows a tool_use).
func orderUserBlocks(blocks []llm.Block) []llm.Block {
	out := make([]llm.Block, 0, len(blocks))
	for _, b := range blocks {
		if b.Type == llm.BlockToolResult {
			out = append(out, b)
		}
	}
	for _, b := range blocks {
		if b.Type != llm.BlockToolResult {
			out = append(out, b)
		}
	}
	return out
}

// sanitizeHistory makes stored history valid for the API: drops empty and
// leading assistant messages, merges consecutive same-role messages (a
// paused turn's results followed by the confirmation result), answers every
// tool_use that never got a result (interrupted turn, expired card) and
// drops orphan tool_results.
func sanitizeHistory(in []llm.Message) []llm.Message {
	var merged []llm.Message
	for _, m := range in {
		content := make([]llm.Block, 0, len(m.Content))
		for _, b := range m.Content {
			if b.Type == llm.BlockText && b.Text == "" {
				continue
			}
			content = append(content, b)
		}
		if len(content) == 0 {
			continue
		}
		if len(merged) == 0 && m.Role != llm.RoleUser {
			continue
		}
		if n := len(merged); n > 0 && merged[n-1].Role == m.Role {
			merged[n-1].Content = append(append([]llm.Block(nil), merged[n-1].Content...), content...)
			continue
		}
		merged = append(merged, llm.Message{Role: m.Role, Content: content})
	}

	out := make([]llm.Message, 0, len(merged))
	for i := 0; i < len(merged); i++ {
		m := merged[i]
		if m.Role == llm.RoleUser {
			allowed := map[string]bool{}
			var prev []llm.Block
			if len(out) > 0 && out[len(out)-1].Role == llm.RoleAssistant {
				prev = out[len(out)-1].ToolUses()
				for _, tu := range prev {
					allowed[tu.ID] = true
				}
			}
			seen := map[string]bool{}
			blocks := make([]llm.Block, 0, len(m.Content)+len(prev))
			for _, b := range m.Content {
				if b.Type == llm.BlockToolResult {
					if !allowed[b.ToolUseID] || seen[b.ToolUseID] {
						continue
					}
					seen[b.ToolUseID] = true
				}
				blocks = append(blocks, b)
			}
			for _, tu := range prev {
				if !seen[tu.ID] {
					blocks = append(blocks, llm.ToolResultBlock(tu.ID, notExecutedResult, true))
				}
			}
			blocks = orderUserBlocks(blocks)
			if len(blocks) == 0 {
				continue
			}
			out = append(out, llm.Message{Role: llm.RoleUser, Content: blocks})
			continue
		}
		out = append(out, m)
		if tus := m.ToolUses(); len(tus) > 0 && (i+1 >= len(merged) || merged[i+1].Role != llm.RoleUser) {
			blocks := make([]llm.Block, 0, len(tus))
			for _, tu := range tus {
				blocks = append(blocks, llm.ToolResultBlock(tu.ID, notExecutedResult, true))
			}
			out = append(out, llm.Message{Role: llm.RoleUser, Content: blocks})
		}
	}
	return out
}

// trimHistory drops the oldest turns beyond maxHistoryTurns and clears the
// tool result payloads of old turns, both in steps so the prompt cache
// prefix stays stable for several turns.
func trimHistory(msgs []llm.Message) []llm.Message {
	turnStarts := func(ms []llm.Message) []int {
		var idx []int
		for i, m := range ms {
			if hasUserText(m) {
				idx = append(idx, i)
			}
		}
		return idx
	}
	starts := turnStarts(msgs)
	if n := len(starts); n > maxHistoryTurns {
		drop := ((n - maxHistoryTurns + keepFullTurns - 1) / keepFullTurns) * keepFullTurns
		if drop < n {
			msgs = msgs[starts[drop]:]
			starts = turnStarts(msgs)
		}
	}
	if n := len(starts); n > keepFullTurns {
		boundary := ((n - keepFullTurns) / keepFullTurns) * keepFullTurns
		if boundary > 0 {
			limit := starts[boundary]
			out := make([]llm.Message, len(msgs))
			for i, m := range msgs {
				if i >= limit {
					out[i] = m
					continue
				}
				blocks := make([]llm.Block, len(m.Content))
				for j, b := range m.Content {
					if b.Type == llm.BlockToolResult && !b.IsError && len(b.Content) > len(clearedToolResult) {
						b.Content = clearedToolResult
					}
					blocks[j] = b
				}
				out[i] = llm.Message{Role: m.Role, Content: blocks}
			}
			msgs = out
		}
	}
	return msgs
}
