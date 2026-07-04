package releaseguard

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
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
	if _, err := os.Stat(filepath.Join(tap, "Formula", "ratchet-cli.rb")); err == nil {
		errs = append(errs, fmt.Errorf("stale unmanaged tap surface Formula/ratchet-cli.rb must be absent"))
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
