package agent

import (
	"context"
	"fmt"
	"os"
	"strings"

	adkmodel "google.golang.org/adk/model"
	adkgemini "google.golang.org/adk/model/gemini"
	"google.golang.org/genai"
)

func NewModel(ctx context.Context, spec ModelSpec) (adkmodel.LLM, error) {
	provider := strings.ToLower(strings.TrimSpace(spec.Provider))
	switch provider {
	case "openai", "deepseek", "ollama":
		return NewOpenAIModel(spec)
	case "gemini", "google":
		apiKey := strings.TrimSpace(os.Getenv(spec.APIKeyEnv))
		return adkgemini.NewModel(ctx, spec.Model, &genai.ClientConfig{
			APIKey:  apiKey,
			Backend: genai.BackendGeminiAPI,
		})
	default:
		return nil, fmt.Errorf("unsupported provider %q", spec.Provider)
	}
}
