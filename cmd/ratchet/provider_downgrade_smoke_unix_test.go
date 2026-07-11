//go:build !windows

package main

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/pem"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/GoCodeAlone/ratchet-cli/internal/client"
	"github.com/GoCodeAlone/ratchet-cli/internal/daemon"
	"github.com/GoCodeAlone/ratchet-cli/internal/harnessredact"
	pb "github.com/GoCodeAlone/ratchet-cli/internal/proto"
	"github.com/GoCodeAlone/workflow/secrets"
	"golang.org/x/sys/unix"
	"google.golang.org/protobuf/encoding/protojson"
	_ "modernc.org/sqlite"
)

const (
	downgradeProviderAlias = "durable-downgrade-smoke"
	downgradeProviderModel = "fixture-model"
)

func TestHarnessSmokeDurableProviderDowngrade(t *testing.T) {
	baseRevision := strings.TrimSpace(os.Getenv("RATCHET_DOWNGRADE_BASE_SHA"))
	if baseRevision == "" {
		t.Skip("set RATCHET_DOWNGRADE_BASE_SHA to opt in to the mixed-version provider durability smoke")
	}
	if raceEnabled {
		t.Skip("mixed-version provider durability smoke is disabled under -race")
	}

	workingDir, err := os.Getwd()
	if err != nil {
		t.Fatal("resolve downgrade working directory: failure=getwd")
	}
	preflightRed := harnessredact.New(workingDir).String
	repoRoot := downgradeGitOutput(t, preflightRed, "rev-parse", "--show-toplevel")
	preflightRed = harnessredact.New(workingDir, repoRoot).String
	currentRevision := downgradeGitOutput(t, preflightRed, "rev-parse", "HEAD")
	baseRevision = downgradeGitOutput(t, preflightRed, "rev-parse", "--verify", baseRevision+"^{commit}")
	t.Logf("provider downgrade revisions base=%s current=%s", baseRevision, currentRevision)
	if baseRevision == currentRevision {
		t.Fatal("provider downgrade revisions must differ")
	}
	baseProto := downgradeGitOutput(t, preflightRed, "show", baseRevision+":internal/proto/ratchet.proto")
	if strings.Contains(baseProto, "CommitProviderSave") {
		t.Fatal("provider downgrade base already contains the durable save RPC")
	}
	baseWorktree := filepath.Join(t.TempDir(), "base-worktree")
	currentBin := filepath.Join(t.TempDir(), "ratchet-current")
	baseBin := filepath.Join(t.TempDir(), "ratchet-base")
	buildRed := harnessredact.New(workingDir, repoRoot, baseWorktree, currentBin, baseBin).String
	addDowngradeWorktree(t, repoRoot, baseRevision, baseWorktree, buildRed)
	setDowngradeAgentDependency(t, baseWorktree, buildRed)
	buildDowngradeRatchetBinary(t, repoRoot, currentBin, "current", buildRed)
	buildDowngradeRatchetBinary(t, baseWorktree, baseBin, "base", buildRed)
	if runtime.GOOS == "darwin" {
		t.Skip("macOS system verification ignores SSL_CERT_FILE; parent/current builds passed and Linux CI owns the hermetic TLS fixture proof")
	}

	const (
		sentinel1      = "DOWNGRADE-CURRENT-V2-SENTINEL"
		sentinel2      = "DOWNGRADE-PARENT-LEGACY-SENTINEL"
		sentinel3      = "DOWNGRADE-FINAL-V2-SENTINEL"
		unrelatedKey   = "downgrade-unrelated"
		unrelatedValue = "DOWNGRADE-UNRELATED-VALUE"
	)
	var expectedAuthorization atomic.Value
	expectedAuthorization.Store("Bearer " + sentinel1)
	fixtureWriteFailed := make(chan struct{}, 1)
	fixture := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || (r.URL.Path != "/chat/completions" && r.URL.Path != "/v1/chat/completions") {
			http.NotFound(w, r)
			return
		}
		if r.Header.Get("Authorization") != expectedAuthorization.Load().(string) {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		if _, err := w.Write([]byte(`{"id":"chatcmpl-downgrade","object":"chat.completion","model":"fixture-model","choices":[{"index":0,"message":{"role":"assistant","content":"provider downgrade ok"},"finish_reason":"stop"}],"usage":{"prompt_tokens":1,"completion_tokens":1,"total_tokens":2}}`)); err != nil {
			select {
			case fixtureWriteFailed <- struct{}{}:
			default:
			}
		}
	}))
	t.Cleanup(func() {
		select {
		case <-fixtureWriteFailed:
			t.Error("provider downgrade fixture response write failed")
		default:
		}
	})
	t.Cleanup(fixture.Close)
	proxy := newHarnessProviderConnectProxy(t, fixture.Listener.Addr().String())
	t.Cleanup(proxy.Close)

	root := t.TempDir()
	home := filepath.Join(root, "home")
	state := filepath.Join(home, ".local", "state")
	work := filepath.Join(root, "work")
	dataDir := filepath.Join(home, ".ratchet")
	secretsDir := filepath.Join(dataDir, "secrets")
	certFile := filepath.Join(root, "provider-fixture-ca.pem")
	for _, dir := range []string{home, state, work, dataDir, secretsDir} {
		if err := os.MkdirAll(dir, 0o700); err != nil {
			t.Fatalf("create downgrade harness directory category: %v", err)
		}
	}
	certificate := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: fixture.Certificate().Raw})
	if err := os.WriteFile(certFile, certificate, 0o600); err != nil {
		t.Fatalf("write downgrade fixture certificate: %v", err)
	}
	providerBaseURL := "https://example.com"
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	t.Setenv("XDG_STATE_HOME", state)
	t.Setenv("SSL_CERT_FILE", certFile)
	t.Setenv("HTTPS_PROXY", proxy.URL)
	t.Setenv("NO_PROXY", "")
	env := []string{
		"HOME=" + home,
		"USERPROFILE=" + home,
		"XDG_STATE_HOME=" + state,
		"SSL_CERT_FILE=" + certFile,
		"HTTPS_PROXY=" + proxy.URL,
		"NO_PROXY=",
	}
	red := harnessredact.New(
		home, state, work, root, dataDir, secretsDir, certFile, fixture.URL, proxy.URL,
		providerBaseURL, currentBin, baseBin, baseWorktree, daemon.SocketPath(), daemon.PIDPath(),
		sentinel1, sentinel2, sentinel3, unrelatedKey, unrelatedValue,
	).String
	assertOutputRedacted := func(output string) {
		assertDowngradeOutputRedacted(t, output, sentinel1, sentinel2, sentinel3, unrelatedValue)
	}
	t.Cleanup(func() { stopDowngradeDaemonBestEffort() })

	secretStore := secrets.NewFileProvider(secretsDir)
	if err := secretStore.Set(t.Context(), unrelatedKey, unrelatedValue); err != nil {
		t.Fatal("seed unrelated secret category: failure=set")
	}
	baseline := snapshotDowngradeUnrelated(t, secretStore, downgradeProviderAlias)

	firstOutput, firstOperationID := runCurrentDowngradeSave(
		t, currentBin, work, env, red, sentinel1, providerBaseURL,
	)
	assertOutputRedacted(firstOutput)
	waitForDowngradeArtifacts(t, true)
	assertDowngradeDaemonLockHeld(t, filepath.Join(dataDir, "daemon.lock"))
	firstStatus := runDowngradeRatchet(t, currentBin, work, env, red,
		"provider", "operation", firstOperationID, "--json")
	assertDowngradeCommittedOperation(t, firstStatus, firstOperationID, red)
	assertOutputRedacted(firstStatus)

	db, err := sql.Open("sqlite", daemon.DBPath()+"?_pragma=busy_timeout(5000)")
	if err != nil {
		t.Fatalf("open downgrade database: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	firstPointer := queryDowngradeProviderPointer(t, db)
	if !strings.HasPrefix(firstPointer, "provider-v2-") {
		t.Fatal("initial provider pointer category is not v2")
	}
	assertDowngradeOperationState(t, db, firstOperationID, "committed")
	assertDowngradeSecretValue(t, secretStore, firstPointer, sentinel1, "active-v2")
	assertDowngradeInventory(t, secretStore, baseline, downgradeProviderAlias, firstPointer, true, false, 1)
	assertDowngradeCleanupRows(t, db, 0)
	currentTest := runDowngradeRatchet(t, currentBin, work, env, red, "provider", "test", downgradeProviderAlias)
	assertDowngradeProviderTest(t, currentTest)
	assertOutputRedacted(currentTest)

	stopDowngradeDaemon(t)
	waitForDowngradeArtifacts(t, false)
	assertDowngradeDaemonLockAvailable(t, filepath.Join(dataDir, "daemon.lock"))

	parentTestV2 := runDowngradeRatchet(t, baseBin, work, env, red, "provider", "test", downgradeProviderAlias)
	waitForDowngradeArtifacts(t, true)
	assertDowngradeProviderTest(t, parentTestV2)
	assertOutputRedacted(parentTestV2)
	parentStatus := runDowngradeRatchet(t, baseBin, work, env, red, "daemon", "status")
	assertOutputRedacted(parentStatus)

	parentAdd := runParentDowngradeSave(t, baseBin, work, env, red, sentinel2, providerBaseURL)
	assertOutputRedacted(parentAdd)
	expectedAuthorization.Store("Bearer " + sentinel2)
	legacyPointer := "provider_" + downgradeProviderAlias
	if queryDowngradeProviderPointer(t, db) != legacyPointer {
		t.Fatal("parent provider pointer category is not the exact legacy key")
	}
	assertDowngradeSecretValue(t, secretStore, legacyPointer, sentinel2, "active-legacy")
	assertDowngradeInventory(t, secretStore, baseline, downgradeProviderAlias, firstPointer, true, true, 2)
	assertDowngradeCleanupRows(t, db, 0)
	parentTestLegacy := runDowngradeRatchet(t, baseBin, work, env, red, "provider", "test", downgradeProviderAlias)
	assertDowngradeProviderTest(t, parentTestLegacy)
	assertOutputRedacted(parentTestLegacy)

	stopDowngradeDaemon(t)
	waitForDowngradeArtifacts(t, false)
	assertDowngradeDaemonLockAvailable(t, filepath.Join(dataDir, "daemon.lock"))

	if _, err := db.Exec(`CREATE TRIGGER hold_downgrade_provider_cleanup
		AFTER INSERT ON provider_secret_cleanup
		BEGIN
			UPDATE provider_secret_cleanup
			SET next_attempt_at = datetime('now', '+1 day')
			WHERE secret_name = NEW.secret_name;
		END`); err != nil {
		t.Fatal("install downgrade cleanup journal gate: failure=database")
	}
	t.Cleanup(func() { _, _ = db.Exec(`DROP TRIGGER IF EXISTS hold_downgrade_provider_cleanup`) })
	currentStart := runDowngradeRatchet(t, currentBin, work, env, red, "daemon", "start", "--background")
	assertOutputRedacted(currentStart)
	waitForDowngradeArtifacts(t, true)
	assertDowngradeDaemonLockHeld(t, filepath.Join(dataDir, "daemon.lock"))
	currentLegacyTest := runDowngradeRatchet(t, currentBin, work, env, red, "provider", "test", downgradeProviderAlias)
	assertDowngradeProviderTest(t, currentLegacyTest)
	assertOutputRedacted(currentLegacyTest)
	waitForDowngradeCleanupQueued(t, db, secretStore, firstPointer)
	if _, err := db.Exec(`DROP TRIGGER hold_downgrade_provider_cleanup`); err != nil {
		t.Fatal("release downgrade cleanup journal gate: failure=database")
	}
	if _, err := db.Exec(`UPDATE provider_secret_cleanup SET next_attempt_at = CURRENT_TIMESTAMP WHERE secret_name = ?`, firstPointer); err != nil {
		t.Fatal("schedule downgrade cleanup retry: failure=database")
	}
	waitForDowngradeSecretConvergence(t, db, secretStore, baseline, downgradeProviderAlias, legacyPointer, "legacy", 0, 1)
	assertDowngradeOperationState(t, db, firstOperationID, "committed")
	replayedFirstStatus := runDowngradeRatchet(t, currentBin, work, env, red,
		"provider", "operation", firstOperationID, "--json")
	assertDowngradeCommittedOperation(t, replayedFirstStatus, firstOperationID, red)
	assertOutputRedacted(replayedFirstStatus)

	finalOutput, finalOperationID := runCurrentDowngradeSave(
		t, currentBin, work, env, red, sentinel3, providerBaseURL,
	)
	assertOutputRedacted(finalOutput)
	expectedAuthorization.Store("Bearer " + sentinel3)
	finalStatus := runDowngradeRatchet(t, currentBin, work, env, red,
		"provider", "operation", finalOperationID, "--json")
	assertDowngradeCommittedOperation(t, finalStatus, finalOperationID, red)
	assertOutputRedacted(finalStatus)
	finalPointer := queryDowngradeProviderPointer(t, db)
	if !strings.HasPrefix(finalPointer, "provider-v2-") || finalPointer == firstPointer {
		t.Fatal("final provider pointer category is not a distinct v2 key")
	}
	waitForDowngradeSecretConvergence(t, db, secretStore, baseline, downgradeProviderAlias, finalPointer, "v2", 1, 0)
	assertDowngradeSecretValue(t, secretStore, finalPointer, sentinel3, "active-v2")
	assertDowngradeOperationState(t, db, finalOperationID, "committed")
	finalTest := runDowngradeRatchet(t, currentBin, work, env, red, "provider", "test", downgradeProviderAlias)
	assertDowngradeProviderTest(t, finalTest)
	assertOutputRedacted(finalTest)

	stopDowngradeDaemon(t)
	waitForDowngradeArtifacts(t, false)
	assertDowngradeDaemonLockAvailable(t, filepath.Join(dataDir, "daemon.lock"))
}

func addDowngradeWorktree(t *testing.T, repoRoot, revision, worktree string, red func(string) string) {
	t.Helper()
	t.Cleanup(func() {
		removeCtx, removeCancel := context.WithTimeout(context.Background(), 30*time.Second)
		remove := exec.CommandContext(removeCtx, "git", "-C", repoRoot, "worktree", "remove", "--force", worktree)
		_, removeErr := remove.CombinedOutput()
		removeCancel()
		if removeErr != nil {
			if err := os.RemoveAll(worktree); err != nil {
				t.Errorf("remove downgrade worktree path: %s", red(err.Error()))
			}
		}
		pruneCtx, pruneCancel := context.WithTimeout(context.Background(), 30*time.Second)
		prune := exec.CommandContext(pruneCtx, "git", "-C", repoRoot, "worktree", "prune")
		if output, err := prune.CombinedOutput(); err != nil {
			t.Errorf("prune downgrade worktree metadata: %s\n%s", red(err.Error()), red(string(output)))
		}
		pruneCancel()
		listCtx, listCancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer listCancel()
		list := exec.CommandContext(listCtx, "git", "-C", repoRoot, "worktree", "list", "--porcelain")
		output, err := list.CombinedOutput()
		if err != nil {
			t.Errorf("verify downgrade worktree cleanup: %s\n%s", red(err.Error()), red(string(output)))
		} else if strings.Contains(string(output), "worktree "+worktree+"\n") {
			t.Error("downgrade worktree registration remained after cleanup")
		}
	})
	cmd := exec.CommandContext(t.Context(), "git", "-C", repoRoot, "worktree", "add", "--detach", worktree, revision)
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("add downgrade base worktree: %s\n%s", red(err.Error()), red(string(output)))
	}
}

