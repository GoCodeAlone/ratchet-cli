package providerauth

import (
	"context"

	wfprovider "github.com/GoCodeAlone/workflow-plugin-agent/provider"
)

// ModelInfo describes an available model from a provider.
type ModelInfo = wfprovider.ModelInfo

// ListModels fetches available models from the provider implementation package.
func ListModels(ctx context.Context, providerType, apiKey, baseURL string) ([]ModelInfo, error) {
	return wfprovider.ListModels(ctx, providerType, apiKey, baseURL)
}

// ListModelsWithSettings fetches available models with provider-specific discovery settings.
func ListModelsWithSettings(ctx context.Context, providerType, apiKey, baseURL string, settings map[string]string) ([]ModelInfo, error) {
	return wfprovider.ListModelsWithSettings(ctx, providerType, apiKey, baseURL, settings)
}
