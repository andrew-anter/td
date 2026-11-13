/*
Copyright © 2025 Andrew Anter <andrew.anter@gmail.com>
*/
package cmd

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"sort"
	"strings"
	"time"

	"charm.land/bubbles/v2/help"
	"charm.land/bubbles/v2/key"
	"charm.land/bubbles/v2/list"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"todo/gitlab"
	"todo/task"
)

// viewMode selects which slice of the list the UI displays.
type viewMode int

const (
	viewPending viewMode = iota
	viewAll
	viewDone
)

func (v viewMode) label() string {
	switch v {
	case viewAll:
		return "all"
	case viewDone:
		return "completed"
	default:
		return "pending"
	}
}

// next/prev cycle the view with tab / shift+tab.
func (v viewMode) next() viewMode {
	if v == viewDone {
		return viewPending
	}
	return v + 1
}

func (v viewMode) prev() viewMode {
	if v == viewPending {
		return viewDone
	}
	return v - 1
}

// uiRow is one visible list row. num is the item's position in the
// FULL sorted list — the same global number `td complete/delete/edit`
// accepts — and fileIdx is its index in the data-file slice.
type uiRow struct {
	num     int
	fileIdx int
	item    task.Item
	// strike/exit drive the completion animation (doneanim.go): strike
	// is the fraction of the text already crossed out, exit the
	// fade-and-slide progress of a departing row. Both zero = static.
	strike float64
	exit   float64
	// visual marks rows inside the visual-mode selection.
	visual bool
}

// visibleRows returns the rows for the given view, numbered and
// ordered exactly like `td list` (pending first, priority ascending,
// stable).
func visibleRows(items []task.Item, view viewMode) []uiRow {
	order := sortedOrderIndices(items)
	rows := make([]uiRow, 0, len(order))
	for pos, fi := range order {
		it := items[fi]
		switch view {
		case viewPending:
			if it.Done {
				continue
			}
		case viewDone:
			if !it.Done {
				continue
			}
		}
		rows = append(rows, uiRow{num: pos + 1, fileIdx: fi, item: it})
	}
	return rows
}

// uiItem adapts a uiRow to the bubbles list.Item interface. Title is
// the rendered label; FilterValue is what relevanceFilter searches.
type uiItem struct{ row uiRow }

func (i uiItem) Title() string       { return i.row.item.Text }
func (i uiItem) Description() string { return "" }

// FilterValue includes the description so `/` finds detail that only
// lives there. The newline keeps a word from matching across the two
// fields.
func (i uiItem) FilterValue() string {
	if d := i.row.item.Description; d != "" {
		return i.row.item.Text + "\n" + d
	}
	return i.row.item.Text
}

// Column widths are shared with columnHeader so the labels stay over
// their columns.
const (
	doneColW = 4
	priColW  = 3
	dateColW = 10
)

// rowDelegate renders a row: global number, done marker, priority,
// creation date, completion date, text.
type rowDelegate struct{ dark bool }

func (d rowDelegate) Height() int                               { return 1 }
func (d rowDelegate) Spacing() int                              { return 0 }
func (d rowDelegate) Update(msg tea.Msg, m *list.Model) tea.Cmd { return nil }

func (d rowDelegate) Render(w io.Writer, m list.Model, index int, listItem list.Item) {
	it, ok := listItem.(uiItem)
	if !ok {
		return
	}
	text := it.row.item.Text
	num := numStyle().Render(fmt.Sprintf("%4d.", it.row.num))
	done := doneStyle().Render(fmt.Sprintf("%-*s", doneColW, strings.TrimSpace(it.row.item.PrettyDone())))
	pri := priStyle(it.row.item.Priority).Render(fmt.Sprintf("%-*s", priColW, strings.TrimSpace(it.row.item.PrettyP())))
	created := dimStyle().Render(fmt.Sprintf("%-*s", dateColW, it.row.item.PrettyCreated()))
	completed := dimStyle().Render(fmt.Sprintf("%-*s", dateColW, it.row.item.PrettyCompleted()))
	sep := " "
	if it.row.visual {
		// Back each column and the gaps individually: one wrap around
		// the whole line would let the columns' inner resets punch
		// unstyled holes through the selection background.
		vs := visualStyle(d.dark)
		num, done, pri, created, completed = vs.Render(num), vs.Render(done), vs.Render(pri), vs.Render(created), vs.Render(completed)
		sep = vs.Render(" ")
	}
	prefix := num + sep + done + sep + pri + sep + created + sep + completed + sep
	if w := m.Width(); w > 0 {
		if avail := w - 2 - lipgloss.Width(prefix); avail > 0 {
			text = fitCells(text, avail)
		}
	}
	switch {
	case it.row.exit > 0:
		// Departing row: the column colors collapse into one gray that
		// fades toward the background while the row drifts right.
		plain := fmt.Sprintf("%4d. %-*s %-*s %-*s %-*s ",
			it.row.num,
			doneColW, strings.TrimSpace(it.row.item.PrettyDone()),
			priColW, strings.TrimSpace(it.row.item.PrettyP()),
			dateColW, it.row.item.PrettyCreated(),
			dateColW, it.row.item.PrettyCompleted())
		st := exitStyle(it.row.exit, d.dark)
		line := strings.Repeat(" ", int(it.row.exit*it.row.exit*animSlideCells)) +
			st.Render(plain) + st.Strikethrough(true).Render(text)
		if w := m.Width(); w > 0 {
			line = fitCells(line, w-2)
		}
		if index == m.Index() {
			fmt.Fprint(w, selectedStyle().Render("▸ "+line))
			return
		}
		fmt.Fprint(w, "  "+line)
		return
	case it.row.strike > 0:
		// Sweep: the struck prefix grows left → right across the text.
		n := int(it.row.strike * float64(lipgloss.Width(text)))
		text = dimStyle().Strikethrough(true).Render(ansi.Truncate(text, n, "")) +
			ansi.TruncateLeft(text, n, "")
	case it.row.item.Done:
		text = dimStyle().Strikethrough(true).Render(text)
	}
	if it.row.visual {
		text = visualStyle(d.dark).Render(text)
	}
	line := prefix + text
	if index == m.Index() {
		fmt.Fprint(w, selectedStyle().Render("▸ "+line))
		return
	}
	fmt.Fprint(w, "  "+line)
}

// Pure mutation helpers — everything that touches items goes through
// these so the mapping is covered by tests.
func toggleItem(items []task.Item, fileIdx int) {
	items[fileIdx].MarkDone(!items[fileIdx].Done, time.Now())
}

func deleteItem(items []task.Item, fileIdx int) []task.Item {
	return append(items[:fileIdx], items[fileIdx+1:]...)
}

type inputMode int

const (
	modeNormal inputMode = iota
	modeAdd
	modeEdit
	modeDescribe    // wrapped field editing the selected item's description
	modeProjects    // full-page projects screen (p to manage, m to pick a move target)
	modeNewProject  // textinput for a new project name
	modeRename      // textinput renaming a project
	modeDetail      // full read-only view of the focused task
	modeConfirm     // delete awaiting y/n
	modeVisual      // vim-style selection over the task list
	modeGitLab      // full-page GitLab configuration
	modeGitLabURL   // textinput for the GitLab instance URL
	modeGitLabToken // textinput for the access token
	modeGitLabTd    // textinput for the td side of a new link
	modeGitLabRef   // textinput for the GitLab project path of a link
)

// isGitLabInput reports whether mode is one of the config page's text
// fields.
func isGitLabInput(mode inputMode) bool {
	switch mode {
	case modeGitLabURL, modeGitLabToken, modeGitLabTd, modeGitLabRef:
		return true
	}
	return false
}

// deleteKind selects the object a modeConfirm prompt will remove.
type deleteKind int

