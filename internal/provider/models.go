package providerauth

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strings"
)

// ModelInfo describes an available model from a provider.
type ModelInfo struct {
	ID   string
	Name string
}

// ListModels fetches available models from the given provider type.
func ListModels(ctx context.Context, providerType, apiKey, baseURL string) ([]ModelInfo, error) {
	switch providerType {
	case "anthropic":
		return listAnthropicModels(ctx, apiKey, baseURL)
	case "openai":
		return listOpenAIModels(ctx, apiKey, baseURL)
	case "copilot":
		return listCopilotModels(ctx, apiKey)
	case "ollama":
		return listOllamaModels(ctx, baseURL)
	case "gemini":
		return listGeminiModels(ctx, apiKey)
	default:
		return nil, fmt.Errorf("unsupported provider type: %s", providerType)
	}
}

func listAnthropicModels(ctx context.Context, apiKey, baseURL string) ([]ModelInfo, error) {
	if baseURL == "" {
		baseURL = "https://api.anthropic.com"
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, baseURL+"/v1/models", nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("x-api-key", apiKey)
	req.Header.Set("anthropic-version", "2023-06-01")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("API error (%d): %s", resp.StatusCode, truncateStr(body, 200))
	}

	var result struct {
		Data []struct {
			ID          string `json:"id"`
			DisplayName string `json:"display_name"`
			Type        string `json:"type"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, err
	}

	var models []ModelInfo
	for _, m := range result.Data {
		if m.Type != "" && m.Type != "model" {
			continue
		}
		name := m.DisplayName
		if name == "" {
			name = m.ID
		}
		models = append(models, ModelInfo{ID: m.ID, Name: name})
	}
	sortModels(models)
	return models, nil
}

func listOpenAIModels(ctx context.Context, apiKey, baseURL string) ([]ModelInfo, error) {
	if baseURL == "" {
		baseURL = "https://api.openai.com"
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, baseURL+"/v1/models", nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+apiKey)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("API error (%d): %s", resp.StatusCode, truncateStr(body, 200))
	}

	var result struct {
		Data []struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, err
	}

	chatPrefixes := []string{"gpt-4", "gpt-3.5", "o1", "o3", "o4", "chatgpt"}
	var models []ModelInfo
	for _, m := range result.Data {
		lower := strings.ToLower(m.ID)
		isChat := false
		for _, prefix := range chatPrefixes {
			if strings.HasPrefix(lower, prefix) {
				isChat = true
				break
			}
		}
		if !isChat {
			continue
		}
		models = append(models, ModelInfo{ID: m.ID, Name: m.ID})
	}
	sortModels(models)
	return models, nil
}

func listCopilotModels(ctx context.Context, apiKey string) ([]ModelInfo, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "https://api.githubcopilot.com/models", nil)
	if err != nil {
		return copilotFallbackModels(), nil
	}
	req.Header.Set("Authorization", "Bearer "+apiKey)
	req.Header.Set("Copilot-Integration-Id", "ratchet")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return copilotFallbackModels(), nil
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return copilotFallbackModels(), nil
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return copilotFallbackModels(), nil
	}

	var result struct {
		Data []struct {
			ID   string `json:"id"`
			Name string `json:"name"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &result); err != nil {
		return copilotFallbackModels(), nil
	}

	var models []ModelInfo
	for _, m := range result.Data {
		name := m.Name
		if name == "" {
			name = m.ID
		}
		models = append(models, ModelInfo{ID: m.ID, Name: name})
	}
	if len(models) == 0 {
		return copilotFallbackModels(), nil
	}
	sortModels(models)
	return models, nil
}

func copilotFallbackModels() []ModelInfo {
	return []ModelInfo{
		{ID: "claude-sonnet-4", Name: "Claude Sonnet 4"},
		{ID: "gpt-4.1", Name: "GPT-4.1"},
		{ID: "gpt-4o", Name: "GPT-4o"},
		{ID: "gpt-4o-mini", Name: "GPT-4o Mini"},
		{ID: "o3-mini", Name: "o3-mini"},
	}
}

func listOllamaModels(ctx context.Context, baseURL string) ([]ModelInfo, error) {
	if baseURL == "" {
		baseURL = "http://localhost:11434"
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, baseURL+"/api/tags", nil)
	if err != nil {
		return nil, err
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("cannot reach Ollama at %s: %w", baseURL, err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("Ollama API error (%d): %s", resp.StatusCode, truncateStr(body, 200))
	}

	var result struct {
		Models []struct {
			Name string `json:"name"`
		} `json:"models"`
	}
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, err
	}

	var models []ModelInfo
	for _, m := range result.Models {
		models = append(models, ModelInfo{ID: m.Name, Name: m.Name})
	}
	sortModels(models)
	return models, nil
}

func listGeminiModels(ctx context.Context, apiKey string) ([]ModelInfo, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet,
		"https://generativelanguage.googleapis.com/v1/models?key="+apiKey, nil)
	if err != nil {
		return nil, err
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("API error (%d): %s", resp.StatusCode, truncateStr(body, 200))
	}

	var result struct {
		Models []struct {
			Name        string   `json:"name"`
			DisplayName string   `json:"displayName"`
			Methods     []string `json:"supportedGenerationMethods"`
		} `json:"models"`
	}
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, err
	}

	var models []ModelInfo
	for _, m := range result.Models {
		// Filter to models that support generateContent
		supportsChat := false
		for _, method := range m.Methods {
			if method == "generateContent" {
				supportsChat = true
				break
			}
		}
		if !supportsChat {
			continue
		}
		// Strip "models/" prefix from name
		id := strings.TrimPrefix(m.Name, "models/")
		name := m.DisplayName
		if name == "" {
			name = id
		}
		models = append(models, ModelInfo{ID: id, Name: name})
	}
	sortModels(models)
	return models, nil
}

func sortModels(models []ModelInfo) {
	sort.Slice(models, func(i, j int) bool {
		return models[i].ID < models[j].ID
	})
}

func truncateStr(b []byte, maxLen int) string {
	s := string(b)
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}
