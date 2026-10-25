//go:build windows

package unibox

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"

	"golang.org/x/sys/windows"
)

type FileLock struct{ f *os.File }

// LockFile locks the whole of a file, as flock does elsewhere. Windows
// passes a child no handle it is not given, so a lock is never held by a
// program unibox started.
func LockFile(path string, shared, wait bool) (*FileLock, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return nil, err
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return nil, err
	}
	var flags uint32
	if !shared {
		flags |= windows.LOCKFILE_EXCLUSIVE_LOCK
	}
	if !wait {
		flags |= windows.LOCKFILE_FAIL_IMMEDIATELY
	}
	if err = windows.LockFileEx(windows.Handle(f.Fd()), flags, 0, ^uint32(0), ^uint32(0), new(windows.Overlapped)); err != nil {
		f.Close()
		return nil, fmt.Errorf("resource is busy; wait for the active unibox operation to finish: %w", err)
	}
	return &FileLock{f}, nil
}

func (l *FileLock) Close() error {
	if l == nil || l.f == nil {
		return nil
	}
	_ = windows.UnlockFileEx(windows.Handle(l.f.Fd()), 0, ^uint32(0), ^uint32(0), new(windows.Overlapped))
	err := l.f.Close()
	l.f = nil
	return err
}

// detach starts a command apart from unibox's console, so it outlives it.
func detach(c *exec.Cmd) {
	c.SysProcAttr = &windows.SysProcAttr{CreationFlags: windows.CREATE_NEW_PROCESS_GROUP | windows.DETACHED_PROCESS}
}