const (
	deleteTask deleteKind = iota
	deleteProject
)

// pendingDelete is the destructive action held behind modeConfirm.
// The label is captured when the prompt opens so the text cannot drift
// while the answer is pending.
type pendingDelete struct {
	kind     deleteKind
	fileIdxs []int
	project  string
	label    string
}

func (p pendingDelete) prompt() string {
	if p.kind == deleteProject {
		return fmt.Sprintf("delete project %q? y confirm · n cancel", p.project)
	}
	if len(p.fileIdxs) == 1 {
		return fmt.Sprintf("delete %q? y confirm · n cancel", p.label)
	}
	return fmt.Sprintf("delete %s? y confirm · n cancel", p.label)
}

type uiKeys struct {
	Quit         key.Binding
	Detail       key.Binding
	Toggle       key.Binding
	Visual       key.Binding
	Add          key.Binding
	Edit         key.Binding
	EditExternal key.Binding
	Describe     key.Binding
	Delete       key.Binding
	Pri1         key.Binding
	Pri2         key.Binding
	Pri3         key.Binding
	PrevProject  key.Binding
	NextProject  key.Binding
	Pick         key.Binding
	Move         key.Binding
	Cycle        key.Binding
	CycleRev     key.Binding
	Reload       key.Binding
	Filter       key.Binding
	GitLab       key.Binding
	Help         key.Binding
}

func (k uiKeys) ShortHelp() []key.Binding {
	return []key.Binding{k.Add, k.Detail, k.Toggle, k.Visual, k.Edit, k.EditExternal, k.Describe, k.Delete, k.Move, k.Pick, k.Cycle, k.Filter, k.Quit}
}

func (k uiKeys) FullHelp() [][]key.Binding {
	return [][]key.Binding{
		{k.Add, k.Detail, k.Edit, k.EditExternal, k.Describe, k.Delete, k.Toggle, k.Visual},
		{k.Pri1, k.Pri2, k.Pri3, k.Move, k.Pick, k.PrevProject, k.NextProject},
		{k.Cycle, k.CycleRev, k.Filter, k.Reload, k.GitLab, k.Help, k.Quit},
	}
}

func newUIKeys() uiKeys {
	return uiKeys{
		Quit:         key.NewBinding(key.WithKeys("q", "esc", "ctrl+c"), key.WithHelp("q", "quit")),
		Detail:       key.NewBinding(key.WithKeys("enter"), key.WithHelp("enter", "details")),
		Toggle:       key.NewBinding(key.WithKeys("x", "space"), key.WithHelp("x", "toggle done")),
		Visual:       key.NewBinding(key.WithKeys("v"), key.WithHelp("v", "visual select")),
		Add:          key.NewBinding(key.WithKeys("a"), key.WithHelp("a", "add")),
		Edit:         key.NewBinding(key.WithKeys("e"), key.WithHelp("e", "edit")),
		EditExternal: key.NewBinding(key.WithKeys("E"), key.WithHelp("E", "edit in $EDITOR")),
		Describe:     key.NewBinding(key.WithKeys("D"), key.WithHelp("D", "describe")),
		Delete:       key.NewBinding(key.WithKeys("d"), key.WithHelp("d", "delete")),
		Pri1:         key.NewBinding(key.WithKeys("1"), key.WithHelp("1", "high pri")),
		Pri2:         key.NewBinding(key.WithKeys("2"), key.WithHelp("2", "med pri")),
		Pri3:         key.NewBinding(key.WithKeys("3"), key.WithHelp("3", "low pri")),
		PrevProject:  key.NewBinding(key.WithKeys("["), key.WithHelp("[", "prev project")),
		NextProject:  key.NewBinding(key.WithKeys("]"), key.WithHelp("]", "next project")),
		Pick:         key.NewBinding(key.WithKeys("p"), key.WithHelp("p", "projects")),
		Move:         key.NewBinding(key.WithKeys("m"), key.WithHelp("m", "move to project")),
		Cycle:        key.NewBinding(key.WithKeys("tab"), key.WithHelp("tab", "next view")),
		CycleRev:     key.NewBinding(key.WithKeys("shift+tab"), key.WithHelp("shift+tab", "prev view")),
		Reload:       key.NewBinding(key.WithKeys("r"), key.WithHelp("r", "reload")),
		Filter:       key.NewBinding(key.WithKeys("/"), key.WithHelp("/", "filter")),
		GitLab:       key.NewBinding(key.WithKeys("g"), key.WithHelp("g", "gitlab sync")),
		Help:         key.NewBinding(key.WithKeys("?", "f1"), key.WithHelp("?", "help")),
	}
}

type uiModel struct {
	path    string
	store   task.Store
	project string
	items   []task.Item

	view viewMode
	mode inputMode
	// editingFileIdx is the file index being edited in modeEdit.
	editingFileIdx int
	// confirm holds the delete waiting behind modeConfirm.
	confirm *pendingDelete
	// describeLoaded holds what the D field was opened with, so an
	// untouched commit cannot flatten a description that still has
	// paragraph breaks stored by the external editor.
	describeLoaded string
	// pickerIdx is the cursor row on the projects screen.
	pickerIdx int
	// projectsForMove marks the projects screen opened by m (pick a
	// destination) rather than p (manage projects).
	projectsForMove bool
	// renameTarget is the project name being renamed in modeRename.
	renameTarget string
	// visualAnchor is the list index where visual mode started; -1
	// when no selection is live. moveFileIdxs carries a visual-mode
	// batch into the projects screen for m.
	visualAnchor int
	moveFileIdxs []int

	// configPath is the td config file the GitLab page writes; empty
	// when the page is unavailable.
	configPath string
	// gitlab holds the config file's own GitLab settings, so edits made
	// in the TUI never persist values that came from the environment.
	gitlab    gitlab.Settings
	gitlabEnv gitlab.Env
	// gitlabIdx is the cursor row on the GitLab page: 0 url, 1 token,
	// 2 and up links.
	gitlabIdx int
	// linkDraft is the td project name while the GitLab path input
	// that follows it is open.
	linkDraft string

	list   list.Model
	input  vimInput
	help   help.Model
	keys   uiKeys
	status string
	width  int
	height int
	isDark bool
	// anim choreographs a just-completed row; doneFlash is the deadline
	// of the completed tab's green flash (see doneanim.go).
	anim      *doneAnim
	doneFlash time.Time
}

// newUIModel builds the TUI over an already-loaded snapshot. When the
// named project is missing from the store, the model falls back to the
// default namespace.
func newUIModel(path string, store task.Store, project string) uiModel {
	if store.Project(project) == nil {
		project = task.DefaultProject
	}
	p := store.EnsureProject(project)

	l := list.New([]list.Item{}, rowDelegate{dark: true}, 0, 0)
	l.SetShowTitle(false)
	l.SetShowStatusBar(false)
	l.SetShowPagination(false)
	l.SetShowHelp(false)
	l.SetFilteringEnabled(true)
	l.Filter = relevanceFilter

	ti := newVimInput()

	h := help.New()

	m := uiModel{
		path:         path,
		store:        store,
		project:      project,
		items:        p.Items,
		list:         l,
		input:        ti,
		help:         h,
		keys:         newUIKeys(),
		isDark:       true,
		visualAnchor: -1,
	}
	m.refresh("")
	return m
}

// visibleRows returns the current view's rows with global numbers.
func (m uiModel) visibleRows() []uiRow {
	return visibleRows(m.items, m.view)
}

// projectNames lists the namespaces the switcher cycles through. An
// empty store still reports the default project so the TUI is usable
// before anything exists.
func (m uiModel) projectNames() []string {
	if len(m.store.Projects) == 0 {
		return []string{task.DefaultProject}
	}
	return m.store.ProjectNames()
}

