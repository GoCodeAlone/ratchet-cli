//go:build windows

package daemon

import (
	"os"
	"os/exec"
	"testing"
)

func TestDaemonLockExcludesSecondOwner(t *testing.T) {
	setDaemonLockTestHome(t)
	first, err := acquireDaemonLock()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = first.Close() })
	if second, err := acquireDaemonLock(); err == nil {
		_ = second.Close()
		t.Fatal("second daemon lock owner succeeded")
	}
}

func TestDaemonLockReleasesAfterClose(t *testing.T) {
	setDaemonLockTestHome(t)
	first, err := acquireDaemonLock()
	if err != nil {
		t.Fatal(err)
	}
	if err := first.Close(); err != nil {
		t.Fatal(err)
	}
	second, err := acquireDaemonLock()
	if err != nil {
		t.Fatalf("acquire after close: %v", err)
	}
	_ = second.Close()
}

func TestDaemonLockReleasesAfterProcessExit(t *testing.T) {
	home := t.TempDir()
	cmd := exec.Command(os.Args[0], "-test.run=^TestDaemonLockHelperProcess$")
	cmd.Env = append(os.Environ(),
		"RATCHET_DAEMON_LOCK_HELPER=1",
		"HOME="+home,
		"USERPROFILE="+home,
	)
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("lock helper: %v\n%s", err, output)
	}
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	lock, err := acquireDaemonLock()
	if err != nil {
		t.Fatalf("acquire after helper exit: %v", err)
	}
	_ = lock.Close()
}

func TestDaemonLockHelperProcess(t *testing.T) {
	if os.Getenv("RATCHET_DAEMON_LOCK_HELPER") != "1" {
		return
	}
	if err := EnsureDataDir(); err != nil {
		t.Fatal(err)
	}
	lock, err := acquireDaemonLock()
	if err != nil {
		t.Fatal(err)
	}
	_ = lock
}

func setDaemonLockTestHome(t *testing.T) {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	if err := EnsureDataDir(); err != nil {
		t.Fatal(err)
	}
}
