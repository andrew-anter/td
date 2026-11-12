/*
Copyright © 2025 Andrew Anter <andrew.anter@gmail.com>
*/
package cmd

import (
	"unicode"

	"charm.land/bubbles/v2/key"
	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
)

type vimState int

const (
	vimInsert vimState = iota
	vimNormal
)

// vimAction reports what a key routed to the field means at its
// boundary: consumed in place, commit the value, or abandon editing.
type vimAction int

const (
	vimKeyHandled vimAction = iota
	vimCommit
	vimCancel
)

// vimInput layers vim's modal editing on top of the bubbles text
// input: INSERT passes keys through (emacs bindings included), NORMAL
// gets motions and operators scoped to a single line. esc toggles
// INSERT→NORMAL, and a second esc cancels the edit, which is why
// commit stays on enter.
type vimInput struct {
	textinput.Model
	state vimState
	count int  // pending count typed before a motion or operator
	op    rune // pending operator: d c r f F t T g
	mot   rune // pending find motion inside d/c: f F t T, awaiting char
}

func newVimInput() vimInput {
	t := textinput.New()
	t.Prompt = "❯ "
	t.SetWidth(60)
	return vimInput{Model: t, state: vimInsert}
}

// Enter arms the field for a fresh edit session in insert mode with
// the cursor at the end of the text.
func (v *vimInput) Enter() {
	v.state = vimInsert
	v.clearPending()
	v.CursorEnd()
}

func (v vimInput) modeTag() string {
	if v.state == vimNormal {
		return vimNormalTagStyle().Render("n") + " "
	}
	return vimInsertTagStyle().Render("i") + " "
}

func (v *vimInput) HandleKey(msg tea.KeyPressMsg) (vimAction, tea.Cmd) {
	switch msg.String() {
	case "enter":
		return vimCommit, nil
	case "esc":
		if v.state == vimInsert {
			v.toNormal()
			return vimKeyHandled, nil
		}
		return vimCancel, nil
	}
	if v.state == vimInsert {
		var cmd tea.Cmd
		v.Model, cmd = v.Model.Update(msg)
		return vimKeyHandled, cmd
	}
	v.normalKey(msg)
	return vimKeyHandled, nil
}

func (v *vimInput) toNormal() {
	v.state = vimNormal
	v.clearPending()
	rs := v.runes()
	if len(rs) == 0 {
		v.SetCursor(0)
		return
	}
	v.SetCursor(clampInt(v.Position()-1, 0, len(rs)-1))
}

func (v *vimInput) clearPending() { v.count, v.op, v.mot = 0, 0, 0 }

// cnt is the accumulated count, defaulting to 1 like vim.
func (v vimInput) cnt() int {
	if v.count > 0 {
		return v.count
	}
	return 1
}

func (v vimInput) runes() []rune { return []rune(v.Value()) }

// normalPos clamps the model cursor onto a character; NORMAL sits on
// a character, never in the gap past the end.
func (v vimInput) normalPos(n int) int {
	if n == 0 {
		return 0
	}
	return clampInt(v.Position(), 0, n-1)
}

func clampInt(x, lo, hi int) int {
	if x < lo {
		return lo
	}
	if x > hi {
		return hi
	}
	return x
}

// printRune extracts a plain typed character. Terminals report
// capitals (A, $, _) with the shift modifier, which must not disqualify
// them; ctrl/alt/meta chords must.
func printRune(msg tea.KeyPressMsg) (rune, bool) {
	if msg.Mod & ^tea.ModShift != 0 {
		return 0, false
	}
	rs := []rune(msg.Text)
	if len(rs) != 1 || unicode.IsControl(rs[0]) {
		return 0, false
	}
	return rs[0], true
}

