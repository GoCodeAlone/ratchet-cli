package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"time"
	"unicode"

	"github.com/GoCodeAlone/ratchet-cli/internal/hooks"
	"github.com/GoCodeAlone/ratchet-cli/internal/plugins"
)

type hookListRecord struct {
	Event         string `json:"event"`
	SourceKind    string `json:"source_kind"`
	SourceID      string `json:"source_id"`
	Status        string `json:"status"`
	Hash          string `json:"hash"`
	Command       string `json:"command"`
	Glob          string `json:"glob,omitempty"`
	PluginName    string `json:"plugin_name,omitempty"`
	PluginVersion string `json:"plugin_version,omitempty"`
	Managed       bool   `json:"managed"`
	Suppressed    bool   `json:"suppressed"`
}

type hookPolicyRecord struct {
	Mode             string `json:"mode"`
	SourcePath       string `json:"source_path"`
	ManagedHookCount int    `json:"managed_hook_count"`
}

var (
	loadManagedHookPolicy        = hooks.LoadManagedPolicy
	defaultManagedHookPolicyPath = hooks.DefaultManagedPolicyPath
)

func handleHooks(args []string) {
	if len(args) == 0 {
		printHooksUsage()
		return
	}
	switch args[0] {
	case "list":
		if err := handleHooksList(args[1:]); err != nil {
			fmt.Fprintf(os.Stderr, "error: %v\n", err)
			os.Exit(1)
		}
	case "policy":
		if err := handleHooksPolicy(args[1:]); err != nil {
			fmt.Fprintf(os.Stderr, "error: %v\n", err)
			os.Exit(1)
		}
	case "audit":
		if err := handleHooksAudit(args[1:]); err != nil {
			fmt.Fprintf(os.Stderr, "error: %v\n", err)
			os.Exit(1)
		}
	case "trust", "untrust", "disable":
		if len(args) != 2 || strings.TrimSpace(args[1]) == "" {
			fmt.Printf("Usage: ratchet hooks %s <hash>\n", args[0])
			return
		}
		if err := mutateHookTrust(args[0], args[1]); err != nil {
			fmt.Fprintf(os.Stderr, "error: %v\n", err)
			os.Exit(1)
		}
	default:
		fmt.Printf("unknown hooks command: %s\n", args[0])
		printHooksUsage()
	}
}

func handleHooksList(args []string) error {
	fs := flag.NewFlagSet("ratchet hooks list", flag.ContinueOnError)
	fs.SetOutput(os.Stdout)
	jsonOut := fs.Bool("json", false, "print JSON")
	cwd := fs.String("cwd", "", "working directory")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *cwd == "" {
		wd, err := os.Getwd()
		if err != nil {
			return err
		}
		*cwd = wd
	}
	records, err := discoverHooks(*cwd)
	if err != nil {
		return err
	}
	if *jsonOut {
		data, err := json.MarshalIndent(records, "", "  ")
		if err != nil {
			return err
		}
		fmt.Println(string(data))
		return nil
	}
	printHookRecords(records)
	return nil
}

func handleHooksPolicy(args []string) error {
	fs := flag.NewFlagSet("ratchet hooks policy", flag.ContinueOnError)
	fs.SetOutput(os.Stdout)
	jsonOut := fs.Bool("json", false, "print JSON")
	if err := fs.Parse(args); err != nil {
		return err
	}
	path, err := defaultManagedHookPolicyPath()
	if err != nil {
		return err
	}
	policy, err := loadManagedHookPolicy(hooks.LoadOptions{ManagedPath: path})
	if err != nil {
		return err
	}
	record := hookPolicyRecord{Mode: "none", SourcePath: controlSafeText(path)}
	if policy != nil {
		record.Mode = string(policy.Mode)
		for _, hookList := range policy.Hooks.Hooks {
			record.ManagedHookCount += len(hookList)
		}
	}
	if *jsonOut {
		data, err := json.MarshalIndent(record, "", "  ")
		if err != nil {
			return err
		}
		fmt.Println(string(data))
		return nil
	}
	fmt.Printf("Mode: %s\nSource: %s\nManaged hooks: %d\n", record.Mode, record.SourcePath, record.ManagedHookCount)
	return nil
}

func handleHooksAudit(args []string) error {
	fs := flag.NewFlagSet("ratchet hooks audit", flag.ContinueOnError)
	fs.SetOutput(os.Stdout)
	jsonOut := fs.Bool("json", false, "print JSON")
	limit := fs.Int("limit", hooks.DefaultHookAuditReadLimit(), "maximum records")
	if err := fs.Parse(args); err != nil {
		return err
	}
	records, err := hooks.NewHookAudit(hooks.DefaultHookAuditPath()).Read(*limit)
	if err != nil {
		return err
	}
	if *jsonOut {
		data, err := json.MarshalIndent(records, "", "  ")
		if err != nil {
			return err
		}
		fmt.Println(string(data))
		return nil
	}
	printHookAuditRecords(records)
	return nil
}

func mutateHookTrust(action, hash string) error {
	policy, err := loadManagedHookPolicy(hooks.LoadOptions{})
	if err != nil {
		return err
	}
	if managedPolicyContainsHash(policy, hash) {
		path, pathErr := defaultManagedHookPolicyPath()
		if pathErr != nil {
			return pathErr
		}
		return fmt.Errorf("managed policy hook %s is immutable; update %s as an administrator", hash, controlSafeText(path))
	}
	store, err := hooks.LoadTrustStore(hooks.DefaultTrustStorePath())
	if err != nil {
		return err
	}
	switch action {
	case "trust":
		if err := store.Trust(hash); err != nil {
			return err
		}
		fmt.Printf("Trusted hook %s\n", hash)
	case "untrust":
		if err := store.Untrust(hash); err != nil {
			return err
		}
		fmt.Printf("Untrusted hook %s\n", hash)
	case "disable":
		if err := store.Disable(hash); err != nil {
			return err
		}
		fmt.Printf("Disabled hook %s\n", hash)
	}
	return nil
}

