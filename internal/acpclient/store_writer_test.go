package acpclient

import (
	"go/ast"
	"go/parser"
	"go/token"
	"path/filepath"
	"testing"
)

func TestSessionWriterInventory(t *testing.T) {
	t.Helper()

	writers := map[string]string{
		"Upsert":                       "transitionSession",
		"InsertSession":                "createSessionWithEvents",
		"updateSessionLifecycle":       "transitionSession",
		"AppendQueuedPrompt":           "transitionSession",
		"MarkQueueRunning":             "transitionSession",
		"updateQueuedPromptWithEvents": "transitionSession",
		"CancelPendingQueue":           "transitionSession",
		"recoverRunningQueueItems":     "transitionSession",
		"MarkPendingCanceled":          "transitionSession",
		"requestCancel":                "transitionSession",
		"setACPSessionID":              "transitionSession",
	}
	directSaveAllowlist := map[string]bool{
		"transitionSession":       true,
		"createSessionWithEvents": true,
	}
	files := []string{"store.go", "archive.go", "eventlog.go"}
	functions := make(map[string]*ast.FuncDecl)
	fset := token.NewFileSet()
	for _, name := range files {
		file, err := parser.ParseFile(fset, filepath.Join(".", name), nil, 0)
		if err != nil {
			t.Fatalf("parse %s: %v", name, err)
		}
		for _, decl := range file.Decls {
			fn, ok := decl.(*ast.FuncDecl)
			if ok && fn.Body != nil {
				functions[fn.Name.Name] = fn
			}
		}
	}

	for writer, guard := range writers {
		fn := functions[writer]
		if fn == nil {
			t.Errorf("session writer %s is missing", writer)
			continue
		}
		if !functionCalls(fn, guard) {
			t.Errorf("session writer %s must call %s", writer, guard)
		}
	}
	for name, fn := range functions {
		if functionCalls(fn, "saveUnlocked") && !directSaveAllowlist[name] {
			t.Errorf("%s writes sessions directly; use guarded transition or create-only helper", name)
		}
	}
}

func functionCalls(fn *ast.FuncDecl, name string) bool {
	found := false
	ast.Inspect(fn.Body, func(node ast.Node) bool {
		call, ok := node.(*ast.CallExpr)
		if !ok {
			return true
		}
		switch target := call.Fun.(type) {
		case *ast.Ident:
			found = found || target.Name == name
		case *ast.SelectorExpr:
			found = found || target.Sel.Name == name
		}
		return !found
	})
	return found
}
