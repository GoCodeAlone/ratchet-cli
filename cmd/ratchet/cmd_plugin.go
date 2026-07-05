package main

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/GoCodeAlone/ratchet-cli/internal/client"
	"github.com/GoCodeAlone/ratchet-cli/internal/plugins"
)

func handlePlugin(args []string) {
	if err := executePlugin(args, os.Stdout); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
}

func executePlugin(args []string, w io.Writer) error {
	if len(args) == 0 {
		fmt.Fprintln(w, "Usage: ratchet plugin <list|install|remove|reload|marketplace|update|enable|disable>")
		return nil
	}
	switch args[0] {
	case "list":
		reg, err := plugins.Load()
		if err != nil {
			return err
		}
		if len(reg.Plugins) == 0 {
			fmt.Fprintln(w, "No plugins installed.")
			return nil
		}
		fmt.Fprintf(w, "%-20s %-10s %-9s %s\n", "NAME", "VERSION", "STATUS", "SOURCE")
		names := make([]string, 0, len(reg.Plugins))
		for name := range reg.Plugins {
			names = append(names, name)
		}
		sort.Strings(names)
		for _, name := range names {
			entry := reg.Plugins[name]
			status := "disabled"
			if entry.Enabled {
				status = "enabled"
			}
			fmt.Fprintf(w, "%-20s %-10s %-9s %s\n", name, entry.Version, status, entry.Source)
		}
	case "marketplace":
		return executePluginMarketplace(args[1:], w)
	case "update":
		if len(args) < 2 {
			return fmt.Errorf("usage: ratchet plugin update <name|--all>")
		}
		if args[1] == "--all" {
			if err := plugins.UpdateAllInstalledPlugins(context.Background()); err != nil {
				return err
			}
			fmt.Fprintln(w, "Updated all plugins.")
			return nil
		}
		if err := plugins.UpdateInstalledPlugin(context.Background(), args[1]); err != nil {
			return err
		}
		fmt.Fprintf(w, "Updated plugin: %s\n", args[1])
	case "enable", "disable":
		if len(args) < 2 {
			return fmt.Errorf("usage: ratchet plugin %s <name>", args[0])
		}
		reg, err := plugins.Load()
		if err != nil {
			return err
		}
		enabled := args[0] == "enable"
		if err := reg.SetEnabled(args[1], enabled); err != nil {
			return err
		}
		status := "disabled"
		if enabled {
			status = "enabled"
		}
		fmt.Fprintf(w, "Plugin %s %s.\n", args[1], status)
	case "install":
		if len(args) < 2 {
			return fmt.Errorf("usage: ratchet plugin install <owner/repo, ./path, or name@marketplace>")
		}
		src := args[1]
		var err error
		if strings.Contains(src, "@") && !isLocalPath(src) {
			err = plugins.InstallFromMarketplace(context.Background(), src)
		} else if isLocalPath(src) {
			src = expandHome(src)
			err = plugins.InstallFromLocal(src)
		} else {
			err = plugins.InstallFromGitHub(context.Background(), src)
		}
		if err != nil {
			return err
		}
		fmt.Fprintf(w, "Installed plugin: %s\n", args[1])
	case "remove":
		if len(args) < 2 {
			return fmt.Errorf("usage: ratchet plugin remove <name>")
		}
		if err := plugins.Uninstall(args[1]); err != nil {
			return err
		}
		fmt.Fprintf(w, "Removed plugin: %s\n", args[1])
	case "reload":
		c, err := client.EnsureDaemon()
		if err != nil {
			return err
		}
		defer c.Close()
		statuses, err := c.RequestPluginReload(context.Background())
		if err != nil {
			return err
		}
		for status := range statuses {
			fmt.Fprintf(w, "%s: %s\n", status.GetStatus(), status.GetMessage())
		}
	default:
		return fmt.Errorf("unknown plugin command: %s", args[0])
	}
	return nil
}

func executePluginMarketplace(args []string, w io.Writer) error {
	if len(args) == 0 {
		return fmt.Errorf("usage: ratchet plugin marketplace <add|list|update|remove>")
	}
	reg, err := plugins.LoadDefaultMarketplaceRegistry()
	if err != nil {
		return err
	}
	switch args[0] {
	case "add":
		if len(args) < 3 {
			return fmt.Errorf("usage: ratchet plugin marketplace add <name> <source> [--auto-update]")
		}
		source := plugins.MarketplaceSource{Name: args[1], Source: expandHome(args[2])}
		for _, arg := range args[3:] {
			if arg == "--auto-update" {
				source.AutoUpdate = true
			}
		}
		if err := reg.Add(source); err != nil {
			return err
		}
		fmt.Fprintf(w, "Added marketplace: %s\n", source.Name)
	case "list":
		sources := reg.List()
		if len(sources) == 0 {
			fmt.Fprintln(w, "No marketplaces configured.")
			return nil
		}
		fmt.Fprintf(w, "%-20s %-8s %s\n", "NAME", "UPDATE", "SOURCE")
		for _, source := range sources {
			update := "manual"
			if source.AutoUpdate {
				update = "auto"
			}
			fmt.Fprintf(w, "%-20s %-8s %s\n", source.Name, update, source.Source)
		}
	case "update":
		if len(args) < 2 {
			return fmt.Errorf("usage: ratchet plugin marketplace update <name|--all>")
		}
		if args[1] == "--all" {
			for _, source := range reg.List() {
				if _, err := plugins.LoadMarketplaceCatalogFromSource(context.Background(), source.Source); err != nil {
					return fmt.Errorf("update marketplace %s: %w", source.Name, err)
				}
			}
			fmt.Fprintln(w, "Updated all marketplaces.")
			return nil
		}
		source, ok := reg.Get(args[1])
		if !ok {
			return fmt.Errorf("marketplace %q not configured", args[1])
		}
		if _, err := plugins.LoadMarketplaceCatalogFromSource(context.Background(), source.Source); err != nil {
			return err
		}
		fmt.Fprintf(w, "Updated marketplace: %s\n", args[1])
	case "remove":
		if len(args) < 2 {
			return fmt.Errorf("usage: ratchet plugin marketplace remove <name>")
		}
		if err := reg.Remove(args[1]); err != nil {
			return err
		}
		fmt.Fprintf(w, "Removed marketplace: %s\n", args[1])
	default:
		return fmt.Errorf("unknown marketplace command: %s", args[0])
	}
	return nil
}

// expandHome replaces a leading ~ with the user's home directory.
func expandHome(path string) string {
	if !strings.HasPrefix(path, "~") {
		return path
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return path
	}
	return filepath.Join(home, path[1:])
}

// isLocalPath returns true if src looks like a local filesystem path
// rather than a GitHub owner/repo reference.
func isLocalPath(src string) bool {
	if len(src) == 0 {
		return false
	}
	// Explicit relative/absolute paths
	if src[0] == '.' || src[0] == '/' {
		return true
	}
	// Home-relative paths
	if strings.HasPrefix(src, "~") {
		return true
	}
	// Windows absolute paths (e.g. C:\...)
	if len(src) >= 2 && src[1] == ':' {
		return true
	}
	// If it exists on disk, treat it as local
	if _, err := os.Stat(src); err == nil {
		return true
	}
	return false
}
