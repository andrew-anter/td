//go:build unix

package task

import (
	"os"
	"syscall"
)

func flock(f *os.File) error {
	return syscall.Flock(int(f.Fd()), syscall.LOCK_EX)
}

func funlock(f *os.File) {
	// Best-effort: the handle close releases the lock anyway.
	_ = syscall.Flock(int(f.Fd()), syscall.LOCK_UN)
}
