/*
Copyright © 2025 Andrew Anter <andrew.anter@gmail.com>
*/
package cmd

import (
	"fmt"
	"strings"
	"testing"

	"charm.land/lipgloss/v2"

	tea "charm.land/bubbletea/v2"
)

func keyMsg(k string) tea.KeyPressMsg {
	switch k {
	case "esc":
		return tea.KeyPressMsg{Code: tea.KeyEsc}
	case "enter":
		return tea.KeyPressMsg{Code: tea.KeyEnter}
	}
	r := []rune(k)[0]
	var mod tea.KeyMod
	if r >= 'A' && r <= 'Z' {
		mod = tea.ModShift
	}
	return tea.KeyPressMsg{Code: r, Text: k, ShiftedCode: r, Mod: mod}
}

func drive(v *vimInput, keys []string) vimAction {
	act := vimKeyHandled
	for _, k := range keys {
		act, _ = v.HandleKey(keyMsg(k))
	}
	return act
}

func newField(text string) *vimInput {
	v := newVimInput()
	v.SetValue(text)
	v.Enter()
	v.Focus()
	return &v
}

func TestVimEditing(t *testing.T) {
	cases := []struct {
		name    string
		text    string
		keys    []string
		want    string
		wantPos int
		wantSt  vimState
		wantAct vimAction
	}{
		{"insert passthrough", "", []string{"s", "q", "u", "a", "t"}, "squat", 5, vimInsert, vimKeyHandled},
		{"enter commits from insert", "", []string{"a", "enter"}, "a", 1, vimInsert, vimCommit},
		{"enter commits from normal", "done task", []string{"esc", "enter"}, "done task", 8, vimNormal, vimCommit},
		{"esc once to normal", "abc", []string{"esc"}, "abc", 2, vimNormal, vimKeyHandled},
		{"esc twice cancels", "abc", []string{"esc", "esc"}, "abc", 2, vimNormal, vimCancel},
		{"empty esc", "", []string{"esc"}, "", 0, vimNormal, vimKeyHandled},

		{"b to word start", "buy milk today", []string{"esc", "b"}, "buy milk today", 9, vimNormal, vimKeyHandled},
		{"bb", "buy milk today", []string{"esc", "b", "b"}, "buy milk today", 4, vimNormal, vimKeyHandled},
		{"bbb tops line", "buy milk today", []string{"esc", "b", "b", "b"}, "buy milk today", 0, vimNormal, vimKeyHandled},
		{"e chain", "buy milk today", []string{"esc", "0", "e", "e", "e"}, "buy milk today", 13, vimNormal, vimKeyHandled},
		{"w chain", "buy milk today", []string{"esc", "0", "w", "w", "w"}, "buy milk today", 13, vimNormal, vimKeyHandled},
		{"gg tops line", "hello world", []string{"esc", "g", "g"}, "hello world", 0, vimNormal, vimKeyHandled},
		{"G bottoms line", "hello world", []string{"esc", "0", "G"}, "hello world", 10, vimNormal, vimKeyHandled},

		{"dw last word", "foo bar", []string{"esc", "b", "d", "w"}, "foo ", 3, vimNormal, vimKeyHandled},
		{"cw then type", "hello world", []string{"esc", "b", "c", "w", "t", "h", "e", "r", "e"}, "hello there", 11, vimInsert, vimKeyHandled},
		{"db", "foo bar", []string{"esc", "d", "b"}, "foo r", 4, vimNormal, vimKeyHandled},
		{"de", "foo bar", []string{"esc", "0", "w", "d", "e"}, "foo ", 3, vimNormal, vimKeyHandled},
		{"dh", "foo bar", []string{"esc", "d", "h"}, "foo br", 5, vimNormal, vimKeyHandled},
		{"dl", "foo bar", []string{"esc", "d", "l"}, "foo ba", 5, vimNormal, vimKeyHandled},
		{"d0", "foo bar", []string{"esc", "d", "0"}, "", 0, vimNormal, vimKeyHandled},
		{"d$", "foo bar baz", []string{"esc", "0", "w", "d", "$"}, "foo ", 3, vimNormal, vimKeyHandled},
		{"D to eol", "hello world", []string{"esc", "0", "w", "D"}, "hello ", 5, vimNormal, vimKeyHandled},
		{"C to eol then type", "hello world", []string{"esc", "0", "w", "C", "!"}, "hello !", 7, vimInsert, vimKeyHandled},
		{"dd clears line", "abc", []string{"esc", "d", "d"}, "", 0, vimNormal, vimKeyHandled},
		{"cc clears into insert", "abc", []string{"esc", "c", "c"}, "", 0, vimInsert, vimKeyHandled},
		{"S clears into insert", "abc", []string{"esc", "S"}, "", 0, vimInsert, vimKeyHandled},
		{"x single", "abcdef", []string{"esc", "0", "x"}, "bcdef", 0, vimNormal, vimKeyHandled},
		{"2x count", "abcdef", []string{"esc", "0", "2", "x"}, "cdef", 0, vimNormal, vimKeyHandled},
		{"s substitute", "abc", []string{"esc", "0", "s", "X"}, "Xbc", 1, vimInsert, vimKeyHandled},
		{"r replace", "abc", []string{"esc", "0", "r", "X"}, "Xbc", 0, vimNormal, vimKeyHandled},
		{"2r replaces two", "abcd", []string{"esc", "0", "2", "r", "X"}, "XXcd", 0, vimNormal, vimKeyHandled},

		{"f find", "hello world", []string{"esc", "0", "f", "o"}, "hello world", 4, vimNormal, vimKeyHandled},
		{"t until", "hello world", []string{"esc", "0", "t", "o"}, "hello world", 5, vimNormal, vimKeyHandled},
		{"F back find", "hello world", []string{"esc", "F", "o"}, "hello world", 7, vimNormal, vimKeyHandled},
		{"2f second match", "hello world", []string{"esc", "0", "2", "f", "o"}, "hello world", 7, vimNormal, vimKeyHandled},
		{"find miss is noop", "hello world", []string{"esc", "0", "f", "z"}, "hello world", 0, vimNormal, vimKeyHandled},
		{"df space", "hello world", []string{"esc", "0", "d", "f", " "}, "world", 0, vimNormal, vimKeyHandled},
		{"dt space", "hello world", []string{"esc", "0", "d", "t", " "}, " world", 0, vimNormal, vimKeyHandled},
		{"cf find change", "hello world", []string{"esc", "0", "c", "f", "o", "!"}, "! world", 1, vimInsert, vimKeyHandled},

		{"0 line start", "hello world", []string{"esc", "0"}, "hello world", 0, vimNormal, vimKeyHandled},
		{"$ line end", "hello world", []string{"esc", "0", "$"}, "hello world", 10, vimNormal, vimKeyHandled},
		{"^ first nonblank", "  indented", []string{"esc", "^"}, "  indented", 2, vimNormal, vimKeyHandled},
		{"I insert at first nonblank", "  indented", []string{"esc", "I"}, "  indented", 2, vimInsert, vimKeyHandled},
		{"A append", "abc", []string{"esc", "A", "!"}, "abc!", 4, vimInsert, vimKeyHandled},
		{"a after char", "abc", []string{"esc", "0", "a", "X"}, "aXbc", 2, vimInsert, vimKeyHandled},
		{"i before char", "abc", []string{"esc", "0", "i", "X"}, "Xabc", 1, vimInsert, vimKeyHandled},
		{"10l count motion", "0123456789ab", []string{"esc", "0", "1", "0", "l"}, "0123456789ab", 10, vimNormal, vimKeyHandled},
		{"2dw deletes two words", "a b c d e f", []string{"esc", "0", "3", "w", "2", "d", "w"}, "a b c f", 6, vimNormal, vimKeyHandled},

		{"unknown normal key consumed", "foo bar", []string{"esc", "y", "l"}, "foo bar", 6, vimNormal, vimKeyHandled},
		{"pending operator aborted", "foo bar", []string{"esc", "d", "z", "w"}, "foo bar", 6, vimNormal, vimKeyHandled},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			v := newField(tc.text)
			act := drive(v, tc.keys)
			if act != tc.wantAct {
				t.Fatalf("action = %v, want %v", act, tc.wantAct)
			}
			if v.Value() != tc.want {
				t.Fatalf("value = %q, want %q", v.Value(), tc.want)
			}
			if v.Position() != tc.wantPos {
				t.Fatalf("pos = %d, want %d", v.Position(), tc.wantPos)
			}
			if v.state != tc.wantSt {
				t.Fatalf("state = %v, want %v", v.state, tc.wantSt)
			}
		})
	}
}

