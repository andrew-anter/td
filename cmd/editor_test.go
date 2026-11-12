/*
Copyright © 2025 Andrew Anter <andrew.anter@gmail.com>
*/
package cmd

import (
	"os"
	"path/filepath"
	"testing"
)

func TestTaskBufferRoundTrip(t *testing.T) {
	buf := taskBuffer("fix the sink", "kerl valve stripped\n\nneed a 9/16 wrench")
	title, desc := parseTaskBuffer(buf)
	if title != "fix the sink" {
		t.Errorf("title = %q", title)
	}
	if desc != "kerl valve stripped\n\nneed a 9/16 wrench" {
		t.Errorf("desc = %q — paragraphs must survive untouched", desc)
	}
}

func TestParseTaskBufferStripsCommentsAndBlanks(t *testing.T) {
	in := "# header comment\r\n#\r\n\r\n  title with trailing space  \r\n\r\n\r\nbody line\r\n\r\n\r\n"
	title, desc := parseTaskBuffer(in)
	if title != "title with trailing space" {
		t.Errorf("title = %q", title)
	}
	if desc != "body line" {
		t.Errorf("desc = %q", desc)
	}
}

func TestParseTaskBufferKeepsInternalParagraphs(t *testing.T) {
	_, desc := parseTaskBuffer("t\n\npara one\nstill one\n\npara two")
	if desc != "para one\nstill one\n\npara two" {
		t.Errorf("desc = %q", desc)
	}
}

func TestParseTaskBufferEmpty(t *testing.T) {
	title, desc := parseTaskBuffer("# only comments\n\n\n")
	if title != "" || desc != "" {
		t.Errorf("title = %q, desc = %q, want both empty", title, desc)
	}
}

func TestParseTaskBufferLenientNoBlankSeparator(t *testing.T) {
	title, desc := parseTaskBuffer("t\ndirect body")
	if title != "t" || desc != "direct body" {
		t.Errorf("title = %q, desc = %q", title, desc)
	}
}

func TestExternalEditorPrefersEnv(t *testing.T) {
	t.Setenv("EDITOR", "mycustom -w")
	if got := externalEditor(); got != "mycustom -w" {
		t.Errorf("EDITOR ignored: %q", got)
	}
	t.Setenv("EDITOR", "")
	t.Setenv("VISUAL", "vis")
	if got := externalEditor(); got != "vis" {
		t.Errorf("VISUAL ignored: %q", got)
	}
}

func TestExternalEditorFallsBackToPathScan(t *testing.T) {
	t.Setenv("EDITOR", "")
	t.Setenv("VISUAL", "")
	dir := t.TempDir()
	for _, name := range []string{"vim", "vi"} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte("#!/bin/sh\n"), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	t.Setenv("PATH", dir)
	if got := externalEditor(); got != "vim" {
		t.Errorf("chain without nvim = %q, want vim", got)
	}
	if err := os.WriteFile(filepath.Join(dir, "nvim"), []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	if got := externalEditor(); got != "nvim" {
		t.Errorf("chain with nvim = %q, want nvim", got)
	}
}
