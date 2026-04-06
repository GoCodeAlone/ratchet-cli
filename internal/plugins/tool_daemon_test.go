package plugins

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"
)

// buildTestDaemon compiles the testdata daemon helper and returns its path.
func buildTestDaemon(t *testing.T) string {
	t.Helper()

	// Write the daemon source into a temp dir and compile it.
	src := `package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
)

type request struct {
	JSONRPC string          ` + "`json:\"jsonrpc\"`" + `
	ID      int             ` + "`json:\"id\"`" + `
	Method  string          ` + "`json:\"method\"`" + `
	Params  json.RawMessage ` + "`json:\"params,omitempty\"`" + `
}

type response struct {
	JSONRPC string ` + "`json:\"jsonrpc\"`" + `
	ID      int    ` + "`json:\"id\"`" + `
	Result  any    ` + "`json:\"result,omitempty\"`" + `
}

func main() {
	scanner := bufio.NewScanner(os.Stdin)
	enc := json.NewEncoder(os.Stdout)

	for scanner.Scan() {
		var req request
		if err := json.Unmarshal(scanner.Bytes(), &req); err != nil {
			fmt.Fprintf(os.Stderr, "parse error: %v\n", err)
			continue
		}

		switch req.Method {
		case "initialize":
			enc.Encode(response{
				JSONRPC: "2.0",
				ID:      req.ID,
				Result: map[string]any{
					"protocol": "daemon",
					"tools": []map[string]any{
						{
							"name":        "greet",
							"description": "returns a greeting",
							"protocol":    "daemon",
							"parameters": map[string]any{
								"type": "object",
								"properties": map[string]any{
									"name": map[string]any{"type": "string"},
								},
							},
						},
					},
				},
			})
		case "call":
			var params struct {
				Name      string         ` + "`json:\"name\"`" + `
				Arguments map[string]any ` + "`json:\"arguments\"`" + `
			}
			json.Unmarshal(req.Params, &params)
			var greeting string
			if n, ok := params.Arguments["name"].(string); ok && n != "" {
				greeting = "Hello, " + n + "!"
			} else {
				greeting = "Hello!"
			}
			enc.Encode(response{
				JSONRPC: "2.0",
				ID:      req.ID,
				Result:  map[string]any{"greeting": greeting},
			})
		default:
			fmt.Fprintf(os.Stderr, "unknown method: %s\n", req.Method)
		}
	}
}
`
	srcDir := t.TempDir()
	srcFile := filepath.Join(srcDir, "main.go")
	if err := os.WriteFile(srcFile, []byte(src), 0644); err != nil {
		t.Fatalf("write daemon source: %v", err)
	}

	binName := "test_daemon"
	if runtime.GOOS == "windows" {
		binName += ".exe"
	}
	binPath := filepath.Join(srcDir, binName)

	cmd := exec.Command("go", "build", "-o", binPath, srcFile)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("compile test daemon: %v\n%s", err, out)
	}
	return binPath
}

func TestDaemonTool_LifecycleAndCall(t *testing.T) {
	binPath := buildTestDaemon(t)

	ctx := context.Background()
	daemon, err := StartDaemon(ctx, binPath)
	if err != nil {
		t.Fatalf("StartDaemon: %v", err)
	}
	t.Cleanup(func() { _ = daemon.Stop() })

	// Verify defs discovered during initialize.
	defs := daemon.Defs()
	if len(defs) != 1 {
		t.Fatalf("expected 1 tool def, got %d", len(defs))
	}
	if defs[0].Name != "greet" {
		t.Errorf("defs[0].Name = %q, want greet", defs[0].Name)
	}

	// Call the greet tool.
	result, err := daemon.Call(ctx, "greet", map[string]any{"name": "World"})
	if err != nil {
		t.Fatalf("Call: %v", err)
	}
	got, ok := result.(map[string]any)
	if !ok {
		t.Fatalf("result type = %T", result)
	}
	if got["greeting"] != "Hello, World!" {
		t.Errorf("greeting = %v, want Hello, World!", got["greeting"])
	}
}

func TestDaemonTool_MultipleCallsSerialised(t *testing.T) {
	binPath := buildTestDaemon(t)

	daemon, err := StartDaemon(context.Background(), binPath)
	if err != nil {
		t.Fatalf("StartDaemon: %v", err)
	}
	t.Cleanup(func() { _ = daemon.Stop() })

	for i := 0; i < 5; i++ {
		name := fmt.Sprintf("User%d", i)
		res, err := daemon.Call(context.Background(), "greet", map[string]any{"name": name})
		if err != nil {
			t.Fatalf("Call %d: %v", i, err)
		}
		got := res.(map[string]any)
		want := "Hello, " + name + "!"
		if got["greeting"] != want {
			t.Errorf("call %d: greeting = %v, want %s", i, got["greeting"], want)
		}
	}
}

func TestDaemonToolRef_ImplementsInterface(t *testing.T) {
	binPath := buildTestDaemon(t)

	daemon, err := StartDaemon(context.Background(), binPath)
	if err != nil {
		t.Fatalf("StartDaemon: %v", err)
	}
	t.Cleanup(func() { _ = daemon.Stop() })

	defs := daemon.Defs()
	if len(defs) == 0 {
		t.Fatal("no defs")
	}

	ref := NewDaemonToolRef(daemon, defs[0])

	if ref.Name() != "greet" {
		t.Errorf("Name() = %q", ref.Name())
	}
	if ref.Description() != "returns a greeting" {
		t.Errorf("Description() = %q", ref.Description())
	}

	td := ref.Definition()
	if td.Name != "greet" {
		t.Errorf("Definition().Name = %q", td.Name)
	}

	result, err := ref.Execute(context.Background(), map[string]any{"name": "Ref"})
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	got := result.(map[string]any)
	if got["greeting"] != "Hello, Ref!" {
		t.Errorf("greeting = %v", got["greeting"])
	}
}

func TestDaemonTool_Stop(t *testing.T) {
	binPath := buildTestDaemon(t)

	daemon, err := StartDaemon(context.Background(), binPath)
	if err != nil {
		t.Fatalf("StartDaemon: %v", err)
	}

	if err := daemon.Stop(); err != nil {
		// A non-zero exit is fine (stdin closed causes scanner to return).
		t.Logf("Stop returned: %v (may be expected)", err)
	}
}

