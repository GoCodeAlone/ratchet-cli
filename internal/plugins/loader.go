package plugins

import (
	"fmt"
	"os"
	"path/filepath"
)

// PluginInfo describes a discovered plugin binary.
type PluginInfo struct {
	Name string
	Path string
}

// Loader discovers and tracks external plugins from the plugins directory.
type Loader struct {
	pluginDir string
	loaded    []PluginInfo
}

func NewLoader(pluginDir string) *Loader {
	return &Loader{pluginDir: pluginDir}
}

func pluginsDir() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".ratchet", "plugins")
}

// LoadAll discovers plugin binaries in the plugins directory.
// Each executable file is treated as a plugin.
func (l *Loader) LoadAll() ([]PluginInfo, error) {
	entries, err := os.ReadDir(l.pluginDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("read plugins dir: %w", err)
	}

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		info, err := entry.Info()
		if err != nil || info.Mode()&0111 == 0 {
			continue // skip non-executable files
		}
		l.loaded = append(l.loaded, PluginInfo{
			Name: entry.Name(),
			Path: filepath.Join(l.pluginDir, entry.Name()),
		})
	}
	return l.loaded, nil
}

// Install downloads a plugin binary from the registry and places it in the plugins dir.
func Install(name string) error {
	return fmt.Errorf("plugin install not yet implemented")
}

// Remove deletes a plugin binary.
func Remove(name string) error {
	path := filepath.Join(pluginsDir(), name)
	return os.Remove(path)
}

// List returns names of installed plugins.
func List() ([]string, error) {
	dir := pluginsDir()
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var names []string
	for _, e := range entries {
		if !e.IsDir() {
			names = append(names, e.Name())
		}
	}
	return names, nil
}
