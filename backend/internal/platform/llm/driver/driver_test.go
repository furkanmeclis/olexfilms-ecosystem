package driver

import (
	"testing"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/config"
)

func TestNewFromConfigPicksProvider(t *testing.T) {
	cases := []struct {
		name string
		cfg  config.AIConfig
		want string
	}{
		{"default anthropic without key", config.AIConfig{}, "disabled"},
		{"anthropic with key", config.AIConfig{Provider: config.AIProviderAnthropic, APIKey: "k"}, "anthropic"},
		{"openai without key or base", config.AIConfig{Provider: config.AIProviderOpenAI, APIKey: "anthropic-key"}, "disabled"},
		{"openai with key", config.AIConfig{Provider: config.AIProviderOpenAI, OpenAIAPIKey: "k"}, "openai"},
		{"openai local server", config.AIConfig{Provider: config.AIProviderOpenAI, OpenAIBaseURL: "http://localhost:11434/v1"}, "openai"},
	}
	for _, tc := range cases {
		if got := NewFromConfig(tc.cfg).Name(); got != tc.want {
			t.Errorf("%s: provider = %s, want %s", tc.name, got, tc.want)
		}
	}
}

func TestDefaultModelsPerProvider(t *testing.T) {
	if c, f := config.DefaultAIModels(config.AIProviderOpenAI); c != "gpt-4.1" || f != "gpt-4.1-mini" {
		t.Fatalf("openai defaults = %s, %s", c, f)
	}
	if c, _ := config.DefaultAIModels(config.AIProviderAnthropic); c != "claude-sonnet-5-5" {
		t.Fatalf("anthropic default = %s", c)
	}
}