func setDowngradeAgentDependency(t *testing.T, sourceDir string, red func(string) string) {
	t.Helper()
	for _, args := range [][]string{
		{"mod", "edit", "-require=github.com/GoCodeAlone/workflow-plugin-agent@v0.12.8"},
		{"mod", "download", "github.com/GoCodeAlone/workflow-plugin-agent@v0.12.8"},
	} {
		ctx, cancel := context.WithTimeout(t.Context(), 2*time.Minute)
		cmd := exec.CommandContext(ctx, "go", args...)
		cmd.Dir = sourceDir
		output, err := cmd.CombinedOutput()
		cancel()
		if err != nil {
			t.Fatalf("prepare downgrade parent dependency: %s\n%s", red(err.Error()), red(string(output)))
		}
	}
	ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "go", "list", "-m", "-f", "{{.Version}}", "github.com/GoCodeAlone/workflow-plugin-agent")
	cmd.Dir = sourceDir
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("verify downgrade parent dependency: %s\n%s", red(err.Error()), red(string(output)))
	}
	if strings.TrimSpace(string(output)) != "v0.12.8" {
		t.Fatal("downgrade parent did not consume released provider adapter fix")
	}
}

func buildDowngradeRatchetBinary(t *testing.T, sourceDir, bin, label string, red func(string) string) {
	t.Helper()
	ctx, cancel := context.WithTimeout(t.Context(), 3*time.Minute)
	defer cancel()
	cmd := exec.CommandContext(ctx, "go", "build", "-o", bin, "./cmd/ratchet")
	cmd.Dir = sourceDir
	var output bytes.Buffer
	cmd.Stdout = &output
	cmd.Stderr = &output
	if err := cmd.Run(); err != nil {
		t.Fatalf("build downgrade binary category %s: %s\n%s", label, red(err.Error()), red(output.String()))
	}
}

