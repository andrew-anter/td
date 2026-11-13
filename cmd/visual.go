/*
Copyright © 2025 Andrew Anter <andrew.anter@gmail.com>
*/
package cmd

import (
	"fmt"
	"sort"
	"time"

	"charm.land/bubbles/v2/key"
	"charm.land/bubbles/v2/list"
	tea "charm.land/bubbletea/v2"

	"todo/task"
)

// Visual mode brings vim's selection to the task list: v anchors the
// selection at the cursor, motions extend it, and the list verbs
// (x, d, 1-3, m) act on every selected row at once. The selection is
// the contiguous range of displayed rows between anchor and cursor.

// markItems sets the done state of every addressed item at once.
func markItems(items []task.Item, fileIdxs []int, done bool) {
	for _, fi := range fileIdxs {
		items[fi].MarkDone(done, time.Now())
	}
}

// deleteItems removes every addressed item; descending order keeps
// the remaining indices valid as the slice shrinks.
func deleteItems(items []task.Item, fileIdxs []int) []task.Item {
	order := append([]int(nil), fileIdxs...)
	sort.Sort(sort.Reverse(sort.IntSlice(order)))
	for _, fi := range order {
		items = deleteItem(items, fi)
	}
	return items
}

// visualRange is the inclusive span of selected list indices.
func (m uiModel) visualRange() (int, int) {
	lo, hi := m.visualAnchor, m.list.Index()
	if lo > hi {
		lo, hi = hi, lo
	}
	hi = min(hi, len(m.list.Items())-1)
	return lo, hi
}

// visualFileIdxs maps the selection to data-file indices.
func (m uiModel) visualFileIdxs() []int {
	lo, hi := m.visualRange()
	items := m.list.Items()
	out := make([]int, 0, max(0, hi-lo+1))
	for i := lo; i <= hi && i < len(items); i++ {
		out = append(out, items[i].(uiItem).row.fileIdx)
	}
	return out
}

// applyVisual repaints the selection highlight on the displayed rows.
// Outside visual mode every flag clears.
func (m *uiModel) applyVisual() {
	lo, hi := m.visualRange()
	items := m.list.Items()
	lis := make([]list.Item, len(items))
	for i, li := range items {
		it := li.(uiItem)
		it.row.visual = m.mode == modeVisual && i >= lo && i <= hi
		lis[i] = it
	}
	m.list.SetItems(lis)
}

// enterVisual anchors the selection at the cursor. An in-flight
// completion animation or an applied filter is retired first so the
// selection addresses exactly the rows on screen.
func (m *uiModel) enterVisual() {
	if _, ok := m.selected(); !ok {
		m.status = "nothing to select"
		return
	}
	if m.anim != nil || m.list.FilterState() != list.Unfiltered {
		m.refresh(m.status)
	}
	m.mode = modeVisual
	m.visualAnchor = m.list.Index()
	m.applyVisual()
}

func (m *uiModel) exitVisual() {
	m.mode = modeNormal
	m.visualAnchor = -1
	m.applyVisual()
}

// visualToggle completes the whole selection when any of it is
// pending, otherwise reopens all of it — one keystroke, one save.
// Batch completion skips the per-row ghost animation but still flashes
// the completed tab.
func (m *uiModel) visualToggle() tea.Cmd {
	idxs := m.visualFileIdxs()
	if len(idxs) == 0 {
		return nil
	}
	done := false
	for _, fi := range idxs {
		if !m.items[fi].Done {
			done = true
			break
		}
	}
	markItems(m.items, idxs, done)
	n := len(idxs)
	m.exitVisual()
	state := "pending"
	if done {
		state = "done"
	}
	m.persist(fmt.Sprintf("%d tasks → %s", n, state))
	if done && m.view != viewDone {
		m.doneFlash = time.Now().Add(animFlash)
		return doneAnimTick()
	}
	return nil
}

// handleVisualKey owns the keyboard while a selection is live: batch
// verbs act on the range, motions extend it, and any other key drops
// the selection and falls through to normal handling.
func (m uiModel) handleVisualKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	keyName := msg.String()
	switch {
	case keyName == "esc" || keyName == "v":
		m.exitVisual()
		m.status = "visual cancelled"
		return m, nil

	case key.Matches(msg, m.keys.Toggle):
		return m, m.visualToggle()

	case key.Matches(msg, m.keys.Delete):
		idxs := m.visualFileIdxs()
		m.confirm = &pendingDelete{kind: deleteTask, fileIdxs: idxs, label: fmt.Sprintf("%d tasks", len(idxs))}
		m.mode = modeConfirm
		return m, nil

	case keyName == "1" || keyName == "2" || keyName == "3":
		pri := int(msg.Code) - '0'
		var tmp task.Item
		if err := tmp.SetPriority(pri); err != nil {
			m.status = err.Error()
			return m, nil
		}
		idxs := m.visualFileIdxs()
		for _, fi := range idxs {
			m.items[fi].Priority = pri
		}
		m.exitVisual()
		m.persist(fmt.Sprintf("%d tasks → priority %d", len(idxs), pri))
		return m, nil

	case key.Matches(msg, m.keys.Move):
		if len(m.moveTargets()) == 0 {
			m.status = "no other projects — press p to create one"
			return m, nil
		}
		m.moveFileIdxs = m.visualFileIdxs()
		m.mode = modeProjects
		m.projectsForMove = true
		m.pickerIdx = 0
		return m, nil

	case key.Matches(msg, m.list.KeyMap.CursorUp),
		key.Matches(msg, m.list.KeyMap.CursorDown),
		key.Matches(msg, m.list.KeyMap.NextPage),
		key.Matches(msg, m.list.KeyMap.PrevPage),
		key.Matches(msg, m.list.KeyMap.GoToStart),
		key.Matches(msg, m.list.KeyMap.GoToEnd):
		var cmd tea.Cmd
		m.list, cmd = m.list.Update(msg)
		m.applyVisual()
		return m, cmd
	}

	// Single-item verbs (edit, detail, …) and everything else leave
	// visual mode first, then act on the cursor row as usual.
	m.exitVisual()
	return m.handleKey(msg)
}

// visualKeys is the context help while a selection is live.
type visualKeys struct{}

func (visualKeys) ShortHelp() []key.Binding {
	return []key.Binding{
		key.NewBinding(key.WithKeys("j", "k"), key.WithHelp("j/k", "extend")),
		key.NewBinding(key.WithKeys("x"), key.WithHelp("x", "done")),
		key.NewBinding(key.WithKeys("d"), key.WithHelp("d", "delete")),
		key.NewBinding(key.WithKeys("1", "2", "3"), key.WithHelp("1-3", "priority")),
		key.NewBinding(key.WithKeys("m"), key.WithHelp("m", "move")),
		key.NewBinding(key.WithKeys("esc"), key.WithHelp("esc", "cancel")),
	}
}

func (k visualKeys) FullHelp() [][]key.Binding { return [][]key.Binding{k.ShortHelp()} }