// setProject points the model at a namespace without touching disk.
func (m *uiModel) setProject(name string) {
	m.project = name
	m.items = m.store.EnsureProject(name).Items
}

// createProjectInPlace validates and adds a project, saves, and parks
// the cursor on it — the projects screen stays open so you can keep
// managing.
func (m *uiModel) createProjectInPlace(name string) error {
	if err := task.ValidateProjectName(name); err != nil {
		return err
	}
	if m.store.Project(name) != nil {
		return fmt.Errorf("project %q already exists", name)
	}
	m.store.EnsureProject(name)
	m.pickerIdx = len(m.store.Projects) - 1
	m.persist(fmt.Sprintf("created project %q", name))
	return nil
}

// renameProject changes the cursor's project (renameTarget) to a new
// name, keeping its items; the view follows if the current project is
// the one renamed.
func (m *uiModel) renameProject(newName string) error {
	if err := task.ValidateProjectName(newName); err != nil {
		return err
	}
	if m.store.Project(newName) != nil {
		return fmt.Errorf("project %q already exists", newName)
	}
	p := m.store.Project(m.renameTarget)
	if p == nil {
		return fmt.Errorf("no such project %q", m.renameTarget)
	}
	p.Name = newName
	if m.project == m.renameTarget {
		m.project = newName
	}
	m.persist(fmt.Sprintf("renamed project to %q", newName))
	return nil
}

// deleteProject removes a project, but only once it is empty —
// silently destroying a list of tasks with one keystroke is not on.
func (m *uiModel) deleteProject(name string) {
	p := m.store.Project(name)
	if p == nil {
		return
	}
	if len(p.Items) > 0 {
		m.status = fmt.Sprintf("project %q has %d item(s) — move or delete them first", name, len(p.Items))
		return
	}
	m.store.RemoveProject(name)
	names := m.projectNames()
	if len(names) == 0 {
		names = []string{task.DefaultProject}
	}
	if name == m.project {
		m.setProject(names[0])
	}
	if m.pickerIdx >= len(names) {
		m.pickerIdx = len(names) - 1
	}
	m.persist(fmt.Sprintf("deleted project %q", name))
}

// moveTargets lists the namespaces the selected item could move to:
// every project except the current one.
func (m uiModel) moveTargets() []string {
	names := m.projectNames()
	out := make([]string, 0, len(names))
	for _, n := range names {
		if n != m.project {
			out = append(out, n)
		}
	}
	return out
}

// panelNames is the row set of the projects screen: every project,
// or — when opened by m — just the other ones.
func (m uiModel) panelNames() []string {
	if m.projectsForMove {
		return m.moveTargets()
	}
	return m.projectNames()
}

// moveSelectedTo relocates the item under the cursor — or the whole
// visual-mode batch carried in moveFileIdxs — into dest, keeping
// priority and done state, and saves immediately.
func (m *uiModel) moveSelectedTo(dest string) {
	idxs := m.moveFileIdxs
	m.moveFileIdxs = nil
	if len(idxs) == 0 {
		it, ok := m.selected()
		if !ok {
			return
		}
		idxs = []int{it.row.fileIdx}
	}
	d := m.store.Project(dest)
	if d == nil {
		m.status = fmt.Sprintf("no such project %q", dest)
		return
	}
	moved := make([]task.Item, 0, len(idxs))
	for _, fi := range idxs {
		moved = append(moved, m.items[fi])
	}
	src := m.store.EnsureProject(m.project)
	src.Items = deleteItems(src.Items, idxs)
	d.Items = append(d.Items, moved...)
	m.items = src.Items
	if len(idxs) == 1 {
		m.persist(fmt.Sprintf("moved %q to %q", moved[0].Text, dest))
	} else {
		m.persist(fmt.Sprintf("moved %d tasks to %q", len(idxs), dest))
	}
}

// switchProject moves the view to the delta-th next project, wrapping
// around. Unsaved edits to the previous project are kept in the store
// in memory and persist on the next mutation.
func (m *uiModel) switchProject(delta int) {
	names := m.projectNames()
	k := 0
	for i, n := range names {
		if n == m.project {
			k = i
			break
		}
	}
	m.setProject(names[(k+delta+len(names))%len(names)])
	m.refresh("")
}

func (m *uiModel) refresh(status string) {
	m.refreshWith(status, false)
}

// refreshWith rebuilds the visible rows and returns the command the
// list needs to re-apply an applied filter. keepFilter leaves that
// search alone, for background updates that must not disturb what the
// user typed; explicit mutations and view switches clear it.
func (m *uiModel) refreshWith(status string, keepFilter bool) tea.Cmd {
	m.status = status
	// A rebuild replaces every displayed row, which retires any
	// in-flight completion ghost along with it.
	m.anim = nil
	if m.mode == modeVisual {
		m.mode = modeNormal
	}
	m.visualAnchor = -1
	rows := visibleRows(m.items, m.view)

	// Keep the cursor on the same underlying item when possible.
	var keepFileIdx = -1
	if sel, ok := m.list.SelectedItem().(uiItem); ok {
		keepFileIdx = sel.row.fileIdx
	}

	lis := make([]list.Item, len(rows))
	for k, r := range rows {
		lis[k] = uiItem{row: r}
	}
	// SetItems kick off an async re-filter against any stale filter
	// text; discard it — view switches show everything.
	if !keepFilter {
		m.list.ResetFilter()
	}
	cmd := m.list.SetItems(lis)

	found := false
	if keepFileIdx >= 0 {
		for k := range lis {
			if lis[k].(uiItem).row.fileIdx == keepFileIdx {
				m.list.Select(k)
				found = true
				break
			}
		}
	}
	if !found {
		// The item under the cursor is gone (completed, deleted):
		// hold the screen position, clamped into the new list — the
		// list itself only clamps pages, not the cursor.
		m.list.Select(min(m.list.Index(), max(0, len(lis)-1)))
	}
	return cmd
}

// persist writes the current snapshot back into the store and to disk
// under the shared data-file lock.
func (m *uiModel) persist(action string) {
	unlock, err := task.Lock(m.path)
	if err != nil {
		m.status = fmt.Sprintf("lock failed: %v", err)
		return
	}
	defer unlock.Close()

	m.store.EnsureProject(m.project).Items = m.items
	if err := task.SaveStore(m.path, m.store); err != nil {
		m.status = fmt.Sprintf("save failed: %v", err)
		return
	}
	m.refresh(action)
}

// gitlabLinkNames lists the linked td projects in display order.
func (m uiModel) gitlabLinkNames() []string {
	names := make([]string, 0, len(m.gitlab.Links))
	for n := range m.gitlab.Links {
		names = append(names, n)
	}
	sort.Strings(names)
	return names
}

// openGitLab shows the configuration page, starting on the current
// project's link when it has one.
func (m *uiModel) openGitLab() {
	m.mode = modeGitLab
	m.gitlabIdx = 0
	for i, n := range m.gitlabLinkNames() {
		if n == m.project {
			m.gitlabIdx = i + 2
			break
		}
	}
}

// saveGitLab persists the settings and adopts them only when the write
// succeeded, so the page never shows state that is not on disk.
func (m *uiModel) saveGitLab(next gitlab.Settings) {
	if m.configPath == "" {
		m.status = "no config file to write"
		return
	}
	if err := gitlab.SaveSettings(m.configPath, next); err != nil {
		m.status = fmt.Sprintf("save config: %v", err)
		return
	}
	m.gitlab = next
	m.status = "saved " + m.configPath
}

