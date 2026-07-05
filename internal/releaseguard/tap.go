package releaseguard

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
)

func GuardTapPreflight(root, tap string) error {
	cfg, err := LoadGoReleaserConfig(filepath.Join(root, ".goreleaser.yaml"))
	if err != nil {
		return err
	}
	if err := ValidateHomebrewCaskConfig(cfg); err != nil {
		return err
	}
	var errs []error
	if _, err := os.Stat(filepath.Join(tap, "ratchet-cli.rb")); err == nil {
		errs = append(errs, fmt.Errorf("stale unmanaged tap surface ratchet-cli.rb must be absent"))
	} else if !errors.Is(err, os.ErrNotExist) {
		errs = append(errs, err)
	}
	caskPath := filepath.Join(tap, "Casks", "ratchet-cli.rb")
	data, err := os.ReadFile(caskPath)
	if err != nil {
		errs = append(errs, fmt.Errorf("read managed cask Casks/ratchet-cli.rb: %w", err))
	} else if err := validateTapCask(data); err != nil {
		errs = append(errs, err)
	}
	formulaPath := filepath.Join(tap, "Formula", "ratchet-cli.rb")
	data, err = os.ReadFile(formulaPath)
	if err != nil {
		errs = append(errs, fmt.Errorf("read managed formula Formula/ratchet-cli.rb: %w", err))
	} else if err := validateTapFormula(data); err != nil {
		errs = append(errs, err)
	}
	return errors.Join(errs...)
}

func validateTapCask(data []byte) error {
	text := string(data)
	if token, ok := findForbiddenToken(text); ok {
		return fmt.Errorf("managed cask contains forbidden smoke token %q", token)
	}
	for _, want := range []string{`cask "ratchet-cli"`, `binary "ratchet"`} {
		if !strings.Contains(text, want) {
			return fmt.Errorf("managed cask missing %s", want)
		}
	}
	return nil
}

func validateTapFormula(data []byte) error {
	text := string(data)
	if token, ok := findForbiddenToken(text); ok {
		return fmt.Errorf("managed formula contains forbidden smoke token %q", token)
	}
	for _, want := range []string{`class RatchetCli < Formula`, `bin.install "ratchet"`} {
		if !strings.Contains(text, want) {
			return fmt.Errorf("managed formula missing %s", want)
		}
	}
	return nil
}

func GuardTapPostcheck(root, tap, names, commits, version string) error {
	if err := GuardTapPreflight(root, tap); err != nil {
		return err
	}
	if !slices.Contains(splitCSV(names), "ratchet-cli") {
		return fmt.Errorf("tap postcheck names %q must include ratchet-cli", names)
	}
	commitMap, err := parsePathCommits(commits)
	if err != nil {
		return err
	}
	if commitMap["Casks/ratchet-cli.rb"] == "" {
		return fmt.Errorf("tap postcheck commits must include Casks/ratchet-cli.rb=<sha>")
	}
	if commitMap["Formula/ratchet-cli.rb"] == "" {
		return fmt.Errorf("tap postcheck commits must include Formula/ratchet-cli.rb=<sha>")
	}
	for _, rel := range []string{"Casks/ratchet-cli.rb", "Formula/ratchet-cli.rb"} {
		data, err := os.ReadFile(filepath.Join(tap, rel))
		if err != nil {
			return err
		}
		text := string(data)
		if !strings.Contains(text, version) && !strings.Contains(text, strings.TrimPrefix(version, "v")) {
			return fmt.Errorf("managed %s does not reference requested version %q", rel, version)
		}
	}
	return nil
}

func parsePathCommits(value string) (map[string]string, error) {
	out := map[string]string{}
	for item := range strings.SplitSeq(value, ",") {
		item = strings.TrimSpace(item)
		if item == "" {
			continue
		}
		path, sha, ok := strings.Cut(item, "=")
		path = strings.TrimSpace(path)
		sha = strings.TrimSpace(sha)
		if !ok || path == "" || sha == "" {
			return nil, fmt.Errorf("tap postcheck commit entry %q must be <path>=<sha>", item)
		}
		out[path] = sha
	}
	return out, nil
}

func splitCSV(value string) []string {
	var out []string
	for item := range strings.SplitSeq(value, ",") {
		item = strings.TrimSpace(item)
		if item != "" {
			out = append(out, item)
		}
	}
	return out
}