func TestVimEnterResetsState(t *testing.T) {
	v := newField("abc")
	drive(v, []string{"esc", "d"})
	v.SetValue("reworn")
	v.Enter()
	if v.state != vimInsert || v.count != 0 || v.op != 0 || v.mot != 0 {
		t.Fatalf("Enter did not reset modal state: %+v", v)
	}
	if v.Position() != len([]rune("reworn")) {
		t.Fatalf("Enter should park cursor at end, pos = %d", v.Position())
	}
}

func TestEditViewWrapsLongText(t *testing.T) {
	v := newField("buy a really long amount of groceries for the entire week including milk eggs bread cheese")
	block, h := v.EditView(30, 6)
	if h < 2 {
		t.Fatalf("height = %d, want wrapped multi-line block", h)
	}
	for _, line := range strings.Split(block, "\n") {
		if w := lipgloss.Width(line); w > 30 {
			t.Errorf("line %q is %d cells, want ≤ 30", line, w)
		}
	}
	if n := strings.Count(block, "\x1b[7m"); n != 1 {
		t.Errorf("caret count = %d, want exactly 1", n)
	}
}

func TestEditViewScrollsToKeepCaretVisible(t *testing.T) {
	var words []string
	for i := range 40 {
		words = append(words, fmt.Sprintf("w%d", i))
	}
	v := newField(strings.Join(words, " "))
	v.CursorEnd()
	block, h := v.EditView(24, 4)
	if h != 4 {
		t.Fatalf("height = %d, want capped at 4", h)
	}
	if strings.Contains(block, "w0 ") {
		t.Errorf("first line still visible — block did not scroll toward the caret")
	}
	last := strings.Split(block, "\n")[h-1]
	if !strings.Contains(last, "\x1b[7m") {
		t.Errorf("caret not on the last visible line: %q", last)
	}
}