// editGitLabRow opens the field or link under the cursor.
func (m *uiModel) editGitLabRow() {
	names := m.gitlabLinkNames()
	m.input.Placeholder = ""
	switch {
	case m.gitlabIdx == 0:
		m.mode = modeGitLabURL
		m.input.SetValue(m.gitlab.URL)
	case m.gitlabIdx == 1:
		m.mode = modeGitLabToken
		m.input.SetValue(m.gitlab.Token)
	default:
		k := m.gitlabIdx - 2
		if k >= len(names) {
			return
		}
		m.linkDraft = names[k]
		m.mode = modeGitLabRef
		m.input.SetValue(m.gitlab.Links[names[k]])
	}
	m.input.Enter()
	m.input.Focus()
}

// commitGitLabInput applies one page field. Empty url/token clears
// it; adding a link validates the td project before asking for the
// GitLab path that follows.
func (m *uiModel) commitGitLabInput(text string) {
	switch m.mode {
	case modeGitLabURL, modeGitLabToken:
		next := m.gitlab
		if m.mode == modeGitLabURL {
			if text != "" {
				if err := gitlab.ValidateURL(text); err != nil {
					m.status = err.Error()
					m.mode = modeGitLab
					return
				}
			}
			next.URL = text
		} else {
			next.Token = text
		}
		m.saveGitLab(next)
		m.mode = modeGitLab

	case modeGitLabTd:
		if text == "" {
			m.status = "link cancelled"
			m.mode = modeGitLab
			return
		}
		if m.store.Project(text) == nil {
			m.status = fmt.Sprintf("no project %q — create it first (p)", text)
			m.mode = modeGitLab
			return
		}
		m.linkDraft = text
		m.mode = modeGitLabRef
		m.input.SetValue("")
		m.input.Enter()
		m.input.Placeholder = "group/project"
		m.input.Focus()

	case modeGitLabRef:
		path := gitlab.NormalizeProjectPath(text)
		if path == "" {
			m.status = "link cancelled"
			m.mode = modeGitLab
			return
		}
		if err := gitlab.ValidateProjectPath(path); err != nil {
			m.status = err.Error()
			m.mode = modeGitLab
			return
		}
		if other, ok := m.gitlab.LinkedTo(path, m.linkDraft); ok {
			m.status = fmt.Sprintf("%s is already linked to %q", path, other)
			m.mode = modeGitLab
			return
		}
		next := m.gitlab
		links := make(map[string]string, len(next.Links)+1)
		for k, v := range next.Links {
			links[k] = v
		}
		links[m.linkDraft] = path
		next.Links = links
		m.saveGitLab(next)
		for i, n := range m.gitlabLinkNames() {
			if n == m.linkDraft {
				m.gitlabIdx = i + 2
				break
			}
		}
		m.mode = modeGitLab
	}
}

// removeGitLabLink drops one link. Items keep their issue fields, so
// relinking the same GitLab project resumes the sync.
func (m *uiModel) removeGitLabLink(name string) {
	next := m.gitlab
	links := make(map[string]string, len(next.Links))
	for k, v := range next.Links {
		if k != name {
			links[k] = v
		}
	}
	next.Links = links
	m.saveGitLab(next)
	if last := 1 + len(m.gitlabLinkNames()); m.gitlabIdx > last {
		m.gitlabIdx = last
	}
}

// gitlabSyncedMsg carries a finished background sync.
type gitlabSyncedMsg struct {
	reports []syncReport
	store   task.Store
	readErr error
}

// gitlabSyncCmd syncs every link in the background; nil when the
// integration is unconfigured, so a local-only td never touches the
// network.
func (m uiModel) gitlabSyncCmd() tea.Cmd {
	settings := m.gitlab.WithEnv()
	if !settings.Configured() || len(settings.Links) == 0 {
		return nil
	}
	dataPath := m.path
	return func() tea.Msg {
		reports, store, err := syncLinks(dataPath, settings, nil)
		return gitlabSyncedMsg{reports: reports, store: store, readErr: err}
	}
}

// startupSyncCmd syncs on open only when the project on screen is
// linked: opening td on a local-only project stays offline.
func (m uiModel) startupSyncCmd() tea.Cmd {
	if _, ok := m.gitlab.Link(m.project); !ok {
		return nil
	}
	return m.gitlabSyncCmd()
}

// gitlabSyncStatus renders a finished sync for the status line.
func gitlabSyncStatus(msg gitlabSyncedMsg) string {
	if msg.readErr != nil {
		return "gitlab: " + msg.readErr.Error()
	}
	var total syncReport
	failed := 0
	var firstErr error
	for _, r := range msg.reports {
		if r.Err != nil {
			failed++
			if firstErr == nil {
				firstErr = r.Err
			}
			continue
		}
		total.Imported += r.Imported
		total.Completed += r.Completed
		total.Closed += r.Closed
		total.Reopened += r.Reopened
		total.Skipped += r.Skipped
	}
	if failed > 0 {
		return fmt.Sprintf("gitlab: %d of %d links failed: %v", failed, len(msg.reports), firstErr)
	}
	return "gitlab: " + total.changeSummary()
}

// gitlabView renders the configuration page: the instance URL, the
// token, and one row per project link.
func (m uiModel) gitlabView() string {
	names := m.gitlabLinkNames()
	labelW := 10
	for _, n := range names {
		labelW = max(labelW, ansi.StringWidth(n))
	}
	row := func(idx int, label, value string) string {
		cursor := "  "
		if idx == m.gitlabIdx {
			cursor = "▸ "
		}
		pad := strings.Repeat(" ", max(0, labelW-ansi.StringWidth(label)))
		line := cursor + label + pad + "  " + value
		if idx == m.gitlabIdx {
			return selectedStyle().Render(line)
		}
		return line
	}

	url := m.gitlab.URL
	if url == "" {
		url = dimStyle().Render("(not set)")
	}
	url += envNote(m.gitlabEnv.URL, "$TODO_GITLAB_URL")
	token := maskToken(m.gitlab.Token) + envNote(m.gitlabEnv.Token, "$TODO_GITLAB_TOKEN")

	lines := []string{
		"  " + projectStyle().Render("gitlab"),
		"",
		row(0, "url", url),
		row(1, "token", token),
		"",
		"  " + headerStyle().Render("links"),
	}
	if len(names) == 0 {
		lines = append(lines, "  "+dimStyle().Render("none yet — press a to link a td project"))
	}
	for i, n := range names {
		lines = append(lines, row(i+2, n, "→ "+m.gitlab.Links[n]))
	}
	if !m.gitlab.WithEnv().Configured() {
		lines = append(lines, "", "  "+dimStyle().Render("td works without this — fill in url and token to sync"))
	}
	return lipgloss.JoinVertical(lipgloss.Left, lines...)
}

// gitlabKeys is the context help for the GitLab page.
type gitlabKeys struct{}

func (gitlabKeys) ShortHelp() []key.Binding {
	return []key.Binding{
		key.NewBinding(key.WithKeys("enter"), key.WithHelp("enter", "edit")),
		key.NewBinding(key.WithKeys("a"), key.WithHelp("a", "add link")),
		key.NewBinding(key.WithKeys("d"), key.WithHelp("d", "remove link")),
		key.NewBinding(key.WithKeys("s"), key.WithHelp("s", "sync now")),
		key.NewBinding(key.WithKeys("j", "k"), key.WithHelp("j/k", "move")),
		key.NewBinding(key.WithKeys("esc", "q", "g"), key.WithHelp("esc", "back to tasks")),
	}
}

func (gitlabKeys) FullHelp() [][]key.Binding { return [][]key.Binding{gitlabKeys{}.ShortHelp()} }

func (m uiModel) Init() tea.Cmd {
	cmds := []tea.Cmd{tea.RequestBackgroundColor}
	if cmd := m.startupSyncCmd(); cmd != nil {
		cmds = append(cmds, cmd)
	}
	return tea.Batch(cmds...)
}

// selected returns the row under the cursor, if any.
func (m uiModel) selected() (uiItem, bool) {
	it, ok := m.list.SelectedItem().(uiItem)
	return it, ok
}

