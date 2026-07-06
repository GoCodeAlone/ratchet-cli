//go:build !windows

package tui

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/GoCodeAlone/ratchet-cli/internal/harnessredact"
)

func TestTUIBinarySmoke(t *testing.T) {
	if raceEnabled {
		t.Skip("TUI binary PTY smoke is disabled under -race")
	}
	root := tuiRepoRoot(t)
	tempRoot := t.TempDir()
	home := filepath.Join(tempRoot, "home")
	state := filepath.Join(tempRoot, "state")
	work := filepath.Join(tempRoot, "work")
	bin := filepath.Join(tempRoot, "ratchet-tui-smoke")
	spec := loadCommandSurfaceSpec(t, root)
	for _, dir := range []string{home, state, work} {
		if err := os.MkdirAll(dir, 0700); err != nil {
			t.Fatalf("mkdir %s: %v", dir, err)
		}
	}
	redValues := append([]string{
		home,
		root,
		tempRoot,
		filepath.Join(tempRoot, "ratchet.sock"),
		bin,
		filepath.Join(tempRoot, "dist"),
		"smoke prompt body",
	}, trustBodiesFromSpec(spec)...)
	redactor := harnessredact.New(redValues...)
	red := redactor.String

	build := exec.CommandContext(context.Background(), "go", "build", "-tags", "tui_smoke", "-o", bin, "./cmd/ratchet-tui-smoke")
	build.Dir = root
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build smoke binary: %v\n%s", err, red(string(out)))
	}

	env := []string{
		"HOME=" + home,
		"USERPROFILE=" + home,
		"XDG_STATE_HOME=" + state,
	}
	s := startTUITestPTY(t, bin, work, env, red)
	if match, _ := s.waitForAny([]string{"press any key", "Message ratchet"}, 6*time.Second); match == "press any key" {
		s.send(" ")
	}
	s.waitFor("Message ratchet", 6*time.Second)

	s.sendLine("smoke prompt body")
	s.waitFor("I have completed the task.", 15*time.Second)
	assertNoInstructionOrHookSurface(t, s.snapshot(), red)

	for _, row := range spec.Commands {
		if row.Evidence != "pty-proven" || row.Command == "/exit" || row.Command == "/tree" {
			continue
		}
		s.clear()
		s.submitSlash(row.Command)
		s.waitFor(expectedForSmokeCommand(row.Command), 8*time.Second)
		assertTrustStateAfterCommand(t, s, row.Command)
	}

	s.clear()
	s.submitSlash("/tree")
	s.waitFor(expectedForSmokeCommand("/tree"), 8*time.Second)
	s.send("\x1b")
	s.waitFor("Message ratchet", 8*time.Second)

	for _, row := range spec.Shortcuts {
		if row.Evidence != "pty-proven" {
			continue
		}
		s.clear()
		switch row.Keys {
		case "ctrl+b":
			// Covered in TestTUIBinarySmokeSessionTreeShortcut to keep tree navigation in a fresh PTY session.
			continue
		case "ctrl+s":
			// Covered in TestTUIBinarySmokeSidebarShortcut to keep sidebar navigation in a fresh PTY session.
			continue
		case "ctrl+t":
			// Covered in TestTUIBinarySmokeTeamShortcut to keep team navigation in a fresh PTY session.
			continue
		case "ctrl+j":
			s.sendCtrl('j')
			s.waitFor("tui-smoke-daemon", 8*time.Second)
			if strings.Contains(strings.ToLower(s.snapshot()), "rpc error") {
				t.Fatalf("job panel contained RPC error:\n%s", red(s.snapshot()))
			}
			s.sendCtrl('j')
			s.waitFor("Message ratchet", 8*time.Second)
		default:
			t.Fatalf("unhandled pty-proven shortcut %q", row.Keys)
		}
	}

	assertNoInstructionOrHookSurface(t, s.snapshot(), red)
	s.clear()
	s.submitSlash("/exit")
	s.waitExit(5 * time.Second)
}

