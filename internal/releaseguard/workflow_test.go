package releaseguard

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

type workflowFile struct {
	Jobs map[string]workflowJob `yaml:"jobs"`
}

type workflowJob struct {
	RunsOn string         `yaml:"runs-on"`
	Needs  any            `yaml:"needs"`
	Steps  []workflowStep `yaml:"steps"`
}

type workflowStep struct {
	Name string            `yaml:"name"`
	Uses string            `yaml:"uses"`
	Run  string            `yaml:"run"`
	With map[string]string `yaml:"with"`
	Env  map[string]string `yaml:"env"`
}

func TestCIReleaseCheckJob(t *testing.T) {
	workflow := loadWorkflow(t, ".github/workflows/ci.yml")
	job := requireJob(t, workflow, "release-check")
	if job.RunsOn != "ubuntu-latest" {
		t.Fatalf("release-check runs-on = %q, want ubuntu-latest", job.RunsOn)
	}
	requireStep(t, job, func(step workflowStep) bool {
		return step.Uses == "actions/checkout@v4" && step.With["fetch-depth"] == "0"
	}, "checkout with fetch-depth 0")
	requireStep(t, job, func(step workflowStep) bool {
		return step.Uses == "actions/setup-go@v5" && step.With["go-version"] == "1.26"
	}, "setup-go 1.26")
	requireRun(t, job, "Configure Git for private modules", `url."https://${{ secrets.GITHUB_TOKEN }}@github.com/".insteadOf`)
	requireGoReleaserStep(t, job, "check")
	requireGoReleaserStep(t, job, "release --snapshot --clean --skip=publish")
	requireRun(t, job, "Check release artifacts", "scripts/check-release-artifacts.sh --manifest-only dist")
	requireStep(t, job, func(step workflowStep) bool {
		return step.Uses == "actions/upload-artifact@v4" &&
			step.With["name"] == "ratchet-snapshot-dist" &&
			step.With["path"] == "dist"
	}, "upload ratchet-snapshot-dist artifact")
}

func TestCIWindowsBuildUsesRunnerTemp(t *testing.T) {
	raw := loadWorkflowRaw(t, ".github/workflows/ci.yml")
	if strings.Contains(raw, "/tmp/ratchet-windows-") {
		t.Fatalf("ci workflow must not write fixed /tmp ratchet windows outputs")
	}
	workflow := parseWorkflow(t, raw)
	job := requireJob(t, workflow, "windows-build")
	requireRun(t, job, "Build Windows binaries", "$RUNNER_TEMP")
	requireRun(t, job, "Build Windows binaries", "GOOS=windows GOARCH=amd64")
	requireRun(t, job, "Build Windows binaries", "GOOS=windows GOARCH=arm64")
}

func TestCITUISmokeAndTapPreflightJobs(t *testing.T) {
	workflow := loadWorkflow(t, ".github/workflows/ci.yml")

	smoke := requireJob(t, workflow, "tui-smoke")
	requireRun(t, smoke, "Run untagged smoke tests", "go test ./cmd/ratchet ./internal/tui -run 'HarnessSmoke|TUIBinarySmoke|StartupSmoke' -count=1 -timeout=10m")
	requireRun(t, smoke, "Run tagged client smoke tests", "go test -tags tui_smoke ./internal/client -run 'ConnectSmokeUnix' -count=1")
	requireRun(t, smoke, "Run tagged daemon smoke tests", "go test -tags tui_smoke ./internal/daemon -run 'SmokeService|ListJobs' -count=1")

	windowsSmoke := requireJob(t, workflow, "windows-conpty-smoke")
	if windowsSmoke.RunsOn != "windows-2025" {
		t.Fatalf("windows-conpty-smoke runs-on = %q, want windows-2025", windowsSmoke.RunsOn)
	}
	requireRun(t, windowsSmoke, "Run Windows ConPTY smoke tests", "go test -tags tui_smoke ./internal/client ./internal/daemon ./internal/tui")
	requireRun(t, windowsSmoke, "Run Windows ConPTY smoke tests", "WindowsConPTY")

	tap := requireJob(t, workflow, "tap-preflight")
	requireRun(t, tap, "Clone Homebrew tap", "gh repo clone GoCodeAlone/homebrew-tap")
	requireRun(t, tap, "Run tap preflight", "RATCHET_RELEASE_GUARD_MODE=tap-preflight")
	requireRun(t, tap, "Run tap preflight", "RATCHET_RELEASE_GUARD_TAP=")
	requireRun(t, tap, "Run tap preflight", "go test -count=1 ./internal/releaseguard -run TestTapPreflight")
}