// editTaskFinishedMsg arrives when the $EDITOR spawned by E exits.
type editTaskFinishedMsg struct {
	err     error
	path    string
	fileIdx int
}

func (m uiModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		m.help.SetWidth(msg.Width)
		m.resizeList()
		return m, nil

	case editTaskFinishedMsg:
		defer os.Remove(msg.path)
		if msg.fileIdx < 0 || msg.fileIdx >= len(m.items) {
			return m, nil
		}
		raw, readErr := os.ReadFile(msg.path)
		if msg.err != nil || readErr != nil {
			m.refresh("edit cancelled")
			return m, nil
		}
		title, desc := parseTaskBuffer(string(raw))
		if title == "" {
			m.refresh("empty title — edit aborted")
			return m, nil
		}
		m.items[msg.fileIdx].Text = title
		m.items[msg.fileIdx].Description = desc
		m.persist(fmt.Sprintf("edited %q", title))
		return m, nil

	case tea.BackgroundColorMsg:
		m.isDark = msg.IsDark()
		m.list.Styles = list.DefaultStyles(m.isDark)
		m.list.SetDelegate(rowDelegate{dark: m.isDark})
		m.help.Styles = help.DefaultStyles(m.isDark)
		return m, nil

	case gitlabSyncedMsg:
		if msg.readErr == nil {
			m.store = msg.store
			m.items = m.store.EnsureProject(m.project).Items
		}
		cmd := m.refreshWith(gitlabSyncStatus(msg), true)
		return m, cmd

	case doneAnimTickMsg:
		return m, m.stepDoneAnim()

	case tea.KeyPressMsg:
		return m.handleKey(msg)
	}

	var cmd tea.Cmd
	m.list, cmd = m.list.Update(msg)
	return m, cmd
}

