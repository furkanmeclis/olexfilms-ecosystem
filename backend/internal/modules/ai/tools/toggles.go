package tools

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database/db"
	"github.com/jackc/pgx/v5"
)

// SettingsReader reads the platform AI settings row.
type SettingsReader interface {
	GetAISettings(ctx context.Context) (db.AiSetting, error)
}

// SettingsToggles reads ai_settings.tool_toggles ({"tool_name": false}).
type SettingsToggles struct{ Q SettingsReader }

// ToolToggles implements ToggleReader. A missing settings row disables
// nothing.
func (s SettingsToggles) ToolToggles(ctx context.Context) (map[string]bool, error) {
	row, err := s.Q.GetAISettings(ctx)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	out := map[string]bool{}
	if len(row.ToolToggles) == 0 {
		return out, nil
	}
	var raw map[string]any
	if err := json.Unmarshal(row.ToolToggles, &raw); err != nil {
		return nil, err
	}
	for k, v := range raw {
		if b, ok := v.(bool); ok {
			out[k] = b
		}
	}
	return out, nil
}