func downgradeGitOutput(t *testing.T, red func(string) string, args ...string) string {
	t.Helper()
	ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "git", args...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git downgrade prerequisite failed: %s\n%s", red(err.Error()), red(string(output)))
	}
	return strings.TrimSpace(string(output))
}

func runCurrentDowngradeSave(t *testing.T, bin, work string, env []string, red func(string) string, secret, baseURL string) (string, string) {
	t.Helper()
	s := startRatchetSmokePTY(t, bin, work, env, red,
		"provider", "add", "custom", downgradeProviderAlias, "--model", downgradeProviderModel, "--json")
	s.waitFor("API key", 10*time.Second)
	s.sendLine(secret)
	s.waitFor("API compatibility", 10*time.Second)
	s.sendLine("")
	s.waitFor("Base URL", 10*time.Second)
	s.sendLine(baseURL)
	output := s.waitExit(30 * time.Second)
	return output, providerOperationIDFromSmokeOutput(t, output, red)
}

func runParentDowngradeSave(t *testing.T, bin, work string, env []string, red func(string) string, secret, baseURL string) string {
	t.Helper()
	s := startRatchetSmokePTY(t, bin, work, env, red,
		"provider", "add", "custom", downgradeProviderAlias, "--model", downgradeProviderModel)
	s.waitFor("API compatibility", 10*time.Second)
	s.sendLine("")
	s.waitFor("API key", 10*time.Second)
	s.sendLine(secret)
	s.waitFor("Base URL", 10*time.Second)
	s.sendLine(baseURL)
	return s.waitExit(30 * time.Second)
}

