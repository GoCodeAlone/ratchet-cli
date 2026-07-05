package routines

import (
	"path/filepath"
	"testing"
)

func TestStoreAddListShowPauseResumeRemoveRoutine(t *testing.T) {
	store, err := Load(filepath.Join(t.TempDir(), "routines.json"))
	if err != nil {
		t.Fatalf("load store: %v", err)
	}

	def, err := store.Add(AddRequest{Schedule: "15m", Prompt: "summarize status", CWD: "/tmp/project", Provider: "openai"})
	if err != nil {
		t.Fatalf("add routine: %v", err)
	}
	if def.ID == "" || def.Schedule != "15m" || def.Prompt != "summarize status" || def.Paused {
		t.Fatalf("definition = %#v", def)
	}

	list := store.List()
	if len(list) != 1 || list[0].ID != def.ID {
		t.Fatalf("list = %#v", list)
	}

	if err := store.Pause(def.ID); err != nil {
		t.Fatalf("pause: %v", err)
	}
	paused, ok := store.Get(def.ID)
	if !ok || !paused.Paused {
		t.Fatalf("paused definition = %#v, ok=%v", paused, ok)
	}

	if err := store.Resume(def.ID); err != nil {
		t.Fatalf("resume: %v", err)
	}
	resumed, ok := store.Get(def.ID)
	if !ok || resumed.Paused {
		t.Fatalf("resumed definition = %#v, ok=%v", resumed, ok)
	}

	if err := store.Remove(def.ID); err != nil {
		t.Fatalf("remove: %v", err)
	}
	if _, ok := store.Get(def.ID); ok {
		t.Fatal("routine still present after remove")
	}
}

func TestStoreManualRunPersistsVisibleState(t *testing.T) {
	path := filepath.Join(t.TempDir(), "routines.json")
	store, err := Load(path)
	if err != nil {
		t.Fatalf("load store: %v", err)
	}
	def, err := store.Add(AddRequest{Schedule: "0 9 * * *", Prompt: "prepare daily plan"})
	if err != nil {
		t.Fatalf("add routine: %v", err)
	}

	run, err := store.RunManual(def.ID)
	if err != nil {
		t.Fatalf("manual run: %v", err)
	}
	if run.ID == "" || run.RoutineID != def.ID || run.Status != RunStatusRecorded {
		t.Fatalf("run = %#v", run)
	}

	reloaded, err := Load(path)
	if err != nil {
		t.Fatalf("reload store: %v", err)
	}
	got, ok := reloaded.Get(def.ID)
	if !ok || got.LastRun == nil || got.LastRun.RunID != run.ID {
		t.Fatalf("definition after reload = %#v, ok=%v", got, ok)
	}
	runs := reloaded.RunsForRoutine(def.ID)
	if len(runs) != 1 || runs[0].ID != run.ID {
		t.Fatalf("runs = %#v", runs)
	}
}

func TestStoreRejectsIncompleteRoutine(t *testing.T) {
	store, err := Load(filepath.Join(t.TempDir(), "routines.json"))
	if err != nil {
		t.Fatalf("load store: %v", err)
	}
	if _, err := store.Add(AddRequest{Schedule: "15m"}); err == nil {
		t.Fatal("expected missing prompt error")
	}
	if _, err := store.Add(AddRequest{Prompt: "summarize"}); err == nil {
		t.Fatal("expected missing schedule error")
	}
}