func TestEditViewSplitsOversizedWord(t *testing.T) {
	v := newField(strings.Repeat("x", 100))
	block, h := v.EditView(42, 6)
	if h != 3 {
		t.Fatalf("height = %d, want 3 (100 cells across a 40-cell text width)", h)
	}
	for _, line := range strings.Split(block, "\n") {
		if w := lipgloss.Width(line); w > 42 {
			t.Errorf("line width %d, want ≤ 42", w)
		}
	}
}

func TestEditViewEmptyShowsPlaceholder(t *testing.T) {
	v := newVimInput()
	v.Placeholder = "new task"
	block, h := v.EditView(60, 6)
	if h != 1 {
		t.Fatalf("height = %d, want 1", h)
	}
	if !strings.Contains(block, "new task") || !strings.Contains(block, "\x1b[7m") {
		t.Errorf("placeholder view = %q, want dim text and a caret", block)
	}
}

func TestEditViewFullLastLineWrapsEolCaret(t *testing.T) {
	v := newField(strings.Repeat("x", 28))
	block, h := v.EditView(30, 6)
	if h != 2 {
		t.Fatalf("height = %d, want 2 — the EOL caret needs its own line", h)
	}
	for _, line := range strings.Split(block, "\n") {
		if w := lipgloss.Width(line); w > 30 {
			t.Errorf("line width %d, want ≤ 30", w)
		}
	}
	if n := strings.Count(block, "\x1b[7m"); n != 1 {
		t.Errorf("caret count = %d, want exactly 1", n)
	}
}
