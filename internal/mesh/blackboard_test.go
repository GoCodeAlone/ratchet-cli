package mesh

import (
	"sort"
	"sync"
	"testing"
)

func TestBlackboard_ReadWrite(t *testing.T) {
	bb := NewBlackboard()

	// Read from non-existent section
	_, ok := bb.Read("plan", "goal")
	if ok {
		t.Fatal("expected false for missing section")
	}

	// Write and read back
	e := bb.Write("plan", "goal", "build mesh", "node-1")
	if e.Key != "goal" || e.Value != "build mesh" || e.Author != "node-1" || e.Revision != 1 {
		t.Fatalf("unexpected entry: %+v", e)
	}

	got, ok := bb.Read("plan", "goal")
	if !ok {
		t.Fatal("expected entry to exist")
	}
	if got.Value != "build mesh" {
		t.Fatalf("got %v, want build mesh", got.Value)
	}

	// Read missing key in existing section
	_, ok = bb.Read("plan", "missing")
	if ok {
		t.Fatal("expected false for missing key")
	}
}

func TestBlackboard_RevisionMonotonic(t *testing.T) {
	bb := NewBlackboard()
	e1 := bb.Write("s", "a", 1, "w")
	e2 := bb.Write("s", "b", 2, "w")
	e3 := bb.Write("s", "a", 3, "w") // overwrite

	if e1.Revision >= e2.Revision || e2.Revision >= e3.Revision {
		t.Fatalf("revisions not monotonic: %d, %d, %d", e1.Revision, e2.Revision, e3.Revision)
	}
}

func TestBlackboard_List(t *testing.T) {
	bb := NewBlackboard()
	bb.Write("sec", "a", 1, "w")
	bb.Write("sec", "b", 2, "w")

	entries := bb.List("sec")
	if len(entries) != 2 {
		t.Fatalf("expected 2 entries, got %d", len(entries))
	}

	// Missing section returns nil
	if bb.List("nope") != nil {
		t.Fatal("expected nil for missing section")
	}
}

func TestBlackboard_ListSections(t *testing.T) {
	bb := NewBlackboard()
	bb.Write("alpha", "k", 1, "w")
	bb.Write("beta", "k", 2, "w")

	secs := bb.ListSections()
	sort.Strings(secs)
	if len(secs) != 2 || secs[0] != "alpha" || secs[1] != "beta" {
		t.Fatalf("unexpected sections: %v", secs)
	}
}

func TestBlackboard_Watch(t *testing.T) {
	bb := NewBlackboard()

	var mu sync.Mutex
	var notifications []string

	bb.Watch(func(key string, val Entry) {
		mu.Lock()
		notifications = append(notifications, key)
		mu.Unlock()
	})

	bb.Write("sec", "k1", "v1", "w")
	bb.Write("sec", "k2", "v2", "w")

	mu.Lock()
	defer mu.Unlock()
	if len(notifications) != 2 {
		t.Fatalf("expected 2 notifications, got %d", len(notifications))
	}
	if notifications[0] != "sec/k1" || notifications[1] != "sec/k2" {
		t.Fatalf("unexpected notifications: %v", notifications)
	}
}

func TestBlackboard_ConcurrentReadWrite(t *testing.T) {
	bb := NewBlackboard()
	const goroutines = 50
	const writes = 100

	var wg sync.WaitGroup
	wg.Add(goroutines * 2) // writers + readers

	// writers
	for g := range goroutines {
		go func(id int) {
			defer wg.Done()
			for i := range writes {
				bb.Write("concurrent", "key", i, "writer")
				_ = id
			}
		}(g)
	}

	// readers
	for range goroutines {
		go func() {
			defer wg.Done()
			for range writes {
				bb.Read("concurrent", "key")
				bb.List("concurrent")
				bb.ListSections()
			}
		}()
	}

	wg.Wait()

	// Verify final state is consistent
	e, ok := bb.Read("concurrent", "key")
	if !ok {
		t.Fatal("expected entry to exist after concurrent writes")
	}
	if e.Revision < 1 {
		t.Fatalf("expected positive revision, got %d", e.Revision)
	}
}