func (m uiModel) handleKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	keyName := msg.String()

	// A delete confirmation owns the keyboard: only y answers it.
	// Every other key, including navigation, is ignored so a stray
	// keystroke cannot act on the list mid-prompt; ctrl+c still
	// force-quits.
	if m.mode == modeConfirm && m.confirm != nil {
		pd := m.confirm
		switch keyName {
		case "ctrl+c":
			return m, tea.Quit
		case "y", "Y":
			m.confirm = nil
			if pd.kind == deleteProject {
				m.mode = modeProjects
				m.deleteProject(pd.project)
			} else {
				m.mode = modeNormal
				idxs := make([]int, 0, len(pd.fileIdxs))
				for _, fi := range pd.fileIdxs {
					if fi >= 0 && fi < len(m.items) {
						idxs = append(idxs, fi)
					}
				}
				if len(idxs) > 0 {
					m.items = deleteItems(m.items, idxs)
					if len(idxs) == 1 {
						m.persist(fmt.Sprintf("deleted %q", pd.label))
					} else {
						m.persist(fmt.Sprintf("deleted %d tasks", len(idxs)))
					}
				}
				m.applyVisual()
			}
		case "n", "N", "esc", "q":
			m.confirm = nil
			m.status = "delete cancelled"
			if pd.kind == deleteProject {
				m.mode = modeProjects
			} else {
				m.mode = modeNormal
			}
			m.applyVisual()
		}
		return m, nil
	}

	// Visual mode owns the keyboard: motions extend the selection and
	// the batch verbs act on it (see visual.go).
	if m.mode == modeVisual {
		return m.handleVisualKey(msg)
	}

	// Input modes own the keyboard: enter commits, esc drops to
	// NORMAL, and a second esc cancels.
	if m.mode == modeAdd || m.mode == modeEdit || m.mode == modeDescribe || m.mode == modeNewProject || m.mode == modeRename || isGitLabInput(m.mode) {
		act, cmd := m.input.HandleKey(msg)
		switch act {
		case vimCommit:
			text := strings.TrimSpace(m.input.Value())
			m.input.Blur()
			if isGitLabInput(m.mode) {
				m.commitGitLabInput(text)
				return m, nil
			}
			switch m.mode {
			case modeNewProject:
				if text != "" {
					if err := m.createProjectInPlace(text); err != nil {
						m.status = err.Error()
					}
				}
				m.mode = modeProjects
				return m, nil
			case modeRename:
				if text != "" && text != m.renameTarget {
					if err := m.renameProject(text); err != nil {
						m.status = err.Error()
					}
				}
				m.mode = modeProjects
				return m, nil
			default:
				switch {
				case m.mode == modeAdd && text != "":
					m.items = append(m.items, task.Item{Text: text, Priority: 2, Created: time.Now()})
					m.persist(fmt.Sprintf("added %q", text))
				case m.mode == modeEdit && text != "" && m.editingFileIdx >= 0 && m.editingFileIdx < len(m.items):
					m.items[m.editingFileIdx].Text = text
					m.persist(fmt.Sprintf("edited %q", text))
				case m.mode == modeDescribe && m.editingFileIdx >= 0 && m.editingFileIdx < len(m.items):
					if text == m.describeLoaded && m.items[m.editingFileIdx].Description != text {
						m.refresh("description unchanged")
					} else {
						m.items[m.editingFileIdx].Description = text
						if text == "" {
							m.persist("description cleared")
						} else {
							m.persist(fmt.Sprintf("described %q", m.items[m.editingFileIdx].Text))
						}
					}
				default:
					m.refresh("cancelled")
				}
				m.mode = modeNormal
				return m, nil
			}
		case vimCancel:
			m.input.Blur()
			if isGitLabInput(m.mode) {
				m.status = "cancelled"
				m.mode = modeGitLab
				return m, nil
			}
			if m.mode == modeNewProject || m.mode == modeRename {
				m.mode = modeProjects
				return m, nil
			}
			m.mode = modeNormal
			m.refresh("cancelled")
			return m, nil
		default:
			return m, cmd
		}
	}

	// The GitLab page owns the keyboard: j/k move between the url,
	// the token, and the links; enter edits the row under the cursor.
	if m.mode == modeGitLab {
		names := m.gitlabLinkNames()
		switch keyName {
		case "up", "k":
			if m.gitlabIdx > 0 {
				m.gitlabIdx--
			}
		case "down", "j":
			if m.gitlabIdx < 1+len(names) {
				m.gitlabIdx++
			}
		case "enter":
			m.editGitLabRow()
		case "a":
			m.mode = modeGitLabTd
			m.input.SetValue("")
			m.input.Placeholder = "td project"
			m.input.Enter()
			m.input.Focus()
		case "d":
			if k := m.gitlabIdx - 2; k >= 0 && k < len(names) {
				m.removeGitLabLink(names[k])
			} else {
				m.status = "select a link to remove"
			}
		case "s":
			sync := m.gitlabSyncCmd()
			if sync == nil {
				m.status = "GitLab is not configured — set url, token, and a link"
				return m, nil
			}
			m.status = "gitlab: syncing…"
			return m, sync
		case "esc", "q", "g":
			m.mode = modeNormal
		}
		return m, nil
	}

	// The projects screen owns the keyboard. Its verbs mirror the task
	// list's — enter commits, a adds, e edits, d deletes, esc backs
	// out — but they act on projects instead of items.
	if m.mode == modeProjects {
		names := m.panelNames()
		switch keyName {
		case "up", "k":
			if m.pickerIdx > 0 {
				m.pickerIdx--
			}
		case "down", "j":
			if m.pickerIdx < len(names)-1 {
				m.pickerIdx++
			}
		case "enter":
			if m.pickerIdx < len(names) {
				if m.projectsForMove {
					m.moveSelectedTo(names[m.pickerIdx])
				} else {
					m.setProject(names[m.pickerIdx])
					m.refresh(fmt.Sprintf("project: %s", names[m.pickerIdx]))
				}
			}
			m.mode = modeNormal
			m.projectsForMove = false
		case "a":
			if !m.projectsForMove {
				m.mode = modeNewProject
				m.input.SetValue("")
				m.input.Enter()
				m.input.Placeholder = "new project name"
				m.input.Focus()
			}
		case "e":
			if !m.projectsForMove && m.pickerIdx < len(names) {
				m.renameTarget = names[m.pickerIdx]
				m.mode = modeRename
				m.input.SetValue(m.renameTarget)
				m.input.Enter()
				m.input.Placeholder = ""
				m.input.Focus()
			}
		case "d":
			if !m.projectsForMove && m.pickerIdx < len(names) {
				name := names[m.pickerIdx]
				m.confirm = &pendingDelete{kind: deleteProject, project: name}
				m.mode = modeConfirm
			}
		case "esc", "q", "p":
			m.mode = modeNormal
			m.projectsForMove = false
			m.moveFileIdxs = nil
		}
		return m, nil
	}

	// The detail screen is a viewer: enter, esc, and q close it; any
	// other key closes it first and then acts on the list below, so
	// the verb hits the very task that was being read.
	if m.mode == modeDetail {
		m.mode = modeNormal
		if keyName == "enter" || keyName == "esc" || keyName == "q" {
			return m, nil
		}
	}

	// While the filter input is open the list owns every key (typing,
	// arrows, esc to cancel). An applied filter only narrows the rows:
	// task verbs below still act on them, and esc clears the filter
	// instead of falling through to the quit binding.
	if m.list.FilterState() == list.Filtering {
		var cmd tea.Cmd
		m.list, cmd = m.list.Update(msg)
		return m, cmd
	}
	if m.list.FilterState() == list.FilterApplied && key.Matches(msg, m.list.KeyMap.ClearFilter) {
		m.list.ResetFilter()
		m.status = "filter cleared"
		return m, nil
	}

	switch {
	case key.Matches(msg, m.keys.Quit):
		return m, tea.Quit

	case key.Matches(msg, m.keys.Cycle):
		m.view = m.view.next()
		m.refresh("")
		return m, nil

	case key.Matches(msg, m.keys.CycleRev):
		m.view = m.view.prev()
		m.refresh("")
		return m, nil

	case key.Matches(msg, m.keys.Help):
		m.help.ShowAll = !m.help.ShowAll
		m.resizeList()
		return m, nil

	case key.Matches(msg, m.keys.Pick):
		m.mode = modeProjects
		m.projectsForMove = false
		m.moveFileIdxs = nil
		m.pickerIdx = 0
		for i, n := range m.projectNames() {
			if n == m.project {
				m.pickerIdx = i
				break
			}
		}
		return m, nil

	case key.Matches(msg, m.keys.GitLab):
		m.openGitLab()
		return m, nil

	case key.Matches(msg, m.keys.Move):
		if _, ok := m.selected(); !ok {
			m.status = "nothing selected to move"
			return m, nil
		}
		if len(m.moveTargets()) == 0 {
			m.status = "no other projects — press p to create one"
			return m, nil
		}
		m.mode = modeProjects
		m.projectsForMove = true
		m.pickerIdx = 0
		return m, nil

	case key.Matches(msg, m.keys.Reload):
		store, err := task.ReadStore(m.path)
		if err != nil {
			m.status = fmt.Sprintf("reload failed: %v", err)
			return m, nil
		}
		m.store = store
		if m.store.Project(m.project) == nil {
			m.project = task.DefaultProject
		}
		m.items = m.store.EnsureProject(m.project).Items
		m.refresh("reloaded from disk")
		return m, nil

	case key.Matches(msg, m.keys.NextProject):
		m.switchProject(1)
		return m, nil

	case key.Matches(msg, m.keys.PrevProject):
		m.switchProject(-1)
		return m, nil

	case key.Matches(msg, m.keys.Add):
		m.mode = modeAdd
		m.input.SetValue("")
		m.input.Enter()
		m.input.Placeholder = "new task"
		m.input.Focus()
		return m, nil

	case key.Matches(msg, m.keys.Edit):
		if it, ok := m.selected(); ok {
			m.mode = modeEdit
			m.editingFileIdx = it.row.fileIdx
			m.input.SetValue(it.row.item.Text)
			m.input.Enter()
			m.input.Placeholder = ""
			m.input.Focus()
		}
		return m, nil

	case key.Matches(msg, m.keys.Describe):
		if it, ok := m.selected(); ok {
			m.mode = modeDescribe
			m.editingFileIdx = it.row.fileIdx
			m.input.SetValue(it.row.item.Description)
			m.input.Enter()
			m.describeLoaded = strings.TrimSpace(m.input.Value())
			m.input.Placeholder = "description"
			m.input.Focus()
		}
		return m, nil

	case key.Matches(msg, m.keys.EditExternal):
		it, ok := m.selected()
		if !ok {
			return m, nil
		}
		editor := externalEditor()
		if _, err := exec.LookPath(strings.Fields(editor)[0]); err != nil {
			m.refresh(fmt.Sprintf("editor %q not found — set $EDITOR", strings.Fields(editor)[0]))
			return m, nil
		}
		f, err := os.CreateTemp("", "td-task-*.md")
		if err != nil {
			m.refresh("temp file: " + err.Error())
			return m, nil
		}
		_, _ = fmt.Fprint(f, taskBuffer(it.row.item.Text, it.row.item.Description))
		_ = f.Close()
		path := f.Name()
		run := exec.Command("sh", "-c", editor+" \"$1\"", "td", path)
		return m, tea.ExecProcess(run, func(err error) tea.Msg {
			return editTaskFinishedMsg{err: err, path: path, fileIdx: it.row.fileIdx}
		})

	case keyName == "enter":
		if _, ok := m.selected(); ok {
			m.mode = modeDetail
		}
		return m, nil

	case keyName == "v":
		m.enterVisual()
		return m, nil

	case key.Matches(msg, m.keys.Toggle):
		var cmd tea.Cmd
		if it, ok := m.selected(); ok {
			wasDone := m.items[it.row.fileIdx].Done
			listIdx := m.list.Index()
			toggleItem(m.items, it.row.fileIdx)
			state := "pending"
			if m.items[it.row.fileIdx].Done {
				state = "done"
			}
			m.persist(fmt.Sprintf("%q → %s", it.row.item.Text, state))
			if !wasDone {
				cmd = m.beginCompletion(it.row, listIdx)
			}
		}
		return m, cmd

	case key.Matches(msg, m.keys.Delete):
		if it, ok := m.selected(); ok {
			m.confirm = &pendingDelete{kind: deleteTask, fileIdxs: []int{it.row.fileIdx}, label: it.row.item.Text}
			m.mode = modeConfirm
		}
		return m, nil

	case keyName == "1" || keyName == "2" || keyName == "3":
		if it, ok := m.selected(); ok {
			pri := int(msg.Code) - '0'
			var tmp task.Item
			if err := tmp.SetPriority(pri); err == nil {
				m.items[it.row.fileIdx].Priority = pri
				m.persist(fmt.Sprintf("%q → priority %d", it.row.item.Text, pri))
			} else {
				m.status = err.Error()
			}
		}
		return m, nil
	}

	// Navigation, filter entry, etc. belong to the list.
	var cmd tea.Cmd
	m.list, cmd = m.list.Update(msg)
	return m, cmd
}

// fitCells truncates s to width cells with an ellipsis; ANSI-aware, so
// it can clamp already-styled strings. List rows, the status line, and
// help must stay on one screen line; the wrapped edit view shows the
// full text.
func fitCells(s string, width int) string {
	return ansi.Truncate(s, width, "…")
}