func runDowngradeRatchet(t *testing.T, bin, work string, env []string, red func(string) string, args ...string) string {
	t.Helper()
	ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, bin, args...)
	cmd.Dir = work
	cmd.Env = ratchetSmokeEnv(env, work)
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("downgrade subprocess failed: %s\n%s", red(err.Error()), red(string(output)))
	}
	return string(output)
}

func stopDowngradeDaemon(t *testing.T) {
	t.Helper()
	c, err := client.Connect()
	if err != nil {
		t.Fatalf("connect to downgrade daemon: failure=connect")
	}
	defer c.Close()
	ctx, cancel := context.WithTimeout(t.Context(), 3*time.Second)
	defer cancel()
	if err := c.Shutdown(ctx); err != nil {
		t.Fatalf("stop downgrade daemon: failure=rpc")
	}
}

func stopDowngradeDaemonBestEffort() {
	if _, err := os.Stat(daemon.SocketPath()); err != nil {
		return
	}
	c, err := client.Connect()
	if err != nil {
		return
	}
	defer c.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	_ = c.Shutdown(ctx)
}

type downgradeConvergence struct {
	done            bool
	state           string
	failureClass    string
	v2Count         int
	legacyCount     int
	unrelatedCount  int
	cleanupRowCount int
}

func waitForDowngradeConvergence(t *testing.T, name string, timeout time.Duration, inspect func() downgradeConvergence) {
	t.Helper()
	ctx, cancel := context.WithTimeout(t.Context(), timeout)
	defer cancel()
	ticker := time.NewTicker(50 * time.Millisecond)
	defer ticker.Stop()
	last := inspect()
	for !last.done {
		select {
		case <-ctx.Done():
			t.Fatalf("downgrade convergence %s timed out: state=%s failure=%s v2_count=%d legacy_count=%d unrelated_count=%d cleanup_count=%d",
				name, last.state, last.failureClass, last.v2Count, last.legacyCount, last.unrelatedCount, last.cleanupRowCount)
		case <-ticker.C:
			last = inspect()
		}
	}
}

