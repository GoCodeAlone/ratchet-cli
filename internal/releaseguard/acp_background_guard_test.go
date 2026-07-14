package releaseguard

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

var backgroundWindowsNativeAttackTests = []string{
	"TestBackgroundWindowsAuditRejectsReparsePoint",
	"TestBackgroundWindowsAuditRejectsHardLink",
	"TestBackgroundWindowsAuditRejectsParentReplacement",
	"TestBackgroundWindowsAuditRejectsWeakDACL",
}

func TestBackgroundWindowsNativeAttackTestsRemainInCI(t *testing.T) {
	path := filepath.Join(repoRoot(t), "internal", "acpclient", "background_persist_windows_test.go")
	source, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read Windows audit tests: %v", err)
	}
	file, err := parser.ParseFile(token.NewFileSet(), path, source, 0)
	if err != nil {
		t.Fatalf("parse Windows audit tests: %v", err)
	}
	declared := make(map[string]*ast.FuncDecl)
	for _, declaration := range file.Decls {
		if fn, ok := declaration.(*ast.FuncDecl); ok {
			declared[fn.Name.Name] = fn
		}
	}
	for _, name := range backgroundWindowsNativeAttackTests {
		fn := declared[name]
		if fn == nil {
			t.Errorf("missing native Windows audit test %s", name)
			continue
		}
		if backgroundWindowsTestIsSkipped(fn) {
			t.Errorf("native Windows audit test %s calls Skip, Skipf, or SkipNow", name)
		}
	}

	workflow, err := os.ReadFile(filepath.Join(repoRoot(t), ".github", "workflows", "ci.yml"))
	if err != nil {
		t.Fatalf("read CI workflow: %v", err)
	}
	if !strings.Contains(string(workflow), "go test ./internal/acpclient -run '^TestBackgroundWindows' -count=1") {
		t.Fatal("CI no longer runs the TestBackgroundWindows selector")
	}
}

func TestBackgroundWindowsSkipDetection(t *testing.T) {
	for _, method := range []string{"Skip", "Skipf", "SkipNow"} {
		t.Run(method, func(t *testing.T) {
			file, err := parser.ParseFile(token.NewFileSet(), "fixture_test.go", `package fixture
import "testing"
func TestBackgroundWindowsAuditRejectsReparsePoint(t *testing.T) { t.`+method+`("not implemented") }
`, 0)
			if err != nil {
				t.Fatalf("parse fixture: %v", err)
			}
			fn := file.Decls[1].(*ast.FuncDecl)
			if !backgroundWindowsTestIsSkipped(fn) {
				t.Fatalf("skip detector accepted t.%s", method)
			}
		})
	}
}

func backgroundWindowsTestIsSkipped(fn *ast.FuncDecl) bool {
	if fn.Body == nil {
		return false
	}
	testingParams := make(map[string]struct{})
	for _, field := range fn.Type.Params.List {
		for _, name := range field.Names {
			testingParams[name.Name] = struct{}{}
		}
	}
	skipped := false
	ast.Inspect(fn.Body, func(node ast.Node) bool {
		call, ok := node.(*ast.CallExpr)
		if !ok {
			return true
		}
		selector, ok := call.Fun.(*ast.SelectorExpr)
		if !ok || !backgroundWindowsSkipMethod(selector.Sel.Name) {
			return true
		}
		receiver, ok := selector.X.(*ast.Ident)
		if !ok {
			return true
		}
		_, skipped = testingParams[receiver.Name]
		return !skipped
	})
	return skipped
}

func backgroundWindowsSkipMethod(name string) bool {
	return name == "Skip" || name == "Skipf" || name == "SkipNow"
}
