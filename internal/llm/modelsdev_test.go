package llm

import (
	"context"
	"errors"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/termia/termia/internal/config"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}

func TestListModelsUsesFreshModelsDevCache(t *testing.T) {
	cachePath := filepath.Join(t.TempDir(), "models.dev-api.json")
	if err := os.WriteFile(cachePath, []byte(`{
		"openai": {
			"id": "openai",
			"name": "OpenAI",
			"api": "https://api.openai.com/v1",
			"models": {
				"gpt-5": {
					"id": "gpt-5",
					"name": "GPT-5",
					"reasoning": true,
					"tool_call": true,
					"temperature": false
				}
			}
		}
	}`), 0o644); err != nil {
		t.Fatalf("write cache: %v", err)
	}
	modTime := time.Date(2026, 4, 2, 0, 0, 0, 0, time.UTC)
	if err := os.Chtimes(cachePath, modTime, modTime); err != nil {
		t.Fatalf("chtimes cache: %v", err)
	}

	oldCachePath := modelsDevCachePath
	oldNow := modelsDevNow
	oldClient := modelsDevHTTPClient
	t.Cleanup(func() {
		modelsDevCachePath = oldCachePath
		modelsDevNow = oldNow
		modelsDevHTTPClient = oldClient
	})

	modelsDevCachePath = func() string { return cachePath }
	modelsDevNow = func() time.Time { return modTime.Add(2 * time.Hour) }
	calls := 0
	modelsDevHTTPClient = &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
		calls++
		return nil, errors.New("network should not be used for a fresh cache")
	})}

	models, err := ListModels(context.Background(), config.ProviderMeta{
		ID:          config.ProviderOpenAI,
		Kind:        config.ProviderOpenAI,
		DisplayName: "OpenAI",
		Config:      config.LLMProviderConfig{APIKey: "sk-openai"},
	})
	if err != nil {
		t.Fatalf("list models: %v", err)
	}
	if calls != 0 {
		t.Fatalf("expected no network calls, got %d", calls)
	}
	if len(models) != 1 || models[0].ID != "gpt-5" {
		t.Fatalf("expected cached openai model, got %#v", models)
	}
}
func TestListModelsDoesNotRequireAPIKeyForModelsDevCatalog(t *testing.T) {
	cachePath := filepath.Join(t.TempDir(), "models.dev-api.json")
	if err := os.WriteFile(cachePath, []byte(`{
		"anthropic": {
			"id": "anthropic",
			"name": "Anthropic",
			"api": "https://api.anthropic.com",
			"models": {
				"claude-3-7-sonnet-latest": {
					"id": "claude-3-7-sonnet-latest",
					"name": "Claude 3.7 Sonnet",
					"reasoning": true,
					"tool_call": true,
					"temperature": true
				}
			}
		}
	}`), 0o644); err != nil {
		t.Fatalf("write cache: %v", err)
	}
	modTime := time.Date(2026, 4, 2, 0, 0, 0, 0, time.UTC)
	if err := os.Chtimes(cachePath, modTime, modTime); err != nil {
		t.Fatalf("chtimes cache: %v", err)
	}

	oldCachePath := modelsDevCachePath
	oldNow := modelsDevNow
	oldClient := modelsDevHTTPClient
	t.Cleanup(func() {
		modelsDevCachePath = oldCachePath
		modelsDevNow = oldNow
		modelsDevHTTPClient = oldClient
	})

	modelsDevCachePath = func() string { return cachePath }
	modelsDevNow = func() time.Time { return modTime.Add(2 * time.Hour) }
	modelsDevHTTPClient = &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
		return nil, errors.New("network should not be used for a fresh cache")
	})}

	models, err := ListModels(context.Background(), config.ProviderMeta{
		ID:          config.ProviderAnthropic,
		Kind:        config.ProviderAnthropic,
		DisplayName: "Anthropic",
	})
	if err != nil {
		t.Fatalf("list models without api key: %v", err)
	}
	if len(models) != 1 || models[0].ID != "claude-3-7-sonnet-latest" {
		t.Fatalf("expected anthropic model without api key, got %#v", models)
	}
}

