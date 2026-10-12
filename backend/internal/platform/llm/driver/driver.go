// Package driver picks the llm.Provider from env config (TEC-636):
// AI_PROVIDER=anthropic (default, ANTHROPIC_*) or openai (any
// OpenAI-compatible Chat Completions API, OPENAI_*). Every place that needs
// a provider (API server, worker, WhatsApp AI, warranty triage) builds it
// here so they always agree.
package driver

import (
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/config"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/llm"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/llm/anthropic"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/llm/openai"
)

// NewFromConfig returns the configured provider, or llm.Disabled when its
// credentials are missing.
func NewFromConfig(c config.AIConfig) llm.Provider {
	if c.Provider == config.AIProviderOpenAI {
		return openai.NewFromConfig(c)
	}
	return anthropic.NewFromConfig(c)
}