func waitForDowngradeArtifacts(t *testing.T, present bool) {
	t.Helper()
	wantState := "absent"
	if present {
		wantState = "present"
	}
	waitForDowngradeConvergence(t, "daemon-artifacts-"+wantState, 5*time.Second, func() downgradeConvergence {
		matches := 0
		failure := "none"
		for _, path := range []string{daemon.SocketPath(), daemon.PIDPath()} {
			_, err := os.Stat(path)
			switch {
			case present && err == nil:
				matches++
			case !present && errors.Is(err, os.ErrNotExist):
				matches++
			case err != nil && !errors.Is(err, os.ErrNotExist):
				failure = "stat"
			}
		}
		return downgradeConvergence{done: matches == 2, state: wantState, failureClass: failure}
	})
}

func waitForDowngradeSecretConvergence(
	t *testing.T,
	db *sql.DB,
	store secrets.Provider,
	baseline downgradeUnrelatedSnapshot,
	alias, pointer, pointerCategory string,
	wantV2, wantLegacy int,
) {
	t.Helper()
	waitForDowngradeConvergence(t, "provider-secret-cleanup", 10*time.Second, func() downgradeConvergence {
		inventory, inventoryOK := readDowngradeInventory(store, alias)
		rows, cleanupFailure, rowsOK := readDowngradeCleanupState(db)
		activePointer, pointerOK := readDowngradeProviderPointer(db)
		unrelatedOK := downgradeUnrelatedMatches(store, baseline, inventory.unrelated)
		pointerKindOK := (pointerCategory == "legacy" && inventory.legacyCount == 1) ||
			(pointerCategory == "v2" && slices.Contains(inventory.v2, pointer))
		done := inventoryOK && rowsOK && pointerOK && unrelatedOK && inventory.unexpectedProvider == 0 &&
			activePointer == pointer && pointerKindOK && len(inventory.v2) == wantV2 &&
			inventory.legacyCount == wantLegacy && rows == 0
		failure := cleanupFailure
		if !inventoryOK || !rowsOK || !pointerOK {
			failure = "inspection"
		} else if inventory.unexpectedProvider != 0 {
			failure = "unexpected-provider-category"
		} else if !unrelatedOK {
			failure = "unrelated-secret-drift"
		}
		return downgradeConvergence{
			done: done, state: pointerCategory, failureClass: failure,
			v2Count: len(inventory.v2), legacyCount: inventory.legacyCount,
			unrelatedCount: len(inventory.unrelated), cleanupRowCount: rows,
		}
	})
}