func TestReleaseWorkflowPrePublishGuards(t *testing.T) {
	workflow := loadWorkflow(t, ".github/workflows/release.yml")
	job := requireJob(t, workflow, "release")
	requireRun(t, job, "Configure Git for private modules", `url."https://${{ secrets.GITHUB_TOKEN }}@github.com/".insteadOf`)
	requireGoReleaserStep(t, job, "check")
	requireGoReleaserStep(t, job, "release --snapshot --clean --skip=publish")
	requireRun(t, job, "Check snapshot release artifacts", "scripts/check-release-artifacts.sh --manifest-only dist")
	requireRun(t, job, "Check draft release config", "go test -count=1 ./internal/releaseguard -run TestGoReleaserReleaseDraftConfig")
	requireRun(t, job, "Run tap preflight", "RATCHET_RELEASE_GUARD_MODE=tap-preflight")
	requireRun(t, job, "Run tap preflight", "go test -count=1 ./internal/releaseguard -run TestTapPreflight")
	requireStep(t, job, func(step workflowStep) bool {
		return step.Name == "Publish GitHub draft with GoReleaser" &&
			step.Uses == "goreleaser/goreleaser-action@v7" &&
			step.With["args"] == "release --clean" &&
			step.Env["HOMEBREW_TAP_TOKEN"] == ""
	}, "GoReleaser draft publish without HOMEBREW_TAP_TOKEN")

	raw := loadWorkflowRaw(t, ".github/workflows/release.yml")
	requireTextOrder(t, raw, "args: check", "args: release --snapshot --clean --skip=publish")
	requireTextOrder(t, raw, "args: release --snapshot --clean --skip=publish", "Check snapshot release artifacts")
	requireTextOrder(t, raw, "Check draft release config", "Publish GitHub draft with GoReleaser")
	requireTextOrder(t, raw, "Run tap preflight", "Publish GitHub draft with GoReleaser")
}

func TestReleaseWorkflowPostPublishGuards(t *testing.T) {
	raw := loadWorkflowRaw(t, ".github/workflows/release.yml")
	workflow := parseWorkflow(t, raw)
	job := requireJob(t, workflow, "release")

	requireRun(t, job, "Resolve draft release", "draft")
	requireRun(t, job, "Download draft release assets", "metadata.json")
	requireRun(t, job, "Check draft release assets", "RATCHET_RELEASE_GUARD_MODE=draft-assets")
	requireRun(t, job, "Check draft release assets", "RATCHET_RELEASE_GUARD_ASSETS=\"$RUNNER_TEMP/release-assets\"")
	requireRun(t, job, "Check draft release assets", "go test -count=1 ./internal/releaseguard -run TestDraftAssets")
	requireRun(t, job, "Render compatibility Homebrew formula", "scripts/render-homebrew-formula.sh dist dist/homebrew/Formula/ratchet-cli.rb")
	requireRun(t, job, "Clone Homebrew tap", "HOMEBREW_TAP_TOKEN")
	requireRun(t, job, "Publish generated Homebrew tap files", "scripts/publish-homebrew-cask.sh --push dist/homebrew/Casks/ratchet-cli.rb dist/homebrew/Formula/ratchet-cli.rb")
	requireRun(t, job, "Check tap post-publish state", "RATCHET_RELEASE_GUARD_MODE=tap-postcheck")
	requireRun(t, job, "Check tap post-publish state", "RATCHET_RELEASE_GUARD_TAP_NAMES=ratchet-cli")
	requireRun(t, job, "Check tap post-publish state", "RATCHET_RELEASE_GUARD_TAP_COMMITS=")
	requireRun(t, job, "Check tap post-publish state", "go test -count=1 ./internal/releaseguard -run TestTapPostcheck")
	requireRun(t, job, "Publish GitHub release", "updateRelease")

	requireTextOrder(t, raw, "Publish GitHub draft with GoReleaser", "Check draft release assets")
	requireTextOrder(t, raw, "Check draft release assets", "Render compatibility Homebrew formula")
	requireTextOrder(t, raw, "Render compatibility Homebrew formula", "Publish generated Homebrew tap files")
	requireTextOrder(t, raw, "Publish generated Homebrew tap files", "Check tap post-publish state")
	requireTextOrder(t, raw, "Check tap post-publish state", "Publish GitHub release")
}