func TestTUIBinarySmokeTeamShortcut(t *testing.T) {
	if raceEnabled {
		t.Skip("TUI binary PTY smoke is disabled under -race")
	}
	root := tuiRepoRoot(t)
	tempRoot := t.TempDir()
	home := filepath.Join(tempRoot, "home")
	state := filepath.Join(tempRoot, "state")
	work := filepath.Join(tempRoot, "work")
	bin := filepath.Join(tempRoot, "ratchet-tui-smoke")
	for _, dir := range []string{home, state, work} {
		if err := os.MkdirAll(dir, 0700); err != nil {
			t.Fatalf("mkdir %s: %v", dir, err)
		}
	}
	red := harnessredact.New(home, root, tempRoot, filepath.Join(tempRoot, "ratchet.sock"), bin).String
	build := exec.CommandContext(context.Background(), "go", "build", "-tags", "tui_smoke", "-o", bin, "./cmd/ratchet-tui-smoke")
	build.Dir = root
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build smoke binary: %v\n%s", err, red(string(out)))
	}

	env := []string{
		"HOME=" + home,
		"USERPROFILE=" + home,
		"XDG_STATE_HOME=" + state,
	}
	s := startTUITestPTY(t, bin, work, env, red)
	if match, _ := s.waitForAny([]string{"press any key", "Message ratchet"}, 6*time.Second); match == "press any key" {
		s.send(" ")
	}
	s.waitFor("Message ratchet", 6*time.Second)
	s.clear()
	s.sendCtrl('t')
	s.waitFor("Team View", 8*time.Second)
	s.sendCtrl('t')
	s.waitFor("Message ratchet", 8*time.Second)
	assertNoInstructionOrHookSurface(t, s.snapshot(), red)
	s.sendCtrl('c')
	s.waitExit(5 * time.Second)
}

func TestTUIBinarySmokeSidebarShortcut(t *testing.T) {
	if raceEnabled {
		t.Skip("TUI binary PTY smoke is disabled under -race")
	}
	root := tuiRepoRoot(t)
	tempRoot := t.TempDir()
	home := filepath.Join(tempRoot, "home")
	state := filepath.Join(tempRoot, "state")
	work := filepath.Join(tempRoot, "work")
	bin := filepath.Join(tempRoot, "ratchet-tui-smoke")
	for _, dir := range []string{home, state, work} {
		if err := os.MkdirAll(dir, 0700); err != nil {
			t.Fatalf("mkdir %s: %v", dir, err)
		}
	}
	red := harnessredact.New(home, root, tempRoot, filepath.Join(tempRoot, "ratchet.sock"), bin).String
	build := exec.CommandContext(context.Background(), "go", "build", "-tags", "tui_smoke", "-o", bin, "./cmd/ratchet-tui-smoke")
	build.Dir = root
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build smoke binary: %v\n%s", err, red(string(out)))
	}

	env := []string{
		"HOME=" + home,
		"USERPROFILE=" + home,
		"XDG_STATE_HOME=" + state,
	}
	s := startTUITestPTY(t, bin, work, env, red)
	if match, _ := s.waitForAny([]string{"press any key", "Message ratchet"}, 6*time.Second); match == "press any key" {
		s.send(" ")
	}
	s.waitFor("Message ratchet", 6*time.Second)
	s.clear()
	s.sendCtrl('s')
	s.waitFor("Sessions", 8*time.Second)
	s.sendCtrl('s')
	s.waitFor("Message ratchet", 8*time.Second)
	assertNoInstructionOrHookSurface(t, s.snapshot(), red)
	s.sendCtrl('c')
	s.waitExit(5 * time.Second)
}