func waitForDowngradeCleanupQueued(t *testing.T, db *sql.DB, store secrets.Provider, secretName string) {
	t.Helper()
	waitForDowngradeConvergence(t, "provider-secret-cleanup-journal", 10*time.Second, func() downgradeConvergence {
		var attempts int
		var failure string
		err := db.QueryRow(`SELECT attempt_count, failure FROM provider_secret_cleanup WHERE secret_name = ?`, secretName).Scan(&attempts, &failure)
		_, secretErr := store.Get(t.Context(), secretName)
		return downgradeConvergence{
			done:            err == nil && attempts == 0 && failure == "" && secretErr == nil,
			state:           "queued",
			failureClass:    failure,
			cleanupRowCount: boolCount(err == nil),
		}
	})
}

func boolCount(value bool) int {
	if value {
		return 1
	}
	return 0
}

func assertDowngradeDaemonLockHeld(t *testing.T, lockPath string) {
	t.Helper()
	file, err := os.OpenFile(lockPath, os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		t.Fatal("daemon lifetime lock failure=open")
	}
	defer file.Close()
	err = unix.Flock(int(file.Fd()), unix.LOCK_EX|unix.LOCK_NB)
	if err == nil {
		_ = unix.Flock(int(file.Fd()), unix.LOCK_UN)
		t.Fatal("daemon lifetime lock state=not-held")
	}
	if !errors.Is(err, unix.EWOULDBLOCK) && !errors.Is(err, unix.EAGAIN) {
		t.Fatal("daemon lifetime lock failure=inspect")
	}
}