// previewLine is the dim description shown under the task list while
// the selected item has one; "" renders nothing.
func (m uiModel) previewLine() string {
	if m.mode != modeNormal {
		return ""
	}
	it, ok := m.selected()
	if !ok || it.row.item.Description == "" {
		return ""
	}
	// The preview is one terminal line: paragraph breaks flatten.
	text := strings.ReplaceAll(it.row.item.Description, "\n", " ")
	if m.width > 6 {
		text = fitCells(text, m.width-6)
	}
	return "  " + dimStyle().Render("↳ "+text)
}

// contentHeight is the space left for the main body once header,
// status/input line, and help are subtracted.
func (m uiModel) contentHeight() int {
	overhead := 5
	if m.help.ShowAll {
		overhead = 10
	}
	if m.previewLine() != "" {
		overhead++
	}
	h := m.height - overhead
	if h < 1 {
		h = 1
	}
	return h
}

// resizeList fits the list into the space below header, status/input
// line, and help.
func (m *uiModel) resizeList() {
	m.list.SetSize(m.width, m.contentHeight())
}

// projectsView renders the full-page projects screen: one row per
// namespace with pending/done counts, a cursor, and the current
// project marked.
func (m uiModel) projectsView() string {
	names := m.panelNames()
	if len(names) == 0 {
		return "  " + dimStyle().Render("no projects yet")
	}
	nameW := 0
	for _, n := range names {
		if len(n) > nameW {
			nameW = len(n)
		}
	}
	lines := make([]string, 0, len(names))
	for i, n := range names {
		pending, done := 0, 0
		if p := m.store.Project(n); p != nil {
			for _, it := range p.Items {
				if it.Done {
					done++
				} else {
					pending++
				}
			}
		}
		cursor := "  "
		if i == m.pickerIdx {
			cursor = "▸ "
		}
		label := fmt.Sprintf("%-*s", nameW, n)
		if n == m.project {
			label = projectStyle().Render(label)
		}
		counts := dimStyle().Render(fmt.Sprintf("%d pending · %d done", pending, done))
		line := cursor + label + "   " + counts
		if n == m.project {
			line += "   " + dimStyle().Render("(current)")
		}
		if i == m.pickerIdx {
			line = selectedStyle().Render(line)
		}
		lines = append(lines, line)
	}
	return lipgloss.JoinVertical(lipgloss.Left, lines...)
}

// projectsKeys is the context help shown while the projects screen is
// open; the verbs mirror the task list's bindings.
type projectsKeys struct{ moving bool }

func (p projectsKeys) ShortHelp() []key.Binding {
	if p.moving {
		return []key.Binding{
			key.NewBinding(key.WithKeys("enter"), key.WithHelp("enter", "move here")),
			key.NewBinding(key.WithKeys("j", "k"), key.WithHelp("j/k", "choose")),
			key.NewBinding(key.WithKeys("esc", "q", "p"), key.WithHelp("esc", "cancel")),
		}
	}
	return []key.Binding{
		key.NewBinding(key.WithKeys("enter"), key.WithHelp("enter", "select project")),
		key.NewBinding(key.WithKeys("a"), key.WithHelp("a", "new project")),
		key.NewBinding(key.WithKeys("e"), key.WithHelp("e", "rename")),
		key.NewBinding(key.WithKeys("d"), key.WithHelp("d", "delete empty")),
		key.NewBinding(key.WithKeys("j", "k"), key.WithHelp("j/k", "move")),
		key.NewBinding(key.WithKeys("esc", "q", "p"), key.WithHelp("esc", "back to tasks")),
	}
}

func (p projectsKeys) FullHelp() [][]key.Binding { return [][]key.Binding{p.ShortHelp()} }

// confirmKeys is the context help while a delete waits for an answer;
// the prompt itself names what will be removed.
type confirmKeys struct{}

func (confirmKeys) ShortHelp() []key.Binding {
	return []key.Binding{
		key.NewBinding(key.WithKeys("y"), key.WithHelp("y", "confirm delete")),
		key.NewBinding(key.WithKeys("n", "esc"), key.WithHelp("n/esc", "cancel")),
	}
}

func (confirmKeys) FullHelp() [][]key.Binding { return [][]key.Binding{confirmKeys{}.ShortHelp()} }

// detailView renders the focused task in full: the whole text and
// description wrapped to the screen — the rows only abbreviate — plus
// the metadata and the CLI handles for the same item.
func (m uiModel) detailView() string {
	it, ok := m.selected()
	if !ok {
		return ""
	}
	item := it.row.item
	w := max(30, m.width-10)
	wrap := lipgloss.NewStyle().Width(w)

	lines := []string{lipgloss.NewStyle().Bold(true).Width(w).Render(item.Text)}
	if desc := strings.TrimRight(item.Description, "\n"); desc != "" {
		lines = append(lines, "", dimStyle().Render("description"))
		for _, para := range strings.Split(desc, "\n") {
			if strings.TrimSpace(para) == "" {
				lines = append(lines, "")
				continue
			}
			lines = append(lines, wrap.Render(para))
		}
	}
	status := "pending"
	if item.Done {
		status = "done"
	}
	pri := "medium"
	switch item.Priority {
	case 1:
		pri = "high"
	case 3:
		pri = "low"
	}
	meta := [][2]string{
		{"project", m.project},
		{"number", fmt.Sprintf("%d — td complete %d -P %s", it.row.num, it.row.num, m.project)},
		{"status", status},
		{"priority", fmt.Sprintf("%d (%s)", item.Priority, pri)},
		{"created", item.PrettyCreated()},
		{"completed", item.PrettyCompleted()},
	}
	if item.Linked() {
		meta = append(meta, [2]string{"issue", fmt.Sprintf("%s#%d", item.GitLabProject, item.GitLabIssue)})
	}
	lines = append(lines, "")
	for _, kv := range meta {
		lines = append(lines, dimStyle().Render(fmt.Sprintf("%-10s", kv[0]))+kv[1])
	}
	return lipgloss.JoinVertical(lipgloss.Left, lines...)
}

// detailKeys is the context help shown while the detail screen is
// open; the verbs act on the detailed task and drop back to the list.
type detailKeys struct{ keys uiKeys }

func (d detailKeys) ShortHelp() []key.Binding {
	return []key.Binding{
		key.NewBinding(key.WithKeys("enter", "esc", "q"), key.WithHelp("enter", "back")),
		d.keys.Toggle, d.keys.Edit, d.keys.EditExternal, d.keys.Describe, d.keys.Delete, d.keys.Move,
	}
}

func (d detailKeys) FullHelp() [][]key.Binding {
	return [][]key.Binding{
		d.ShortHelp(),
		{d.keys.Pri1, d.keys.Pri2, d.keys.Pri3, d.keys.Pick, d.keys.Reload},
	}
}