func TestListModelsMatchesOpenAICompatibleProviderByBaseURL(t *testing.T) {
	cachePath := filepath.Join(t.TempDir(), "models.dev-api.json")
	oldCachePath := modelsDevCachePath
	oldNow := modelsDevNow
	oldClient := modelsDevHTTPClient
	t.Cleanup(func() {
		modelsDevCachePath = oldCachePath
		modelsDevNow = oldNow
		modelsDevHTTPClient = oldClient
	})

	modelsDevCachePath = func() string { return cachePath }
	modelsDevNow = func() time.Time { return time.Date(2026, 4, 2, 0, 0, 0, 0, time.UTC) }
	modelsDevHTTPClient = &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
		body := `{
			"openrouter": {
				"id": "openrouter",
				"name": "OpenRouter",
				"api": "https://openrouter.ai/api/v1",
				"models": {
					"openai/gpt-5": {
						"id": "openai/gpt-5",
						"name": "GPT-5 via OpenRouter",
						"reasoning": true,
						"tool_call": true,
						"temperature": false
					}
				}
			}
		}`
		return &http.Response{
			StatusCode: http.StatusOK,
			Body:       io.NopCloser(strings.NewReader(body)),
			Header:     make(http.Header),
		}, nil
	})}

	models, err := ListModels(context.Background(), config.ProviderMeta{
		ID:          "custom:openrouter",
		Kind:        config.ProviderOpenAICompatible,
		DisplayName: "My Router",
		Config: config.LLMProviderConfig{
			APIKey:  "sk-router",
			BaseURL: "https://openrouter.ai/api/v1",
		},
	})
	if err != nil {
		t.Fatalf("list models: %v", err)
	}
	if len(models) != 1 || models[0].ID != "openai/gpt-5" {
		t.Fatalf("expected openrouter model, got %#v", models)
	}
	if models[0].Provider != "openrouter" {
		t.Fatalf("expected matched provider openrouter, got %#v", models[0])
	}
}
func TestListModelsMapsOllamaToModelsDevCatalog(t *testing.T) {
	cachePath := filepath.Join(t.TempDir(), "models.dev-api.json")
	if err := os.WriteFile(cachePath, []byte(`{
		"ollama-cloud": {
			"id": "ollama-cloud",
			"name": "Ollama Cloud",
			"api": "https://ollama.com/api",
			"models": {
				"llama3.1": {
					"id": "llama3.1",
					"name": "Llama 3.1",
					"reasoning": false,
					"tool_call": true,
					"temperature": true
				}
			}
		}
	}`), 0o644); err != nil {
		t.Fatalf("write cache: %v", err)
	}
	modTime := time.Date(2026, 4, 2, 0, 0, 0, 0, time.UTC)
	if err := os.Chtimes(cachePath, modTime, modTime); err != nil {
		t.Fatalf("chtimes cache: %v", err)
	}

	oldCachePath := modelsDevCachePath
	oldNow := modelsDevNow
	oldClient := modelsDevHTTPClient
	t.Cleanup(func() {
		modelsDevCachePath = oldCachePath
		modelsDevNow = oldNow
		modelsDevHTTPClient = oldClient
	})

	modelsDevCachePath = func() string { return cachePath }
	modelsDevNow = func() time.Time { return modTime.Add(2 * time.Hour) }
	modelsDevHTTPClient = &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
		return nil, errors.New("network should not be used for a fresh cache")
	})}

	models, err := ListModels(context.Background(), config.ProviderMeta{
		ID:          config.ProviderOllama,
		Kind:        config.ProviderOllama,
		DisplayName: "Ollama",
	})
	if err != nil {
		t.Fatalf("list models for ollama: %v", err)
	}
	if len(models) != 1 || models[0].ID != "llama3.1" {
		t.Fatalf("expected ollama-cloud mapped model, got %#v", models)
	}
	if models[0].Provider != "ollama-cloud" {
		t.Fatalf("expected provider ollama-cloud, got %#v", models[0])
	}
}
func TestListModelsKeepsCodexForNativeOpenAI(t *testing.T) {
	cachePath := filepath.Join(t.TempDir(), "models.dev-api.json")
	if err := os.WriteFile(cachePath, []byte(`{
		"openai": {
			"id": "openai",
			"name": "OpenAI",
			"api": "https://api.openai.com/v1",
			"models": {
				"gpt-5.1": {
					"id": "gpt-5.1",
					"name": "GPT-5.1",
					"reasoning": true,
					"tool_call": true,
					"temperature": false
				},
				"gpt-5.1-codex": {
					"id": "gpt-5.1-codex",
					"name": "GPT-5.1 Codex",
					"reasoning": true,
					"tool_call": true,
					"temperature": false
				}
			}
		}
	}`), 0o644); err != nil {
		t.Fatalf("write cache: %v", err)
	}
	modTime := time.Date(2026, 4, 2, 0, 0, 0, 0, time.UTC)
	if err := os.Chtimes(cachePath, modTime, modTime); err != nil {
		t.Fatalf("chtimes cache: %v", err)
	}

	oldCachePath := modelsDevCachePath
	oldNow := modelsDevNow
	oldClient := modelsDevHTTPClient
	t.Cleanup(func() {
		modelsDevCachePath = oldCachePath
		modelsDevNow = oldNow
		modelsDevHTTPClient = oldClient
	})

	modelsDevCachePath = func() string { return cachePath }
	modelsDevNow = func() time.Time { return modTime.Add(2 * time.Hour) }
	modelsDevHTTPClient = &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
		return nil, errors.New("network should not be used for a fresh cache")
	})}

	models, err := ListModels(context.Background(), config.ProviderMeta{
		ID:          config.ProviderOpenAI,
		Kind:        config.ProviderOpenAI,
		DisplayName: "OpenAI",
		Config:      config.LLMProviderConfig{APIKey: "sk-openai"},
	})
	if err != nil {
		t.Fatalf("list models: %v", err)
	}
	if len(models) != 2 || models[0].ID != "gpt-5.1" || models[1].ID != "gpt-5.1-codex" {
		t.Fatalf("expected codex to remain available for native openai, got %#v", models)
	}
}