func TestTUIBinarySmokeSessionTreeShortcut(t *testing.T) {
	if raceEnabled {
		t.Skip("TUI binary PTY smoke is disabled under -race")
	}
	root := tuiRepoRoot(t)
	tempRoot := t.TempDir()
	home := filepath.Join(tempRoot, "home")
	state := filepath.Join(tempRoot, "state")
	work := filepath.Join(tempRoot, "work")
	bin := filepath.Join(tempRoot, "ratchet-tui-smoke")
	for _, dir := range []string{home, state, work} {
		if err := os.MkdirAll(dir, 0700); err != nil {
			t.Fatalf("mkdir %s: %v", dir, err)
		}
	}
	red := harnessredact.New(home, root, tempRoot, filepath.Join(tempRoot, "ratchet.sock"), bin).String
	build := exec.CommandContext(context.Background(), "go", "build", "-tags", "tui_smoke", "-o", bin, "./cmd/ratchet-tui-smoke")
	build.Dir = root
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build smoke binary: %v\n%s", err, red(string(out)))
	}

	env := []string{
		"HOME=" + home,
		"USERPROFILE=" + home,
		"XDG_STATE_HOME=" + state,
	}
	s := startTUITestPTY(t, bin, work, env, red)
	if match, _ := s.waitForAny([]string{"press any key", "Message ratchet"}, 6*time.Second); match == "press any key" {
		s.send(" ")
	}
	s.waitFor("Message ratchet", 6*time.Second)
	s.clear()
	s.sendCtrl('b')
	s.waitFor("Session Tree", 8*time.Second)
	s.send("\x1b")
	s.waitFor("Message ratchet", 8*time.Second)
	assertNoInstructionOrHookSurface(t, s.snapshot(), red)
	s.sendCtrl('c')
	s.waitExit(5 * time.Second)
}

func TestTUIBinarySmokeEmptyJobs(t *testing.T) {
	if raceEnabled {
		t.Skip("TUI binary PTY smoke is disabled under -race")
	}
	root := tuiRepoRoot(t)
	tempRoot := t.TempDir()
	home := filepath.Join(tempRoot, "home")
	state := filepath.Join(tempRoot, "state")
	work := filepath.Join(tempRoot, "work")
	bin := filepath.Join(tempRoot, "ratchet-tui-smoke")
	for _, dir := range []string{home, state, work} {
		if err := os.MkdirAll(dir, 0700); err != nil {
			t.Fatalf("mkdir %s: %v", dir, err)
		}
	}
	red := harnessredact.New(home, root, tempRoot, filepath.Join(tempRoot, "ratchet.sock"), bin).String
	build := exec.CommandContext(context.Background(), "go", "build", "-tags", "tui_smoke", "-o", bin, "./cmd/ratchet-tui-smoke")
	build.Dir = root
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build smoke binary: %v\n%s", err, red(string(out)))
	}

	env := []string{
		"HOME=" + home,
		"USERPROFILE=" + home,
		"XDG_STATE_HOME=" + state,
		"RATCHET_TUI_SMOKE_EMPTY_JOBS=1",
	}
	s := startTUITestPTY(t, bin, work, env, red)
	if match, _ := s.waitForAny([]string{"press any key", "Message ratchet"}, 6*time.Second); match == "press any key" {
		s.send(" ")
	}
	s.waitFor("Message ratchet", 6*time.Second)
	s.clear()
	s.sendCtrl('j')
	s.waitFor("No active jobs", 8*time.Second)
	if strings.Contains(strings.ToLower(s.snapshot()), "rpc error") {
		t.Fatalf("empty job panel contained RPC error:\n%s", red(s.snapshot()))
	}
	s.sendCtrl('c')
	s.waitExit(5 * time.Second)
}

func TestTUIBinarySmokeExitKeys(t *testing.T) {
	if raceEnabled {
		t.Skip("TUI binary PTY smoke is disabled under -race")
	}
	root := tuiRepoRoot(t)
	tempRoot := t.TempDir()
	bin := filepath.Join(tempRoot, "ratchet-tui-smoke")
	redBuild := harnessredact.New(root, tempRoot, bin).String
	build := exec.Command("go", "build", "-tags", "tui_smoke", "-o", bin, "./cmd/ratchet-tui-smoke")
	build.Dir = root
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build smoke binary: %v\n%s", err, redBuild(string(out)))
	}
	for _, tc := range []struct {
		name string
		key  byte
	}{
		{name: "ctrl-c", key: 'c'},
		{name: "ctrl-d", key: 'd'},
	} {
		t.Run(tc.name, func(t *testing.T) {
			home := filepath.Join(t.TempDir(), "home")
			state := filepath.Join(t.TempDir(), "state")
			work := filepath.Join(t.TempDir(), "work")
			for _, dir := range []string{home, state, work} {
				if err := os.MkdirAll(dir, 0700); err != nil {
					t.Fatalf("mkdir %s: %v", dir, err)
				}
			}
			red := harnessredact.New(home, root, filepath.Dir(home), filepath.Join(filepath.Dir(home), "ratchet.sock"), bin).String
			env := []string{
				"HOME=" + home,
				"USERPROFILE=" + home,
				"XDG_STATE_HOME=" + state,
			}
			s := startTUITestPTY(t, bin, work, env, red)
			s.waitFor("Message ratchet", 8*time.Second)
			s.sendCtrl(tc.key)
			s.waitExit(5 * time.Second)
		})
	}
}

