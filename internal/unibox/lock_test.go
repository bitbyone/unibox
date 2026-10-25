package unibox

import (
	"path/filepath"
	"testing"
)

// Each lock is its own open file, as two unibox processes have, so it
// behaves alike within a process and on every system.
func TestALockKeepsOthersOutUntilItIsClosed(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "locks", "x.lock")
	exclusive, err := LockFile(path, false, false)
	if err != nil {
		t.Fatal(err)
	}
	for _, shared := range []bool{false, true} {
		if l, err := LockFile(path, shared, false); err == nil {
			l.Close()
			t.Fatalf("shared %v: taken while held", shared)
		}
	}
	if err = exclusive.Close(); err != nil {
		t.Fatal(err)
	}
	first, err := LockFile(path, true, false)
	if err != nil {
		t.Fatal(err)
	}
	second, err := LockFile(path, true, false)
	if err != nil {
		t.Fatalf("a shared lock kept another out: %v", err)
	}
	if l, err := LockFile(path, false, false); err == nil {
		l.Close()
		t.Fatal("an exclusive lock was taken while shared ones were held")
	}
	first.Close()
	second.Close()
	waited, err := LockFile(path, false, true)
	if err != nil {
		t.Fatal(err)
	}
	if err = waited.Close(); err != nil || waited.Close() != nil {
		t.Fatal("closing a lock twice failed")
	}
}
