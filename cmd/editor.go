/*
Copyright © 2025 Andrew Anter <andrew.anter@gmail.com>
*/
package cmd

import (
	"os"
	"os/exec"
	"strings"
)

// taskBuffer renders git-style edit content: first line is the title,
// text after it is the description. The comment header is stripped on
// parse, like git's default commentchar behavior.
func taskBuffer(text, description string) string {
	var b strings.Builder
	b.WriteString("# Edit the task.\n")
	b.WriteString("# The first line below becomes the title; the text after it\n")
	b.WriteString("# (separated by a blank line) becomes the description.\n")
	b.WriteString("# Paragraph breaks inside the description are kept.\n")
	b.WriteString("# Lines starting with '#' are ignored.\n\n")
	b.WriteString(text)
	b.WriteString("\n")
	if description != "" {
		b.WriteString("\n" + description + "\n")
	}
	return b.String()
}

// parseTaskBuffer splits edited content into title and description:
// comment lines and CRLF are dropped, leading blank lines are trimmed,
// the first remaining line is the title, and everything after it (with
// outer blank lines trimmed, inner ones kept) is the description.
func parseTaskBuffer(s string) (string, string) {
	var lines []string
	for _, l := range strings.Split(strings.ReplaceAll(s, "\r\n", "\n"), "\n") {
		if strings.HasPrefix(l, "#") {
			continue
		}
		lines = append(lines, strings.TrimRight(l, " \t\r"))
	}
	for len(lines) > 0 && strings.TrimSpace(lines[0]) == "" {
		lines = lines[1:]
	}
	if len(lines) == 0 {
		return "", ""
	}
	title := strings.TrimSpace(lines[0])
	rest := lines[1:]
	for len(rest) > 0 && strings.TrimSpace(rest[0]) == "" {
		rest = rest[1:]
	}
	for len(rest) > 0 && strings.TrimSpace(rest[len(rest)-1]) == "" {
		rest = rest[:len(rest)-1]
	}
	return title, strings.Join(rest, "\n")
}

// externalEditor resolves git's EDITOR/VISUAL chain, then falls back
// to the first of nvim/vim/vi on PATH — many systems ship modern
// editors without providing `vi`.
func externalEditor() string {
	for _, key := range []string{"EDITOR", "VISUAL"} {
		if v := os.Getenv(key); v != "" {
			return v
		}
	}
	for _, prog := range []string{"nvim", "vim", "vi"} {
		if _, err := exec.LookPath(prog); err == nil {
			return prog
		}
	}
	return "vi"
}