type commandSurfaceSpec struct {
	Commands  []commandSurfaceRow  `json:"commands"`
	Shortcuts []shortcutSurfaceRow `json:"shortcuts"`
}

type commandSurfaceRow struct {
	Command  string `json:"command"`
	Evidence string `json:"evidence"`
}

type shortcutSurfaceRow struct {
	Keys     string `json:"keys"`
	Evidence string `json:"evidence"`
}

func loadCommandSurfaceSpec(t *testing.T, root string) commandSurfaceSpec {
	t.Helper()
	path := filepath.Join(root, "internal", "tui", "commands", "testdata", "command_surface_spec.json")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read command surface spec: %v", err)
	}
	var spec commandSurfaceSpec
	if err := json.Unmarshal(data, &spec); err != nil {
		t.Fatalf("parse command surface spec: %v", err)
	}
	if len(spec.Commands) == 0 || len(spec.Shortcuts) == 0 {
		t.Fatalf("command surface spec must declare pty-proven commands and shortcuts")
	}
	return spec
}

func trustBodiesFromSpec(spec commandSurfaceSpec) []string {
	var values []string
	for _, row := range spec.Commands {
		if row.Evidence != "pty-proven" || !strings.HasPrefix(row.Command, "/trust ") {
			continue
		}
		fields := commandFields(row.Command)
		if len(fields) < 3 {
			continue
		}
		switch fields[1] {
		case "allow", "deny", "revoke":
			values = append(values, fields[2])
		case "persist":
			if len(fields) >= 4 {
				values = append(values, fields[3])
			}
		}
	}
	return values
}

func (s *tuiPTY) submitSlash(cmd string) {
	s.t.Helper()
	text := cmd + " "
	for _, r := range text {
		s.send(string(r))
		time.Sleep(20 * time.Millisecond)
	}
	time.Sleep(300 * time.Millisecond)
	s.send("\r")
}

func expectedForSmokeCommand(cmd string) string {
	switch {
	case cmd == "/help":
		return "Quit ratchet"
	case cmd == "/provider list":
		return "e2e-mock"
	case cmd == "/tree":
		return "Session Tree"
	case strings.HasPrefix(cmd, "/mode "):
		return "Mode switched"
	case strings.HasPrefix(cmd, "/trust allow"):
		return "Added allow rule"
	case strings.HasPrefix(cmd, "/trust deny"):
		return "Added deny rule"
	case strings.HasPrefix(cmd, "/trust persist allow"):
		return "Persisted allow grant"
	case strings.HasPrefix(cmd, "/trust persist deny"):
		return "Persisted deny grant"
	case strings.HasPrefix(cmd, "/trust revoke"):
		return "Revoked persistent trust grant"
	case cmd == "/trust list":
		return "Mode:"
	case cmd == "/trust grants":
		return "smoke"
	case cmd == "/trust reset":
		return "Mode: conservative"
	default:
		return cmd
	}
}