func TestStartModelsCatalogRefreshSkipsFreshCacheYoungerThan48Hours(t *testing.T) {
	cachePath := filepath.Join(t.TempDir(), "models.dev-api.json")
	if err := os.WriteFile(cachePath, []byte(`{"openai":{"id":"openai","name":"OpenAI","models":{}}}`), 0o644); err != nil {
		t.Fatalf("write cache: %v", err)
	}
	modTime := time.Date(2026, 4, 1, 0, 0, 0, 0, time.UTC)
	if err := os.Chtimes(cachePath, modTime, modTime); err != nil {
		t.Fatalf("chtimes cache: %v", err)
	}

	oldCachePath := modelsDevCachePath
	oldNow := modelsDevNow
	oldClient := modelsDevHTTPClient
	t.Cleanup(func() {
		modelsDevCachePath = oldCachePath
		modelsDevNow = oldNow
		modelsDevHTTPClient = oldClient
	})

	modelsDevCachePath = func() string { return cachePath }
	modelsDevNow = func() time.Time { return modTime.Add(47 * time.Hour) }
	calls := 0
	modelsDevHTTPClient = &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
		calls++
		return nil, errors.New("network should not be used for a fresh cache")
	})}

	StartModelsCatalogRefresh()

	if calls != 0 {
		t.Fatalf("expected no refresh request for cache younger than 48h, got %d", calls)
	}
}

func TestStartModelsCatalogRefreshRefreshesCacheOlderThan48Hours(t *testing.T) {
	cachePath := filepath.Join(t.TempDir(), "models.dev-api.json")
	if err := os.WriteFile(cachePath, []byte(`{"openai":{"id":"openai","name":"OpenAI","models":{}}}`), 0o644); err != nil {
		t.Fatalf("write cache: %v", err)
	}
	modTime := time.Date(2026, 4, 1, 0, 0, 0, 0, time.UTC)
	if err := os.Chtimes(cachePath, modTime, modTime); err != nil {
		t.Fatalf("chtimes cache: %v", err)
	}

	oldCachePath := modelsDevCachePath
	oldNow := modelsDevNow
	oldClient := modelsDevHTTPClient
	t.Cleanup(func() {
		modelsDevCachePath = oldCachePath
		modelsDevNow = oldNow
		modelsDevHTTPClient = oldClient
	})

	modelsDevCachePath = func() string { return cachePath }
	modelsDevNow = func() time.Time { return modTime.Add(49 * time.Hour) }
	var mu sync.Mutex
	calls := 0
	modelsDevHTTPClient = &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
		mu.Lock()
		calls++
		mu.Unlock()
		return &http.Response{
			StatusCode: http.StatusOK,
			Body: io.NopCloser(strings.NewReader(`{
				"openai": {
					"id": "openai",
					"name": "OpenAI",
					"api": "https://api.openai.com/v1",
					"models": {
						"gpt-5": {
							"id": "gpt-5",
							"name": "GPT-5",
							"reasoning": true,
							"tool_call": true,
							"temperature": false
						}
					}
				}
			}`)),
			Header: make(http.Header),
		}, nil
	})}

	StartModelsCatalogRefresh()

	deadline := time.Now().Add(2 * time.Second)
	for {
		mu.Lock()
		gotCalls := calls
		mu.Unlock()
		if gotCalls > 0 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("expected background refresh request")
		}
		time.Sleep(10 * time.Millisecond)
	}

	data, err := os.ReadFile(cachePath)
	if err != nil {
		t.Fatalf("read cache: %v", err)
	}
	if !strings.Contains(string(data), `"gpt-5"`) {
		t.Fatalf("expected refreshed cache to be written, got %s", string(data))
	}
}
