package llm

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/termia/termia/internal/config"
	"github.com/termia/termia/internal/providerpolicy"
)

const (
	modelsDevAPIURL   = "https://models.dev/api.json"
	modelsDevCacheTTL = 24 * time.Hour
)

var (
	modelsDevHTTPClient = http.DefaultClient
	modelsDevNow        = time.Now
	modelsDevCachePath  = func() string {
		return filepath.Join(config.CacheDir(), "models.dev-api.json")
	}
)

type modelsDevCatalog map[string]modelsDevProvider

type modelsDevProvider struct {
	ID     string                    `json:"id"`
	NPM    string                    `json:"npm"`
	API    string                    `json:"api"`
	Name   string                    `json:"name"`
	Models map[string]modelsDevModel `json:"models"`
}

type modelsDevModel struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Reasoning   bool   `json:"reasoning"`
	ToolCall    bool   `json:"tool_call"`
	Temperature bool   `json:"temperature"`
}

func listModelsFromModelsDev(ctx context.Context, meta config.ProviderMeta) ([]ModelDescriptor, error) {
	catalog, err := loadModelsDevCatalog(ctx)
	if err != nil {
		return nil, err
	}

	providerIDs := modelsDevProviderIDsForMeta(meta, catalog)
	if len(providerIDs) == 0 {
		return nil, nil
	}

	models := make([]ModelDescriptor, 0, 64)
	seen := make(map[string]struct{})
	for _, providerID := range providerIDs {
		provider, ok := catalog[providerID]
		if !ok {
			continue
		}
		ids := make([]string, 0, len(provider.Models))
		for modelID := range provider.Models {
			ids = append(ids, modelID)
		}
		sort.Strings(ids)
		for _, modelID := range ids {
			item := provider.Models[modelID]
			if strings.TrimSpace(item.ID) == "" {
				item.ID = modelID
			}
			if shouldSkipModelsDevModel(meta, providerID, item.ID) {
				continue
			}
			label := strings.TrimSpace(item.Name)
			if label == "" {
				label = item.ID
			}
			dedupeKey := strings.ToLower(strings.TrimSpace(item.ID))
			if _, ok := seen[dedupeKey]; ok {
				continue
			}
			seen[dedupeKey] = struct{}{}
			raw, _ := json.Marshal(item)
			models = append(models, newDescriptor(providerID, item.ID, label, string(raw)))
		}
	}

	sortDescriptors(models)
	return models, nil
}

func shouldSkipModelsDevModel(meta config.ProviderMeta, providerID, modelID string) bool {
	providerKind := config.NormalizeProviderName(meta.Kind)
	if providerKind != config.ProviderOpenAI && providerKind != config.ProviderOpenAICompatible {
		return false
	}
	if !IsOpenAIResponsesOnlyModel(modelID) {
		return false
	}
	return !providerUsesOpenAIResponses(meta, providerID)
}

func providerUsesOpenAIResponses(meta config.ProviderMeta, providerID string) bool {
	if providerID != "openai" {
		return false
	}
	return providerpolicy.UsesNativeOpenAIResponses(meta.Kind, meta.Config.BaseURL)
}

func loadModelsDevCatalog(ctx context.Context) (modelsDevCatalog, error) {
	if catalog, err := readModelsDevCatalogCache(true); err == nil {
		return catalog, nil
	}

	catalog, raw, err := fetchModelsDevCatalog(ctx)
	if err == nil {
		_ = writeModelsDevCatalogCache(raw)
		return catalog, nil
	}

	if cached, cacheErr := readModelsDevCatalogCache(false); cacheErr == nil {
		return cached, nil
	}
	return nil, err
}

func readModelsDevCatalogCache(requireFresh bool) (modelsDevCatalog, error) {
	path := modelsDevCachePath()
	info, err := os.Stat(path)
	if err != nil {
		return nil, err
	}
	if requireFresh && modelsDevCacheTTL > 0 && modelsDevNow().Sub(info.ModTime()) > modelsDevCacheTTL {
		return nil, fmt.Errorf("models.dev cache is stale")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	return parseModelsDevCatalog(data)
}

func fetchModelsDevCatalog(ctx context.Context) (modelsDevCatalog, []byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, modelsDevAPIURL, nil)
	if err != nil {
		return nil, nil, err
	}
	req.Header.Set("Accept", "application/json")

	resp, err := modelsDevHTTPClient.Do(req)
	if err != nil {
		return nil, nil, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, nil, err
	}
	if resp.StatusCode >= http.StatusBadRequest {
		message := strings.TrimSpace(string(body))
		if message == "" {
			message = resp.Status
		}
		return nil, nil, fmt.Errorf("%s", message)
	}

	catalog, err := parseModelsDevCatalog(body)
	if err != nil {
		return nil, nil, err
	}
	return catalog, body, nil
}

func writeModelsDevCatalogCache(data []byte) error {
	path := modelsDevCachePath()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o644)
}

func parseModelsDevCatalog(data []byte) (modelsDevCatalog, error) {
	var catalog modelsDevCatalog
	if err := json.Unmarshal(data, &catalog); err != nil {
		return nil, fmt.Errorf("failed to decode models.dev catalog: %w", err)
	}
	for providerID, provider := range catalog {
		if strings.TrimSpace(provider.ID) == "" {
			provider.ID = providerID
		}
		for modelID, model := range provider.Models {
			if strings.TrimSpace(model.ID) == "" {
				model.ID = modelID
			}
			provider.Models[modelID] = model
		}
		catalog[providerID] = provider
	}
	return catalog, nil
}