func TestWorkflowsAvoidStaleWindowsRunnerAndReleaseExeExecution(t *testing.T) {
	for _, rel := range []string{".github/workflows/ci.yml", ".github/workflows/release.yml"} {
		raw := loadWorkflowRaw(t, rel)
		if strings.Contains(raw, "windows-latest") {
			t.Fatalf("%s must pin concrete Windows images instead of windows-latest", rel)
		}
	}
	raw := loadWorkflowRaw(t, ".github/workflows/release.yml")
	if strings.Contains(raw, "windows-2025") {
		t.Fatalf("release workflow must not add a Windows runner")
	}
	if strings.Contains(raw, "./ratchet.exe") || strings.Contains(raw, " ratchet.exe") {
		t.Fatalf("release workflow must not execute ratchet.exe")
	}
}

func loadWorkflow(t *testing.T, rel string) workflowFile {
	t.Helper()
	return parseWorkflow(t, loadWorkflowRaw(t, rel))
}

func loadWorkflowRaw(t *testing.T, rel string) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(repoRoot(t), rel))
	if err != nil {
		t.Fatalf("read workflow %s: %v", rel, err)
	}
	return string(data)
}

func parseWorkflow(t *testing.T, raw string) workflowFile {
	t.Helper()
	var workflow workflowFile
	if err := yaml.Unmarshal([]byte(raw), &workflow); err != nil {
		t.Fatalf("parse workflow: %v", err)
	}
	return workflow
}

func requireJob(t *testing.T, workflow workflowFile, name string) workflowJob {
	t.Helper()
	job, ok := workflow.Jobs[name]
	if !ok {
		t.Fatalf("workflow missing job %q", name)
	}
	return job
}

func requireRun(t *testing.T, job workflowJob, name, contains string) {
	t.Helper()
	requireStep(t, job, func(step workflowStep) bool {
		return step.Name == name && (strings.Contains(step.Run, contains) || strings.Contains(step.With["script"], contains))
	}, name+" containing "+contains)
}

func requireGoReleaserStep(t *testing.T, job workflowJob, args string) {
	t.Helper()
	requireStep(t, job, func(step workflowStep) bool {
		return step.Uses == "goreleaser/goreleaser-action@v7" &&
			step.With["version"] == "~> v2" &&
			step.With["args"] == args
	}, "GoReleaser action args "+args)
}

func requireStep(t *testing.T, job workflowJob, match func(workflowStep) bool, description string) {
	t.Helper()
	for _, step := range job.Steps {
		if match(step) {
			return
		}
	}
	t.Fatalf("workflow job missing step: %s", description)
}

func requireTextOrder(t *testing.T, text, before, after string) {
	t.Helper()
	beforeIndex := strings.Index(text, before)
	if beforeIndex < 0 {
		t.Fatalf("workflow missing %q", before)
	}
	afterIndex := strings.Index(text, after)
	if afterIndex < 0 {
		t.Fatalf("workflow missing %q", after)
	}
	if beforeIndex >= afterIndex {
		t.Fatalf("workflow has %q after %q", before, after)
	}
}