func managedPolicyContainsHash(policy *hooks.ManagedPolicy, hash string) bool {
	if policy == nil || strings.TrimSpace(hash) == "" {
		return false
	}
	cfg := &hooks.HookConfig{Hooks: make(map[hooks.Event][]hooks.Hook)}
	cfg.ApplyManagedPolicy(policy)
	for _, hookList := range cfg.Hooks {
		for _, hook := range hookList {
			if hook.SourceKind == hooks.SourceManaged && hook.Hash == hash {
				return true
			}
		}
	}
	return false
}

func discoverHooks(cwd string) ([]hookListRecord, error) {
	store, err := hooks.LoadTrustStore(hooks.DefaultTrustStorePath())
	if err != nil {
		return nil, err
	}
	cfg, err := hooks.LoadWithOptions(hooks.LoadOptions{WorkingDir: cwd, TrustStore: store})
	if err != nil {
		return nil, err
	}
	pluginResult, err := plugins.NewLoader(filepath.Join(ratchetHomeDir(), "plugins")).LoadAll(context.Background())
	if err != nil {
		return nil, err
	}
	if pluginResult.Hooks != nil {
		pluginResult.Hooks.ApplyTrust(store)
		for event, hookList := range pluginResult.Hooks.Hooks {
			cfg.Hooks[event] = append(cfg.Hooks[event], hookList...)
		}
	}
	cfg.ApplyTrust(store)
	policy, err := loadManagedHookPolicy(hooks.LoadOptions{})
	if err != nil {
		return nil, err
	}
	cfg.ApplyManagedPolicy(policy)

	var records []hookListRecord
	for event, hookList := range cfg.Hooks {
		for _, h := range hookList {
			records = append(records, hookListRecord{
				Event:         string(event),
				SourceKind:    string(h.SourceKind),
				SourceID:      h.SourceID,
				Status:        hookStatus(h),
				Hash:          h.Hash,
				Command:       summarizeHookCommand(h),
				Glob:          h.Glob,
				PluginName:    h.PluginName,
				PluginVersion: h.PluginVersion,
				Managed:       h.SourceKind == hooks.SourceManaged,
				Suppressed:    h.Suppressed,
			})
		}
	}
	sort.Slice(records, func(i, j int) bool {
		if records[i].Event != records[j].Event {
			return records[i].Event < records[j].Event
		}
		if records[i].SourceKind != records[j].SourceKind {
			return records[i].SourceKind < records[j].SourceKind
		}
		return records[i].Hash < records[j].Hash
	})
	return records, nil
}

func hookStatus(h hooks.Hook) string {
	switch {
	case h.Suppressed:
		return "suppressed"
	case h.Disabled:
		return "disabled"
	case h.UnsupportedPlatform:
		return "unsupported"
	case h.SourceKind == hooks.SourceManaged:
		return "managed"
	case h.Trusted || h.SourceKind == "":
		return "trusted"
	default:
		return "untrusted"
	}
}

func summarizeHookCommand(h hooks.Hook) string {
	command := h.Command
	if runtime.GOOS == "windows" && h.CommandWindows != "" {
		command = h.CommandWindows
	}
	command = strings.Join(strings.Fields(controlSafeText(command)), " ")
	const maxRunes = 80
	runes := []rune(command)
	if len(runes) <= maxRunes {
		return command
	}
	return string(runes[:maxRunes-3]) + "..."
}

func controlSafeText(value string) string {
	return strings.Map(func(r rune) rune {
		if unicode.IsControl(r) {
			return ' '
		}
		return r
	}, value)
}

func ratchetHomeDir() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".ratchet")
}

func printHookRecords(records []hookListRecord) {
	if len(records) == 0 {
		fmt.Println("No hooks configured.")
		return
	}
	fmt.Printf("%-14s %-8s %-11s %-12s %s\n", "EVENT", "SOURCE", "STATUS", "HASH", "COMMAND")
	for _, record := range records {
		hash := record.Hash
		if len(hash) > 12 {
			hash = hash[:12]
		}
		fmt.Printf("%-14s %-8s %-11s %-12s %s\n", record.Event, record.SourceKind, record.Status, hash, record.Command)
	}
}

func printHookAuditRecords(records []hooks.HookAuditRecord) {
	if len(records) == 0 {
		fmt.Println("No managed hook audit records.")
		return
	}
	fmt.Printf("%-30s %-18s %-8s %-15s %-8s %s\n", "TIMESTAMP", "EVENT", "SOURCE", "RESULT", "MS", "HASH")
	for _, record := range records {
		hash := record.Hash
		if len(hash) > 12 {
			hash = hash[:12]
		}
		fmt.Printf("%-30s %-18s %-8s %-15s %-8d %s\n",
			record.Timestamp.UTC().Format(time.RFC3339Nano), record.Event, record.Source, record.Result, record.DurationMS, hash)
	}
}

func printHooksUsage() {
	fmt.Println(`Usage: ratchet hooks <command>

Commands:
  list [--json] [--cwd dir]  Review discovered hooks and enforcement status
  policy [--json]            Inspect administrator-managed hook policy
  audit [--json] [--limit n] Inspect managed hook execution audit
  trust <hash>               Trust a hook descriptor hash
  untrust <hash>             Remove explicit trust for a hook hash
  disable <hash>             Disable a hook hash; disabled wins over trust`)
}