func (v *vimInput) normalKey(msg tea.KeyPressMsg) {
	ch, printable := printRune(msg)
	if !printable {
		v.clearPending()
		v.Model, _ = v.Model.Update(msg)
		return
	}

	if ch >= '1' && ch <= '9' {
		v.count = v.count*10 + int(ch-'0')
		return
	}
	if ch == '0' {
		switch {
		case v.count > 0:
			v.count *= 10
			return
		case v.op == 0 && v.mot == 0:
			v.moveMot('0')
			return
		}
		// else: 0 is a motion operand for the pending d/c operator.
	}

	rs := v.runes()
	n := len(rs)
	p := v.normalPos(n)

	// d or c followed by f/F/t/T: the next char is the find operand.
	if v.mot != 0 {
		if k := v.findNth(rs, p, v.mot, ch); k >= 0 {
			s, e := findRange(p, k, v.mot)
			v.applyOp(s, e)
		}
		v.clearPending()
		return
	}

	switch v.op {
	case 'd', 'c':
		if ch == v.op {
			v.lineOp(ch == 'c')
			return
		}
		if ch == 'f' || ch == 'F' || ch == 't' || ch == 'T' {
			v.mot = ch
			return
		}
		if s, e, ok := v.opRange(ch, rs); ok {
			v.applyOp(s, e)
		}
		v.clearPending()
		return
	case 'r':
		v.replaceCh(rs, p, ch)
		return
	case 'f', 'F', 't', 'T':
		if k := v.findNth(rs, p, v.op, ch); k >= 0 {
			np := k
			switch v.op {
			case 't':
				np = clampInt(k+1, 0, n-1)
			case 'T':
				np = clampInt(k-1, 0, n-1)
			}
			v.setNormal(np)
		} else {
			v.clearPending()
		}
		return
	case 'g':
		if ch == 'g' {
			v.moveMot('0')
			return
		}
		v.clearPending()
		return
	}

	switch ch {
	case 'd', 'c', 'r', 'f', 'F', 't', 'T', 'g':
		v.op = ch
	case 'h', 'l', 'w', 'b', 'e', '$', '^', 'G':
		v.moveMot(ch)
	case 'x':
		v.deleteRange(p, min(n, p+v.cnt()), false)
	case 's':
		v.deleteRange(p, min(n, p+v.cnt()), true)
	case 'D':
		v.deleteRange(p, n, false)
	case 'C':
		v.deleteRange(p, n, true)
	case 'S':
		v.lineOp(true)
	case 'i':
		v.insertAt(p)
	case 'a':
		v.insertAt(min(p+1, n))
	case 'A':
		v.insertAt(n)
	case 'I':
		v.insertAt(min(firstNonBlank(rs), n))
	default:
		v.clearPending()
	}
}

// opRange is [s, e) of the characters an active d/c operator would
// remove for motion ch, in vim's half-open convention.
func (v *vimInput) opRange(mot rune, rs []rune) (int, int, bool) {
	n := len(rs)
	p := v.normalPos(n)
	c := v.cnt()
	if n == 0 {
		return 0, 0, false
	}
	switch mot {
	case 'h':
		return max(0, p-c), p, true
	case 'l':
		return p, min(n, p+c), true
	case 'w':
		e := nextWordStart(rs, p, c)
		if e <= p {
			e = n
		}
		return p, e, true
	case 'b':
		return prevWordStart(rs, p, c), p, true
	case 'e':
		return p, min(n, nextWordEnd(rs, p, c)+1), true
	case '0':
		return 0, p + 1, true
	case '^':
		s := min(firstNonBlank(rs), p)
		return s, p + 1, s <= p
	case '$':
		return p, n, p < n
	}
	return 0, 0, false
}

// findRange is [s, e) for operator + f/F/t/T onto index k.
func findRange(p, k int, mot rune) (int, int) {
	if k > p {
		if mot == 'f' {
			return p, k + 1
		}
		return p, k
	}
	if mot == 'F' {
		return k, p + 1
	}
	return k + 1, p + 1
}

func (v *vimInput) findNth(rs []rune, p int, mot, ch rune) int {
	dir := 1
	if mot == 'F' || mot == 'T' {
		dir = -1
	}
	k := p
	for range v.cnt() {
		if k = findTo(rs, k, dir, ch); k < 0 {
			return -1
		}
	}
	return k
}

func (v *vimInput) applyOp(s, e int) {
	v.deleteRange(s, e, v.op == 'c')
}

func (v *vimInput) deleteRange(s, e int, thenInsert bool) {
	rs := v.runes()
	s = clampInt(s, 0, len(rs))
	e = clampInt(e, 0, len(rs))
	v.clearPending()
	if e <= s {
		return
	}
	rr := append(append([]rune{}, rs[:s]...), rs[e:]...)
	v.SetValue(string(rr))
	if thenInsert {
		v.state = vimInsert
		v.SetCursor(s)
		return
	}
	v.SetCursor(min(s, max(len(rr)-1, 0)))
}

func (v *vimInput) lineOp(thenInsert bool) {
	v.SetValue("")
	v.clearPending()
	v.SetCursor(0)
	if thenInsert {
		v.state = vimInsert
	}
}

func (v *vimInput) replaceCh(rs []rune, p int, ch rune) {
	n := len(rs)
	if n > 0 {
		for i := p; i < min(n, p+v.cnt()); i++ {
			rs[i] = ch
		}
		v.SetValue(string(rs))
		v.SetCursor(p)
	}
	v.clearPending()
}

func (v *vimInput) moveMot(mot rune) {
	rs := v.runes()
	n := len(rs)
	p := v.normalPos(n)
	c := v.cnt()
	np := p
	if n > 0 {
		switch mot {
		case 'h':
			np = max(0, p-c)
		case 'l':
			np = min(n-1, p+c)
		case 'w':
			np = min(n-1, nextWordStart(rs, p, c))
		case 'b':
			np = prevWordStart(rs, p, c)
		case 'e':
			np = min(n-1, nextWordEnd(rs, p, c))
		case '0':
			np = 0
		case '^':
			np = min(firstNonBlank(rs), n-1)
		case '$', 'G':
			np = n - 1
		}
	}
	v.clearPending()
	v.SetCursor(np)
}