func assertDowngradeDaemonLockAvailable(t *testing.T, lockPath string) {
	t.Helper()
	file, err := os.OpenFile(lockPath, os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		t.Fatal("daemon lifetime lock failure=open")
	}
	defer file.Close()
	if err := unix.Flock(int(file.Fd()), unix.LOCK_EX|unix.LOCK_NB); err != nil {
		t.Fatal("daemon lifetime lock failure=acquire")
	}
	if err := unix.Flock(int(file.Fd()), unix.LOCK_UN); err != nil {
		t.Fatal("daemon lifetime lock failure=release")
	}
}

func assertDowngradeCommittedOperation(t *testing.T, output, operationID string, red func(string) string) {
	t.Helper()
	var operation pb.ProviderOperation
	if err := protojson.Unmarshal([]byte(output), &operation); err != nil {
		t.Fatalf("decode downgrade operation: failure=json\n%s", red(output))
	}
	if operation.GetOperationId() != operationID || operation.GetState() != pb.ProviderOperationState_PROVIDER_OPERATION_STATE_COMMITTED {
		t.Fatalf("downgrade operation state=%s id_match=%t", operation.GetState(), operation.GetOperationId() == operationID)
	}
}

func assertDowngradeOperationState(t *testing.T, db *sql.DB, operationID, want string) {
	t.Helper()
	var state string
	if err := db.QueryRow(`SELECT state FROM provider_operations WHERE operation_id = ?`, operationID).Scan(&state); err != nil {
		t.Fatal("query downgrade operation state: failure=database")
	}
	if state != want {
		t.Fatalf("downgrade operation state=%s", state)
	}
}

func queryDowngradeProviderPointer(t *testing.T, db *sql.DB) string {
	t.Helper()
	pointer, ok := readDowngradeProviderPointer(db)
	if !ok {
		t.Fatal("query downgrade provider pointer: failure=database")
	}
	return pointer
}

func readDowngradeProviderPointer(db *sql.DB) (string, bool) {
	var pointer string
	err := db.QueryRow(`SELECT secret_name FROM llm_providers WHERE alias = ?`, downgradeProviderAlias).Scan(&pointer)
	return pointer, err == nil
}

func assertDowngradeCleanupRows(t *testing.T, db *sql.DB, want int) {
	t.Helper()
	got, failure, ok := readDowngradeCleanupState(db)
	if !ok {
		t.Fatal("query provider cleanup rows: failure=database")
	}
	if got != want {
		t.Fatalf("provider cleanup state=queued failure=%s count=%d", failure, got)
	}
}

func readDowngradeCleanupState(db *sql.DB) (int, string, bool) {
	var count, failed int
	err := db.QueryRow(`SELECT count(*), COALESCE(sum(CASE WHEN failure != '' THEN 1 ELSE 0 END), 0)
		FROM provider_secret_cleanup`).Scan(&count, &failed)
	failureClass := "none"
	if count > 0 {
		failureClass = "queued"
	}
	if failed > 0 {
		failureClass = "retry"
	}
	return count, failureClass, err == nil
}

func assertDowngradeSecretValue(t *testing.T, store secrets.Provider, key, want, category string) {
	t.Helper()
	got, err := store.Get(t.Context(), key)
	if err != nil || got != want {
		t.Fatalf("provider secret category=%s active=%t failure=%t", category, got == want, err != nil)
	}
}

