/*
Package Task Copyright © 2025 Andrew Anter <andrew.anter@gmail.com>
*/
package task

import (
	"io"
	"os"
	"path/filepath"
)

// Lock acquires an exclusive advisory lock on a sidecar file next to
// filename, serializing read-modify-write cycles across concurrent td
// processes. The returned Closer releases the lock. On platforms where
// file locking is unsupported this still returns a valid (no-op)
// closer; SaveItems' atomic rename keeps the data file uncorrupted even
// then, at the cost of possible lost updates.
func Lock(filename string) (io.Closer, error) {
	lockPath := filepath.Join(filepath.Dir(filename), filepath.Base(filename)+".lock")
	f, err := os.OpenFile(lockPath, os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return nil, err
	}
	if err := flock(f); err != nil {
		f.Close()
		return nil, err
	}
	return &fileLock{f}, nil
}

type fileLock struct{ f *os.File }

func (l *fileLock) Close() error {
	funlock(l.f)
	return l.f.Close()
}
