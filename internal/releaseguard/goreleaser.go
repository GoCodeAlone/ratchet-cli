package releaseguard

import (
	"fmt"
	"os"
	"slices"

	"gopkg.in/yaml.v3"
)

type GoReleaserConfig struct {
	RawTopLevel  map[string]any      `yaml:"-"`
	Version      int                 `yaml:"version"`
	Builds       []GoReleaserBuild   `yaml:"builds"`
	Archives     []GoReleaserArchive `yaml:"archives"`
	Checksum     GoReleaserChecksum  `yaml:"checksum"`
	Changelog    any                 `yaml:"changelog"`
	HomebrewCask []GoReleaserCask    `yaml:"homebrew_casks"`
	Release      GoReleaserRelease   `yaml:"release"`
}

type GoReleaserBuild struct {
	ID     string   `yaml:"id"`
	Main   string   `yaml:"main"`
	Binary string   `yaml:"binary"`
	Goos   []string `yaml:"goos"`
	Goarch []string `yaml:"goarch"`
}

type GoReleaserArchive struct {
	ID           string   `yaml:"id"`
	IDs          []string `yaml:"ids"`
	NameTemplate string   `yaml:"name_template"`
}

type GoReleaserChecksum struct {
	NameTemplate string `yaml:"name_template"`
}

type GoReleaserRelease struct {
	Draft bool `yaml:"draft"`
}

type GoReleaserCask struct {
	Name         string                 `yaml:"name"`
	IDs          []string               `yaml:"ids"`
	Binaries     []string               `yaml:"binaries"`
	Repository   GoReleaserRepository   `yaml:"repository"`
	SkipUpload   bool                   `yaml:"skip_upload"`
	CommitAuthor GoReleaserCommitAuthor `yaml:"commit_author"`
}

type GoReleaserRepository struct {
	Owner  string `yaml:"owner"`
	Name   string `yaml:"name"`
	Branch string `yaml:"branch"`
	Token  string `yaml:"token"`
}

type GoReleaserCommitAuthor struct {
	Name  string `yaml:"name"`
	Email string `yaml:"email"`
}

func LoadGoReleaserConfig(path string) (GoReleaserConfig, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return GoReleaserConfig{}, err
	}
	var raw map[string]any
	if err := yaml.Unmarshal(data, &raw); err != nil {
		return GoReleaserConfig{}, fmt.Errorf("parse goreleaser yaml: %w", err)
	}
	var cfg GoReleaserConfig
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return GoReleaserConfig{}, fmt.Errorf("decode goreleaser yaml: %w", err)
	}
	cfg.RawTopLevel = raw
	return cfg, nil
}

func ValidateGoReleaserConfig(cfg GoReleaserConfig) error {
	allowed := map[string]struct{}{
		"version":        {},
		"builds":         {},
		"archives":       {},
		"checksum":       {},
		"changelog":      {},
		"homebrew_casks": {},
		"release":        {},
	}
	publishable := map[string]struct{}{
		"builds":         {},
		"archives":       {},
		"checksum":       {},
		"homebrew_casks": {},
		"release":        {},
	}
	for key, value := range cfg.RawTopLevel {
		if key == "brews" {
			return fmt.Errorf("deprecated publish surface %q is not allowed", key)
		}
		if _, ok := allowed[key]; !ok {
			return fmt.Errorf("unknown top-level goreleaser key %q", key)
		}
		if _, ok := publishable[key]; ok {
			if token, ok := findForbiddenToken(value); ok {
				return fmt.Errorf("goreleaser publish section %q contains forbidden smoke token %q", key, token)
			}
		}
	}
	if !cfg.Release.Draft {
		return fmt.Errorf("goreleaser release.draft must be true")
	}
	if len(cfg.Builds) == 0 {
		return fmt.Errorf("goreleaser builds must not be empty")
	}
	if len(cfg.Archives) == 0 {
		return fmt.Errorf("goreleaser archives must not be empty")
	}
	if cfg.Checksum.NameTemplate == "" {
		return fmt.Errorf("goreleaser checksum.name_template is required")
	}
	return nil
}

func ValidateHomebrewCaskConfig(cfg GoReleaserConfig) error {
	if _, ok := cfg.RawTopLevel["brews"]; ok {
		return fmt.Errorf("deprecated brews section is not allowed")
	}
	if len(cfg.HomebrewCask) != 1 {
		return fmt.Errorf("expected one homebrew_casks entry, got %d", len(cfg.HomebrewCask))
	}
	cask := cfg.HomebrewCask[0]
	if cask.Name != "ratchet-cli" {
		return fmt.Errorf("homebrew cask name = %q, want ratchet-cli", cask.Name)
	}
	if !slices.Equal(cask.IDs, []string{"ratchet"}) {
		return fmt.Errorf("homebrew cask ids = %v, want [ratchet]", cask.IDs)
	}
	if !slices.Equal(cask.Binaries, []string{"ratchet"}) {
		return fmt.Errorf("homebrew cask binaries = %v, want [ratchet]", cask.Binaries)
	}
	if cask.Repository.Owner != "GoCodeAlone" || cask.Repository.Name != "homebrew-tap" || cask.Repository.Branch != "main" {
		return fmt.Errorf("homebrew cask repository = %s/%s@%s, want GoCodeAlone/homebrew-tap@main", cask.Repository.Owner, cask.Repository.Name, cask.Repository.Branch)
	}
	if cask.Repository.Token != "{{ .Env.HOMEBREW_TAP_TOKEN }}" {
		return fmt.Errorf("homebrew cask repository token must use HOMEBREW_TAP_TOKEN")
	}
	if !cask.SkipUpload {
		return fmt.Errorf("homebrew cask skip_upload must be true so tap publish is gated after draft asset checks")
	}
	if cask.CommitAuthor.Name == "" || cask.CommitAuthor.Email == "" {
		return fmt.Errorf("homebrew cask commit_author is required")
	}
	return nil
}
