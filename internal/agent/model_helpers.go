package agent

import (
	"os"
	"strings"
	"time"

	"github.com/termia/termia/internal/providerpolicy"
)

const defaultModelTimeout = 2 * time.Minute

func effectiveModelBaseURL(spec ModelSpec) string {
	return strings.TrimSpace(providerpolicy.EffectiveBaseURL(spec.Provider, spec.BaseURL))
}

func resolveAPIKey(spec ModelSpec) string {
	if apiKey := strings.TrimSpace(spec.APIKey); apiKey != "" {
		return apiKey
	}
	if envKey := strings.TrimSpace(spec.APIKeyEnv); envKey != "" {
		return strings.TrimSpace(os.Getenv(envKey))
	}
	return ""
}
