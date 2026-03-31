package checkpoint

import "os"

const defaultModel = "claude-haiku-4-5-20251001"

// Config holds summarizer configuration from environment variables.
type Config struct {
	APIKey  string
	Model   string
	Enabled bool
}

// LoadConfig reads summarizer configuration from environment variables.
func LoadConfig() Config {
	apiKey := os.Getenv("ANTHROPIC_API_KEY")
	model := os.Getenv("DOCKET_SUMMARIZER_MODEL")
	if model == "" {
		model = defaultModel
	}

	enabled := apiKey != ""
	if v := os.Getenv("DOCKET_SUMMARIZER_ENABLED"); v == "false" {
		enabled = false
	}

	return Config{
		APIKey:  apiKey,
		Model:   model,
		Enabled: enabled,
	}
}