func (m uiModel) View() tea.View {
	var v tea.View
	v.AltScreen = true

	inProjects := m.mode == modeProjects || m.mode == modeNewProject || m.mode == modeRename ||
		(m.mode == modeConfirm && m.confirm != nil && m.confirm.kind == deleteProject)
	inGitLab := m.mode == modeGitLab || isGitLabInput(m.mode)

	var header string
	if !inProjects && !inGitLab {
		pending, done := 0, 0
		for _, it := range m.items {
			if it.Done {
				done++
			} else {
				pending++
			}
		}

		tabs := make([]string, 0, 3)
		for _, vm := range []viewMode{viewPending, viewAll, viewDone} {
			label := vm.label()
			count := 0
			switch vm {
			case viewPending:
				count = pending
			case viewDone:
				count = done
			case viewAll:
				count = len(m.items)
			}
			tab := fmt.Sprintf(" %s (%d) ", label, count)
			switch {
			case vm == viewDone && time.Now().Before(m.doneFlash):
				tab = doneFlashStyle().Render(tab)
			case vm == m.view:
				tab = activeTabStyle().Render(tab)
			default:
				tab = tabStyle().Render(tab)
			}
			tabs = append(tabs, tab)
		}
		chipLabel := m.project
		if names := m.projectNames(); len(names) > 1 {
			for i, n := range names {
				if n == m.project {
					chipLabel = fmt.Sprintf("%s %d/%d", m.project, i+1, len(names))
					break
				}
			}
		}
		chip := projectStyle().Render(fmt.Sprintf(" %s ", chipLabel))
		header = lipgloss.JoinHorizontal(lipgloss.Left, append([]string{chip}, tabs...)...)
	}

	// The edit block is laid out before the body: when it wraps to
	// several lines the list shrinks to keep the footer on screen.
	var inputLine string
	inputH := 1
	inInput := m.mode == modeAdd || m.mode == modeEdit || m.mode == modeDescribe || m.mode == modeNewProject || m.mode == modeRename || isGitLabInput(m.mode)
	if inInput {
		var label string
		switch m.mode {
		case modeAdd:
			label = "add  "
		case modeEdit:
			label = "edit "
		case modeDescribe:
			label = "describe "
		case modeNewProject:
			label = "project "
		case modeRename:
			label = "rename  "
		case modeGitLabURL:
			label = "url "
		case modeGitLabToken:
			label = "token "
		case modeGitLabTd:
			label = "link "
		case modeGitLabRef:
			label = "project "
		}
		prefix := m.input.modeTag() + label
		avail := max(20, m.width-lipgloss.Width(prefix))
		maxLines := min(maxEditLines, m.contentHeight()-2)
		block, h := m.input.EditView(avail, max(1, maxLines))
		inputLine = lipgloss.JoinHorizontal(lipgloss.Top, prefix, block)
		inputH = h
	} else if m.mode == modeConfirm && m.confirm != nil {
		text := m.confirm.prompt()
		if m.width > 0 {
			text = fitCells(text, m.width)
		}
		inputLine = statusStyle(m.isDark).Render(text)
	} else if m.mode == modeVisual {
		lo, hi := m.visualRange()
		text := fmt.Sprintf("visual: %d selected", hi-lo+1)
		if m.width > 0 {
			text = fitCells(text, m.width)
		}
		inputLine = statusStyle(m.isDark).Render(text)
	} else if m.status != "" {
		text := m.status
		if m.width > 0 {
			text = fitCells(text, m.width)
		}
		inputLine = statusStyle(m.isDark).Render(text)
	}

	bodyH := max(1, m.contentHeight()-(inputH-1))
	m.list.SetSize(m.width, bodyH)
	body := m.list.View()
	if inProjects {
		// The projects screen drops the task-list chrome — its rows
		// already list every project — and reclaims both lines, so
		// total height (and thus the footer position) matches the
		// task list.
		body = lipgloss.NewStyle().Height(bodyH + 2).Render(m.projectsView())
	} else if inGitLab {
		body = lipgloss.NewStyle().Height(bodyH + 2).Render(m.gitlabView())
	} else if m.mode == modeDetail {
		body = lipgloss.NewStyle().Height(bodyH+2).MaxHeight(bodyH+2).
			Padding(0, 2).Render(m.detailView())
	}

	var footer string
	switch {
	case inInput:
		footer = m.help.View(vimInputKeys{insert: m.input.state == vimInsert})
	case m.mode == modeConfirm:
		footer = m.help.View(confirmKeys{})
	case m.mode == modeVisual:
		footer = m.help.View(visualKeys{})
	case inProjects:
		footer = m.help.View(projectsKeys{moving: m.projectsForMove})
	case inGitLab:
		footer = m.help.View(gitlabKeys{})
	case m.mode == modeDetail:
		footer = m.help.View(detailKeys{keys: m.keys})
	default:
		footer = m.help.View(m.keys)
	}

	parts := []string{header}
	if m.width > 0 && footer != "" {
		clamped := make([]string, 0, 8)
		for _, l := range strings.Split(footer, "\n") {
			clamped = append(clamped, fitCells(l, m.width))
		}
		footer = strings.Join(clamped, "\n")
	}
	if !inProjects && !inGitLab {
		parts = append(parts, columnHeader())
	}
	parts = append(parts, body)
	if pv := m.previewLine(); pv != "" {
		parts = append(parts, pv)
	}
	if inProjects || inGitLab {
		parts = []string{body}
	}
	if inputLine != "" {
		parts = append(parts, inputLine)
	}
	parts = append(parts, footer)
	v.SetContent(lipgloss.JoinVertical(lipgloss.Left, parts...))
	return v
}

// columnHeader labels the task-list columns; widths mirror
// rowDelegate.Render.
func columnHeader() string {
	cells := fmt.Sprintf("%5s %-*s %-*s %-*s %-*s %s",
		"#", doneColW, "DONE", priColW, "PRI", dateColW, "CREATED", dateColW, "COMPLETED", "TASK")
	return "  " + headerStyle().Render(cells)
}

// Styles — colors from the ANSI 16 palette so they adapt to themes.

func numStyle() lipgloss.Style {
	return lipgloss.NewStyle().Foreground(lipgloss.Color("240"))
}

func headerStyle() lipgloss.Style {
	return lipgloss.NewStyle().Foreground(lipgloss.Color("244")).Bold(true)
}

func doneStyle() lipgloss.Style {
	return lipgloss.NewStyle().Foreground(lipgloss.Color("42")).Bold(true)
}

func priStyle(pri int) lipgloss.Style {
	switch pri {
	case 1:
		return lipgloss.NewStyle().Foreground(lipgloss.Color("196")).Bold(true)
	case 3:
		return lipgloss.NewStyle().Foreground(lipgloss.Color("33"))
	default:
		return lipgloss.NewStyle().Foreground(lipgloss.Color("240"))
	}
}

func dimStyle() lipgloss.Style {
	return lipgloss.NewStyle().Foreground(lipgloss.Color("243"))
}

func selectedStyle() lipgloss.Style {
	return lipgloss.NewStyle().Bold(true)
}

func activeTabStyle() lipgloss.Style {
	return lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("230")).Background(lipgloss.Color("238"))
}

// projectStyle renders the current-project chip left of the view tabs.
func projectStyle() lipgloss.Style {
	return lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("141"))
}

func tabStyle() lipgloss.Style {
	return lipgloss.NewStyle().Foreground(lipgloss.Color("244"))
}

// doneFlashStyle marks the completed tab while a finished row "lands"
// in it.
func doneFlashStyle() lipgloss.Style {
	return lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("42"))
}

// exitStyle fades a departing row toward the background: down the gray
// ramp on dark themes, up it on light ones.
func exitStyle(p float64, dark bool) lipgloss.Style {
	g := 245 - int(p*12)
	if !dark {
		g = 240 + int(p*14)
	}
	return lipgloss.NewStyle().Foreground(lipgloss.Color(fmt.Sprintf("%d", g)))
}

// visualStyle backs the selected block in visual mode; each column's
// own color rides on top of it.
func visualStyle(dark bool) lipgloss.Style {
	bg := lipgloss.Color("238")
	if !dark {
		bg = lipgloss.Color("254")
	}
	return lipgloss.NewStyle().Background(bg)
}

func statusStyle(isDark bool) lipgloss.Style {
	fg := lipgloss.Color("220")
	if !isDark {
		fg = lipgloss.Color("94")
	}
	return lipgloss.NewStyle().Foreground(fg)
}

// vimInsertTagStyle / vimNormalTagStyle mark the field's modal state.
func vimInsertTagStyle() lipgloss.Style {
	return lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("42"))
}

func vimNormalTagStyle() lipgloss.Style {
	return lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("220"))
}

// vimCaretStyle renders the caret block over the character under the
// edit cursor in the wrapped input view.
func vimCaretStyle() lipgloss.Style {
	return lipgloss.NewStyle().Inline(true).Reverse(true)
}
