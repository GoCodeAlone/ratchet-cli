package providerauth

import (
	"slices"
	"strings"
	"testing"

	ratchetagent "github.com/GoCodeAlone/workflow-plugin-agent/orchestrator"
)

func TestCatalogCoversRuntimeProviderTypes(t *testing.T) {
	runtimeTypes := ratchetagent.NewProviderRegistry(nil, nil).ProviderTypes()
	if err := ValidateCatalog(runtimeTypes); err != nil {
		t.Fatalf("ValidateCatalog: %v", err)
	}
	for _, providerType := range runtimeTypes {
		if providerType == "mock" || providerType == "test" {
			continue
		}
		if _, ok := LookupSetup(providerType); !ok {
			t.Errorf("runtime provider %q has no setup entry", providerType)
		}
	}

	entries := Catalog()
	if got, want := len(entries), 22; got != want {
		t.Fatalf("Catalog() entries = %d, want %d", got, want)
	}
	visibleTypes := make([]string, 0, len(entries))
	for _, entry := range entries {
		visibleTypes = append(visibleTypes, entry.Type)
	}
	for _, providerType := range []string{"bedrock", "openai_azure", "anthropic_vertex", "openai_chatgpt", "cursor_cli"} {
		if !slices.Contains(visibleTypes, providerType) {
			t.Errorf("Catalog() missing visible type %q", providerType)
		}
	}
}

func TestCatalogBedrockAliasResolvesCanonicalEntry(t *testing.T) {
	canonical, ok := LookupSetup("bedrock")
	if !ok {
		t.Fatal("LookupSetup(bedrock) not found")
	}
	for _, alias := range []string{"anthropic_bedrock", "anthropic-bedrock", "aws-bedrock"} {
		entry, found := LookupSetup(alias)
		if !found {
			t.Errorf("LookupSetup(%q) not found", alias)
			continue
		}
		if entry.Type != canonical.Type || entry.Type != "bedrock" {
			t.Errorf("LookupSetup(%q).Type = %q, want bedrock", alias, entry.Type)
		}
	}
}

func TestCatalogReturnsDefensiveCopies(t *testing.T) {
	first := Catalog()
	if len(first) == 0 || len(first[0].Aliases) == 0 {
		t.Fatal("catalog fixture requires an entry with an alias")
	}
	originalType := first[0].Type
	originalAlias := first[0].Aliases[0]
	first[0].Type = "mutated"
	first[0].Aliases[0] = "mutated"
	customIndex := slices.IndexFunc(first, func(entry SetupEntry) bool { return entry.Type == "custom" })
	if customIndex < 0 || len(first[customIndex].Settings) == 0 || len(first[customIndex].Settings[0].Choices) == 0 {
		t.Fatal("catalog fixture requires custom compatibility choices")
	}
	originalChoice := first[customIndex].Settings[0].Choices[0]
	first[customIndex].Settings[0].Choices[0] = "mutated"

	second := Catalog()
	if second[0].Type != originalType || second[0].Aliases[0] != originalAlias {
		t.Fatalf("Catalog() returned shared state: %#v", second[0])
	}
	if second[customIndex].Settings[0].Choices[0] != originalChoice {
		t.Fatal("Catalog() returned shared setting choice state")
	}
	lookup, ok := LookupSetup(originalAlias)
	if !ok || lookup.Type != originalType {
		t.Fatalf("LookupSetup(%q) = %#v, %v", originalAlias, lookup, ok)
	}
	lookup.Aliases[0] = "mutated-again"
	if next, _ := LookupSetup(originalAlias); next.Aliases[0] != originalAlias {
		t.Fatal("LookupSetup returned shared alias state")
	}
}

func TestCatalogDeclaresNonSecretSettingsAndModelFallbacks(t *testing.T) {
	for _, entry := range Catalog() {
		for _, field := range entry.Settings {
			switch strings.ToLower(field.Key) {
			case "api_key", "secret", "secret_access_key", "session_token", "entra_token", "credentials_json", "password", "token":
				t.Errorf("provider %q exposes secret setting %q", entry.Type, field.Key)
			}
		}
		if entry.Model == ModelExternal && entry.AllowManualModel {
			t.Errorf("external provider %q must leave model ownership to its CLI", entry.Type)
		}
		if entry.Model != ModelExternal && !entry.AllowManualModel {
			t.Errorf("provider %q must declare manual-model recovery", entry.Type)
		}
		if entry.BaseURLRequired && !entry.PromptBaseURL {
			t.Errorf("provider %q requires base URL without prompting for it", entry.Type)
		}
	}

	for _, providerType := range []string{"custom", "openai_compatible", "anthropic_compatible"} {
		entry, _ := LookupSetup(providerType)
		if !entry.BaseURLRequired {
			t.Errorf("provider %q BaseURLRequired = false", providerType)
		}
	}
}

func TestValidateCatalogRejectsInvalidEntriesAndRuntimeGaps(t *testing.T) {
	base := Catalog()
	tests := []struct {
		name    string
		mutate  func([]SetupEntry) []SetupEntry
		wantErr string
	}{
		{
			name: "duplicate type",
			mutate: func(entries []SetupEntry) []SetupEntry {
				return append(entries, entries[0])
			},
			wantErr: "duplicate provider type",
		},
		{
			name: "alias collision",
			mutate: func(entries []SetupEntry) []SetupEntry {
				entries[1].Aliases = append(entries[1].Aliases, entries[0].Type)
				return entries
			},
			wantErr: "duplicate provider name",
		},
		{
			name: "unknown auth strategy",
			mutate: func(entries []SetupEntry) []SetupEntry {
				entries[0].Auth = AuthStrategy("unknown")
				return entries
			},
			wantErr: "unknown auth strategy",
		},
		{
			name: "secret setting",
			mutate: func(entries []SetupEntry) []SetupEntry {
				entries[0].Settings = append(entries[0].Settings, SettingField{Key: "client_secret", Label: "Client secret"})
				return entries
			},
			wantErr: "secret setting",
		},
		{
			name: "missing guide metadata",
			mutate: func(entries []SetupEntry) []SetupEntry {
				entries[0].SetupCommand = ""
				return entries
			},
			wantErr: "setup guide metadata",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			entries := cloneSetupEntries(base)
			err := validateCatalog(tt.mutate(entries), nil)
			if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("validateCatalog() error = %v, want substring %q", err, tt.wantErr)
			}
		})
	}

	runtimeTypes := ratchetagent.NewProviderRegistry(nil, nil).ProviderTypes()
	runtimeTypes = append(runtimeTypes, "future_provider")
	if err := validateCatalog(base, runtimeTypes); err == nil || !strings.Contains(err.Error(), "future_provider") {
		t.Fatalf("validateCatalog(runtime gap) error = %v", err)
	}
}