type downgradeSecretInventory struct {
	v2                 []string
	legacyCount        int
	unrelated          []string
	unexpectedProvider int
}

type downgradeUnrelatedSnapshot struct {
	keys   []string
	values map[string]string
}

func snapshotDowngradeUnrelated(t *testing.T, store secrets.Provider, alias string) downgradeUnrelatedSnapshot {
	t.Helper()
	inventory, ok := readDowngradeInventory(store, alias)
	if !ok {
		t.Fatal("snapshot unrelated secret set: failure=list")
	}
	if len(inventory.v2) != 0 || inventory.legacyCount != 0 || inventory.unexpectedProvider != 0 {
		t.Fatal("isolated secret set contains provider key categories")
	}
	values := make(map[string]string, len(inventory.unrelated))
	for _, key := range inventory.unrelated {
		value, err := store.Get(t.Context(), key)
		if err != nil {
			t.Fatal("snapshot unrelated secret value: failure=get")
		}
		values[key] = value
	}
	return downgradeUnrelatedSnapshot{keys: slices.Clone(inventory.unrelated), values: values}
}

func readDowngradeInventory(store secrets.Provider, alias string) (downgradeSecretInventory, bool) {
	keys, err := store.List(context.Background())
	if err != nil {
		return downgradeSecretInventory{}, false
	}
	legacyKey := "provider_" + alias
	var inventory downgradeSecretInventory
	for _, key := range keys {
		switch {
		case strings.HasPrefix(key, "provider-v2-"):
			inventory.v2 = append(inventory.v2, key)
		case key == legacyKey:
			inventory.legacyCount++
		case strings.HasPrefix(key, "provider_"):
			inventory.unexpectedProvider++
		default:
			inventory.unrelated = append(inventory.unrelated, key)
		}
	}
	slices.Sort(inventory.v2)
	slices.Sort(inventory.unrelated)
	return inventory, true
}

func assertDowngradeInventory(
	t *testing.T,
	store secrets.Provider,
	baseline downgradeUnrelatedSnapshot,
	alias, activeV2 string,
	wantV2, wantLegacy bool,
	wantProviderCount int,
) {
	t.Helper()
	inventory, ok := readDowngradeInventory(store, alias)
	if !ok {
		t.Fatal("provider secret inventory: failure=list")
	}
	v2Count := 0
	if wantV2 {
		v2Count = 1
	}
	legacyCount := 0
	if wantLegacy {
		legacyCount = 1
	}
	providerCount := len(inventory.v2) + inventory.legacyCount + inventory.unexpectedProvider
	if len(inventory.v2) != v2Count || inventory.legacyCount != legacyCount ||
		inventory.unexpectedProvider != 0 || providerCount != wantProviderCount ||
		(wantV2 && !slices.Contains(inventory.v2, activeV2)) {
		t.Fatalf("provider secret inventory v2_count=%d legacy_count=%d unexpected_count=%d provider_count=%d",
			len(inventory.v2), inventory.legacyCount, inventory.unexpectedProvider, providerCount)
	}
	if !downgradeUnrelatedMatches(store, baseline, inventory.unrelated) {
		t.Fatalf("unrelated secret inventory state=drift count=%d", len(inventory.unrelated))
	}
}

func downgradeUnrelatedMatches(store secrets.Provider, baseline downgradeUnrelatedSnapshot, gotKeys []string) bool {
	if !slices.Equal(gotKeys, baseline.keys) {
		return false
	}
	for _, key := range baseline.keys {
		value, err := store.Get(context.Background(), key)
		if err != nil || value != baseline.values[key] {
			return false
		}
	}
	return true
}

func assertDowngradeProviderTest(t *testing.T, output string) {
	t.Helper()
	if !strings.HasPrefix(strings.TrimSpace(output), "OK (") {
		t.Fatal("provider resolution state=failed")
	}
}

func assertDowngradeOutputRedacted(t *testing.T, output string, sentinels ...string) {
	t.Helper()
	for _, sentinel := range sentinels {
		if strings.Contains(output, sentinel) {
			t.Fatal("downgrade subprocess output exposed credential category")
		}
	}
}