func modelsDevProviderIDsForMeta(meta config.ProviderMeta, catalog modelsDevCatalog) []string {
	switch config.NormalizeProviderName(meta.Kind) {
	case config.ProviderOpenAI:
		return filterKnownModelsDevProviders(catalog, "openai")
	case config.ProviderAnthropic:
		return filterKnownModelsDevProviders(catalog, "anthropic")
	case config.ProviderDeepSeek:
		return filterKnownModelsDevProviders(catalog, "deepseek")
	case config.ProviderOllama:
		ids := filterKnownModelsDevProviders(catalog, "ollama-cloud")
		if len(ids) > 0 {
			return ids
		}
		return inferModelsDevProviderIDs(meta, catalog)
	case config.ProviderOpenAICompatible:
		return inferModelsDevProviderIDs(meta, catalog)
	default:
		return nil
	}
}

func filterKnownModelsDevProviders(catalog modelsDevCatalog, ids ...string) []string {
	results := make([]string, 0, len(ids))
	for _, id := range ids {
		if _, ok := catalog[id]; ok {
			results = append(results, id)
		}
	}
	return results
}

func inferModelsDevProviderIDs(meta config.ProviderMeta, catalog modelsDevCatalog) []string {
	results := make([]string, 0, 4)
	seen := make(map[string]struct{})
	add := func(ids ...string) {
		for _, id := range ids {
			id = strings.TrimSpace(id)
			if id == "" {
				continue
			}
			if _, ok := catalog[id]; !ok {
				continue
			}
			if _, ok := seen[id]; ok {
				continue
			}
			seen[id] = struct{}{}
			results = append(results, id)
		}
	}

	baseURL := strings.TrimSpace(meta.Config.BaseURL)
	host, path := normalizeModelsDevURLParts(baseURL)
	add(modelsDevProviderHintsForHost(host, path)...)

	lookupText := strings.ToLower(strings.Join([]string{meta.DisplayName, meta.ID, baseURL}, " "))
	add(modelsDevProviderHintsForText(lookupText)...)

	if host != "" {
		for providerID, provider := range catalog {
			apiHost, _ := normalizeModelsDevURLParts(provider.API)
			if apiHost == "" {
				continue
			}
			if host == apiHost || strings.HasSuffix(host, "."+apiHost) || strings.HasSuffix(apiHost, "."+host) {
				add(providerID)
			}
		}
	}

	return results
}

func normalizeModelsDevURLParts(raw string) (string, string) {
	raw = strings.TrimSpace(strings.ToLower(raw))
	if raw == "" {
		return "", ""
	}
	parsed, err := url.Parse(raw)
	if err != nil {
		return "", ""
	}
	host := strings.TrimSpace(parsed.Hostname())
	path := strings.Trim(strings.TrimSpace(parsed.Path), "/")
	return host, path
}

func modelsDevProviderHintsForHost(host, path string) []string {
	switch {
	case host == "":
		return nil
	case strings.Contains(host, "openrouter.ai"):
		return []string{"openrouter"}
	case strings.Contains(host, ".openai.azure.com") || strings.Contains(host, "azure.com") || strings.Contains(path, "openai/deployments"):
		return []string{"azure"}
	case strings.Contains(host, "api.openai.com"):
		return []string{"openai"}
	case strings.Contains(host, "anthropic.com"):
		return []string{"anthropic"}
	case strings.Contains(host, "deepseek.com"):
		return []string{"deepseek"}
	case strings.Contains(host, "groq.com"):
		return []string{"groq"}
	case strings.Contains(host, "x.ai"):
		return []string{"xai"}
	case strings.Contains(host, "githubcopilot.com"):
		return []string{"github-copilot"}
	case strings.Contains(host, "github.ai"):
		return []string{"github-models"}
	case strings.Contains(host, "generativelanguage.googleapis.com"):
		return []string{"google"}
	case strings.Contains(host, "aiplatform.googleapis.com") || strings.Contains(host, "vertexai.googleapis.com"):
		return []string{"google-vertex"}
	case strings.Contains(host, "mistral.ai"):
		return []string{"mistral"}
	case strings.Contains(host, "together.xyz") || strings.Contains(host, "together.ai"):
		return []string{"togetherai"}
	case strings.Contains(host, "fireworks.ai"):
		return []string{"fireworks-ai"}
	case strings.Contains(host, "ollama.com"):
		return []string{"ollama-cloud"}
	default:
		return nil
	}
}

func modelsDevProviderHintsForText(text string) []string {
	results := make([]string, 0, 4)
	appendIf := func(id string, ok bool) {
		if ok {
			results = append(results, id)
		}
	}
	appendIf("openrouter", strings.Contains(text, "openrouter"))
	appendIf("azure", strings.Contains(text, "azure"))
	appendIf("openai", strings.Contains(text, "openai"))
	appendIf("anthropic", strings.Contains(text, "anthropic") || strings.Contains(text, "claude"))
	appendIf("deepseek", strings.Contains(text, "deepseek"))
	appendIf("groq", strings.Contains(text, "groq"))
	appendIf("xai", strings.Contains(text, "xai") || strings.Contains(text, "grok"))
	appendIf("github-copilot", strings.Contains(text, "copilot"))
	appendIf("github-models", strings.Contains(text, "github models"))
	appendIf("google-vertex", strings.Contains(text, "vertex"))
	appendIf("google", strings.Contains(text, "gemini") || strings.Contains(text, "google"))
	appendIf("mistral", strings.Contains(text, "mistral"))
	appendIf("togetherai", strings.Contains(text, "together"))
	appendIf("fireworks-ai", strings.Contains(text, "fireworks"))
	appendIf("ollama-cloud", strings.Contains(text, "ollama cloud"))
	return results
}
