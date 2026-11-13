/*
Copyright © 2025 Andrew Anter <andrew.anter@gmail.com>
*/
package cmd

import (
	"time"

	tea "charm.land/bubbletea/v2"
)

// Completing a task is choreographed instead of instant: the row stays
// put while a strikethrough sweeps across its text, holds long enough
// to read, then — in the pending view — slides away and fades out as it
// "lands" in the completed tab, which flashes green. The toggle is
// persisted before the first frame, so quitting mid-animation never
// loses the change; the animation only replays what is already saved.
const (
	animFrame      = 30 * time.Millisecond  // ~33 fps
	animSweep      = 260 * time.Millisecond // strikethrough draws left → right
	animHold       = 140 * time.Millisecond // fully struck, still in place
	animExit       = 220 * time.Millisecond // slide right + fade (pending view)
	animFlash      = 800 * time.Millisecond
	animSlideCells = 10 // how far the departing row drifts right
)

// doneAnimTickMsg drives one animation frame; the same tick keeps the
// completed-tab flash alive after the row itself is gone.
type doneAnimTickMsg struct{}

func doneAnimTick() tea.Cmd {
	return tea.Tick(animFrame, func(time.Time) tea.Msg { return doneAnimTickMsg{} })
}

// doneAnim tracks one in-flight completion. fileIdx addresses the real
// (already saved) item; in the pending view a ghost row with the same
// fileIdx holds its slot until the exit finishes. In the all view the
// real row stays and only the sweep plays.
type doneAnim struct {
	fileIdx int
	start   time.Time
}

func (a doneAnim) total(view viewMode) time.Duration {
	if view == viewAll {
		return animSweep + animHold
	}
	return animSweep + animHold + animExit
}

// animProgress maps elapsed time to the strike fraction (ease-out, so
// the line lands softly) and the exit fraction (linear here; Render
// squares it so the row accelerates away).
func animProgress(elapsed time.Duration, view viewMode) (strike, exit float64) {
	p := float64(elapsed) / float64(animSweep)
	if p > 1 {
		p = 1
	}
	strike = 1 - (1-p)*(1-p)
	if view == viewAll {
		return strike, 0
	}
	e := float64(elapsed-animSweep-animHold) / float64(animExit)
	if e < 0 {
		e = 0
	}
	if e > 1 {
		e = 1
	}
	return strike, e
}

// beginCompletion starts the animation for the row just toggled done.
// The data is already persisted and the filter reset by the refresh,
// so the list is unfiltered here; this only choreographs the view. The
// cursor keeps the next pending task so work can continue while the
// ghost departs.
func (m *uiModel) beginCompletion(row uiRow, listIdx int) tea.Cmd {
	if m.view == viewDone {
		return nil
	}
	m.anim = &doneAnim{fileIdx: row.fileIdx, start: time.Now()}
	if m.view == viewPending {
		ghost := row
		ghost.item = m.items[row.fileIdx] // done, with its completion date
		ghost.strike = 0.001              // sweep path from the first frame
		at := min(listIdx, len(m.list.Items()))
		keep := -1
		if sel, ok := m.list.SelectedItem().(uiItem); ok {
			keep = sel.row.fileIdx
		}
		m.list.InsertItem(at, uiItem{row: ghost})
		// Insertion shifts everything below it; park the cursor back on
		// the task it had, not on the departing ghost.
		if keep >= 0 {
			for i, li := range m.list.Items() {
				if li.(uiItem).row.fileIdx == keep {
					m.list.Select(i)
					break
				}
			}
		}
	}
	return doneAnimTick()
}

// stepDoneAnim advances one frame and returns the command that keeps
// the animation ticking. Any explicit refresh (view switch, another
// mutation, reload) cancels the animation by rebuilding the rows, so
// this only ever sees a stable list.
func (m *uiModel) stepDoneAnim() tea.Cmd {
	now := time.Now()
	if m.anim != nil {
		a := m.anim
		if now.Sub(a.start) >= a.total(m.view) {
			m.anim = nil
			m.doneFlash = now.Add(animFlash)
			cmd := m.refreshWith(m.status, true)
			return tea.Batch(cmd, doneAnimTick())
		}
		strike, exit := animProgress(now.Sub(a.start), m.view)
		for i, li := range m.list.Items() {
			it := li.(uiItem)
			if it.row.fileIdx != a.fileIdx {
				continue
			}
			it.row.strike, it.row.exit = strike, exit
			m.list.SetItem(i, it)
			break
		}
		return doneAnimTick()
	}
	if now.Before(m.doneFlash) {
		return doneAnimTick()
	}
	return nil
}