func (v *vimInput) setNormal(pos int) {
	v.clearPending()
	v.SetCursor(clampInt(pos, 0, max(len(v.runes())-1, 0)))
}

func (v *vimInput) insertAt(gap int) {
	v.clearPending()
	v.state = vimInsert
	v.SetCursor(gap)
}

// wordClass buckets runes the way vim's default word motions do:
// whitespace, alphanumeric (with _), and everything else.
func wordClass(r rune) int {
	switch {
	case unicode.IsSpace(r):
		return 0
	case unicode.IsLetter(r) || unicode.IsDigit(r) || r == '_':
		return 1
	default:
		return 2
	}
}

// wTo is the start of the next word after index i, or len(rs).
func wTo(rs []rune, i int) int {
	if i >= len(rs) {
		return i
	}
	if wordClass(rs[i]) == 0 {
		for i < len(rs) && wordClass(rs[i]) == 0 {
			i++
		}
		return i
	}
	c := wordClass(rs[i])
	for i < len(rs) && wordClass(rs[i]) == c {
		i++
	}
	for i < len(rs) && wordClass(rs[i]) == 0 {
		i++
	}
	return i
}

// eTo is the last index of the word ending after i.
func eTo(rs []rune, i int) int {
	if len(rs) == 0 || i >= len(rs)-1 {
		return max(i, 0)
	}
	end := runEnd(rs, i)
	if i < end {
		return end
	}
	j := end + 1
	for j < len(rs) && wordClass(rs[j]) == 0 {
		j++
	}
	if j >= len(rs) {
		return len(rs) - 1
	}
	return runEnd(rs, j)
}

// bTo is the start of the word before i.
func bTo(rs []rune, i int) int {
	if i <= 0 || len(rs) == 0 {
		return 0
	}
	j := i - 1
	for j > 0 && wordClass(rs[j]) == 0 {
		j--
	}
	if wordClass(rs[j]) == 0 {
		return 0
	}
	for j > 0 && wordClass(rs[j-1]) == wordClass(rs[j]) {
		j--
	}
	return j
}

func runEnd(rs []rune, i int) int {
	c := wordClass(rs[i])
	for i+1 < len(rs) && wordClass(rs[i+1]) == c {
		i++
	}
	return i
}

func findTo(rs []rune, i, dir int, ch rune) int {
	for j := i + dir; j >= 0 && j < len(rs); j += dir {
		if rs[j] == ch {
			return j
		}
	}
	return -1
}

func firstNonBlank(rs []rune) int {
	for i, r := range rs {
		if wordClass(r) != 0 {
			return i
		}
	}
	return 0
}

func nextWordStart(rs []rune, i, count int) int {
	for range count {
		i = wTo(rs, i)
	}
	return i
}

func nextWordEnd(rs []rune, i, count int) int {
	for range count {
		i = eTo(rs, i)
	}
	return i
}

func prevWordStart(rs []rune, i, count int) int {
	for range count {
		i = bTo(rs, i)
	}
	return i
}

// vimInputKeys is the contextual help while the field is focused; the
// bindings shown track the current vim mode.
type vimInputKeys struct{ insert bool }

func (k vimInputKeys) ShortHelp() []key.Binding {
	if k.insert {
		return []key.Binding{
			key.NewBinding(key.WithKeys("enter"), key.WithHelp("enter", "save")),
			key.NewBinding(key.WithKeys("esc"), key.WithHelp("esc", "normal mode")),
		}
	}
	return []key.Binding{
		key.NewBinding(key.WithKeys("i", "a", "I", "A"), key.WithHelp("i/a/I/A", "insert")),
		key.NewBinding(key.WithKeys("w", "b", "e"), key.WithHelp("w/b/e", "motion")),
		key.NewBinding(key.WithKeys("d", "c", "x", "s", "r"), key.WithHelp("d/c/x/s/r", "edit")),
		key.NewBinding(key.WithKeys("enter"), key.WithHelp("enter", "save")),
		key.NewBinding(key.WithKeys("esc"), key.WithHelp("esc", "cancel")),
	}
}

func (k vimInputKeys) FullHelp() [][]key.Binding {
	if k.insert {
		return [][]key.Binding{k.ShortHelp()}
	}
	return [][]key.Binding{
		{
			key.NewBinding(key.WithKeys("h", "l"), key.WithHelp("h/l", "left/right")),
			key.NewBinding(key.WithKeys("0", "$", "^"), key.WithHelp("0/$/^", "line pos")),
			key.NewBinding(key.WithKeys("f", "t"), key.WithHelp("f/t", "find forward")),
			key.NewBinding(key.WithKeys("d", "c"), key.WithHelp("d/c + motion", "delete/change")),
			key.NewBinding(key.WithKeys("D", "C", "S"), key.WithHelp("D/C/S", "to end of line")),
			key.NewBinding(key.WithKeys("1-9"), key.WithHelp("count", "repeat next op")),
		},
		k.ShortHelp(),
	}
}
