//go:build !unix

package task

import "os"

func flock(f *os.File) error { return nil }

func funlock(f *os.File) {}