func assertTrustStateAfterCommand(t *testing.T, s *tuiPTY, cmd string) {
	t.Helper()
	switch {
	case strings.HasPrefix(cmd, "/trust allow"):
		s.clear()
		s.submitSlash("/trust list")
		out := s.waitFor("smoke:allow", 8*time.Second)
		assertTrustRule(t, s, out, cmd, "allow", "smoke", "smoke:allow")
	case strings.HasPrefix(cmd, "/trust deny"):
		s.clear()
		s.submitSlash("/trust list")
		out := s.waitFor("smoke:deny", 8*time.Second)
		assertTrustRule(t, s, out, cmd, "deny", "smoke", "smoke:deny")
	case strings.HasPrefix(cmd, "/trust persist allow"):
		s.clear()
		s.submitSlash("/trust grants")
		out := s.waitFor("smoke:persist-allow", 8*time.Second)
		assertTrustGrant(t, s, out, cmd, "allow", "smoke", "operator", "smoke:persist-allow")
	case strings.HasPrefix(cmd, "/trust persist deny"):
		s.clear()
		s.submitSlash("/trust grants")
		out := s.waitFor("smoke:persist-deny", 8*time.Second)
		assertTrustGrant(t, s, out, cmd, "deny", "smoke", "operator", "smoke:persist-deny")
	case strings.HasPrefix(cmd, "/trust revoke"):
		s.clear()
		s.submitSlash("/trust grants")
		out := s.waitFor("smoke:persist-deny", 8*time.Second)
		assertTrustGrant(t, s, out, cmd, "deny", "smoke", "operator", "smoke:persist-deny")
		if trustGrantPresent(out, "allow", "smoke", "operator", "smoke:persist-allow") {
			t.Fatalf("trust grants after %q still included revoked grant:\n%s", cmd, s.red(out))
		}
	case cmd == "/trust reset":
		s.clear()
		s.submitSlash("/trust list")
		out := s.waitFor("Mode: conservative", 8*time.Second)
		if strings.Contains(out, "smoke:allow") || strings.Contains(out, "smoke:deny") {
			t.Fatalf("trust reset left runtime smoke rules:\n%s", s.red(out))
		}
		s.clear()
		s.submitSlash("/trust grants")
		s.waitFor("smoke:persist-deny", 8*time.Second)
	}
}

func assertTrustRule(t *testing.T, s *tuiPTY, out, cmd, action, scope, pattern string) {
	t.Helper()
	if !trustRulePresent(out, action, scope, pattern) {
		t.Fatalf("trust state after %q missing rule %s %s %q:\n%s", cmd, action, scope, pattern, s.red(out))
	}
}

func assertTrustGrant(t *testing.T, s *tuiPTY, out, cmd, action, scope, grantedBy, pattern string) {
	t.Helper()
	if !trustGrantPresent(out, action, scope, grantedBy, pattern) {
		t.Fatalf("trust state after %q missing grant %s %s %s %q:\n%s", cmd, action, scope, grantedBy, pattern, s.red(out))
	}
}

func trustRulePresent(out, action, scope, pattern string) bool {
	for _, line := range trustOutputLines(out) {
		fields := strings.Fields(line)
		if len(fields) >= 3 && fields[0] == action && fields[1] == scope && strings.Join(fields[2:], " ") == pattern {
			return true
		}
	}
	return false
}

func trustGrantPresent(out, action, scope, grantedBy, pattern string) bool {
	for _, line := range trustOutputLines(out) {
		fields := strings.Fields(line)
		if len(fields) >= 4 && fields[0] == action && fields[1] == scope && fields[2] == grantedBy && strings.Join(fields[3:], " ") == pattern {
			return true
		}
	}
	return false
}

func trustOutputLines(out string) []string {
	var lines []string
	for _, line := range strings.Split(out, "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "⚙") {
			line = strings.TrimSpace(strings.TrimPrefix(line, "⚙"))
		}
		if i := strings.Index(line, "Message ratchet"); i >= 0 {
			line = strings.TrimSpace(line[:i])
		}
		if line != "" {
			lines = append(lines, line)
		}
	}
	return lines
}

func commandFields(s string) []string {
	var fields []string
	var b strings.Builder
	inQuote := false
	escaped := false
	for _, r := range s {
		switch {
		case escaped:
			b.WriteRune(r)
			escaped = false
		case r == '\\' && inQuote:
			escaped = true
		case r == '"':
			inQuote = !inQuote
		case (r == ' ' || r == '\t' || r == '\n') && !inQuote:
			if b.Len() > 0 {
				fields = append(fields, b.String())
				b.Reset()
			}
		default:
			b.WriteRune(r)
		}
	}
	if b.Len() > 0 {
		fields = append(fields, b.String())
	}
	return fields
}

func assertNoInstructionOrHookSurface(t *testing.T, out string, red func(string) string) {
	t.Helper()
	for _, token := range []string{".ratchet/hooks.yaml", "AGENTS.md", "CLAUDE.md", ".codex"} {
		if strings.Contains(out, token) {
			t.Fatalf("runtime output leaked instruction/hook surface %q:\n%s", token, red(out))
		}
	}
}
