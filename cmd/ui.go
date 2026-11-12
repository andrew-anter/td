/*
Copyright © 2025 Andrew Anter <andrew.anter@gmail.com>
*/
package cmd

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"time"

	"charm.land/bubbles/v2/help"
	"charm.land/bubbles/v2/key"
	"charm.land/bubbles/v2/list"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

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
// what the built-in fuzzy filter matches against.
type uiItem struct{ row uiRow }

func (i uiItem) Title() string       { return i.row.item.Text }
func (i uiItem) Description() string { return "" }
func (i uiItem) FilterValue() string { return i.row.item.Text }

// Column widths are shared with columnHeader so the labels stay over
// their columns.
const (
	doneColW = 4
	priColW  = 3
	dateColW = 10
)

// rowDelegate renders a row: global number, done marker, priority,
// creation date, completion date, text.
type rowDelegate struct{}

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
	prefix := fmt.Sprintf("%s %s %s %s %s ", num, done, pri, created, completed)
	if w := m.Width(); w > 0 {
		if avail := w - 2 - lipgloss.Width(prefix); avail > 0 {
			text = fitCells(text, avail)
		}
	}
	if it.row.item.Done {
		text = dimStyle().Strikethrough(true).Render(text)
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
	items[fileIdx].Done = !items[fileIdx].Done
	if items[fileIdx].Done {
		items[fileIdx].Completed = time.Now()
	} else {
		items[fileIdx].Completed = time.Time{}
	}
}

func deleteItem(items []task.Item, fileIdx int) []task.Item {
	return append(items[:fileIdx], items[fileIdx+1:]...)
}

type inputMode int

const (
	modeNormal inputMode = iota
	modeAdd
	modeEdit
	modeDescribe   // wrapped field editing the selected item's description
	modeProjects   // full-page projects screen (p to manage, m to pick a move target)
	modeNewProject // textinput for a new project name
	modeRename     // textinput renaming a project
	modeDetail     // full read-only view of the focused task
	modeConfirm    // delete awaiting y/n
)

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
	kind    deleteKind
	fileIdx int
	project string
	label   string
}

func (p pendingDelete) prompt() string {
	if p.kind == deleteProject {
		return fmt.Sprintf("delete project %q? y confirm · n cancel", p.project)
	}
	return fmt.Sprintf("delete %q? y confirm · n cancel", p.label)
}

type uiKeys struct {
	Quit         key.Binding
	Detail       key.Binding
	Toggle       key.Binding
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
	Help         key.Binding
}

func (k uiKeys) ShortHelp() []key.Binding {
	return []key.Binding{k.Add, k.Detail, k.Toggle, k.Edit, k.EditExternal, k.Describe, k.Delete, k.Move, k.Pick, k.Cycle, k.Filter, k.Quit}
}

func (k uiKeys) FullHelp() [][]key.Binding {
	return [][]key.Binding{
		{k.Add, k.Detail, k.Edit, k.EditExternal, k.Describe, k.Delete, k.Toggle},
		{k.Pri1, k.Pri2, k.Pri3, k.Move, k.Pick, k.PrevProject, k.NextProject},
		{k.Cycle, k.CycleRev, k.Filter, k.Reload, k.Help, k.Quit},
	}
}

func newUIKeys() uiKeys {
	return uiKeys{
		Quit:         key.NewBinding(key.WithKeys("q", "esc", "ctrl+c"), key.WithHelp("q", "quit")),
		Detail:       key.NewBinding(key.WithKeys("enter"), key.WithHelp("enter", "details")),
		Toggle:       key.NewBinding(key.WithKeys("x", "space"), key.WithHelp("x", "toggle done")),
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

	list   list.Model
	input  vimInput
	help   help.Model
	keys   uiKeys
	status string
	width  int
	height int
	isDark bool
}

// newUIModel builds the TUI over an already-loaded snapshot. When the
// named project is missing from the store, the model falls back to the
// default namespace.
func newUIModel(path string, store task.Store, project string) uiModel {
	if store.Project(project) == nil {
		project = task.DefaultProject
	}
	p := store.EnsureProject(project)

	l := list.New([]list.Item{}, rowDelegate{}, 0, 0)
	l.SetShowTitle(false)
	l.SetShowStatusBar(false)
	l.SetShowPagination(false)
	l.SetShowHelp(false)
	l.SetFilteringEnabled(true)

	ti := newVimInput()

	h := help.New()

	m := uiModel{
		path:    path,
		store:   store,
		project: project,
		items:   p.Items,
		list:    l,
		input:   ti,
		help:    h,
		keys:    newUIKeys(),
		isDark:  true,
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

// moveSelectedTo relocates the item under the cursor into dest,
// keeping its priority and done state, and saves immediately.
func (m *uiModel) moveSelectedTo(dest string) {
	it, ok := m.selected()
	if !ok {
		return
	}
	d := m.store.Project(dest)
	if d == nil {
		m.status = fmt.Sprintf("no such project %q", dest)
		return
	}
	src := m.store.EnsureProject(m.project)
	src.Items = append(src.Items[:it.row.fileIdx], src.Items[it.row.fileIdx+1:]...)
	d.Items = append(d.Items, it.row.item)
	m.items = src.Items
	m.persist(fmt.Sprintf("moved %q to %q", it.row.item.Text, dest))
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
	m.status = status
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
	m.list.ResetFilter()
	m.list.SetItems(lis)

	if keepFileIdx >= 0 {
		for k := range lis {
			if lis[k].(uiItem).row.fileIdx == keepFileIdx {
				m.list.Select(k)
				break
			}
		}
	}
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

func (m uiModel) Init() tea.Cmd {
	return tea.RequestBackgroundColor
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
		m.help.Styles = help.DefaultStyles(m.isDark)
		return m, nil

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
				if pd.fileIdx >= 0 && pd.fileIdx < len(m.items) {
					m.items = deleteItem(m.items, pd.fileIdx)
					m.persist(fmt.Sprintf("deleted %q", pd.label))
				}
			}
		case "n", "N", "esc", "q":
			m.confirm = nil
			m.status = "delete cancelled"
			if pd.kind == deleteProject {
				m.mode = modeProjects
			} else {
				m.mode = modeNormal
			}
		}
		return m, nil
	}

	// Input modes own the keyboard: enter commits, esc drops to
	// NORMAL, and a second esc cancels.
	if m.mode == modeAdd || m.mode == modeEdit || m.mode == modeDescribe || m.mode == modeNewProject || m.mode == modeRename {
		act, cmd := m.input.HandleKey(msg)
		switch act {
		case vimCommit:
			text := strings.TrimSpace(m.input.Value())
			m.input.Blur()
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
		m.pickerIdx = 0
		for i, n := range m.projectNames() {
			if n == m.project {
				m.pickerIdx = i
				break
			}
		}
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

	case key.Matches(msg, m.keys.Toggle):
		if it, ok := m.selected(); ok {
			toggleItem(m.items, it.row.fileIdx)
			state := "pending"
			if m.items[it.row.fileIdx].Done {
				state = "done"
			}
			m.persist(fmt.Sprintf("%q → %s", it.row.item.Text, state))
		}
		return m, nil

	case key.Matches(msg, m.keys.Delete):
		if it, ok := m.selected(); ok {
			m.confirm = &pendingDelete{kind: deleteTask, fileIdx: it.row.fileIdx, label: it.row.item.Text}
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

	var header string
	if !inProjects {
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
			if vm == m.view {
				tab = activeTabStyle().Render(tab)
			} else {
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
	inInput := m.mode == modeAdd || m.mode == modeEdit || m.mode == modeDescribe || m.mode == modeNewProject || m.mode == modeRename
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
	case inProjects:
		footer = m.help.View(projectsKeys{moving: m.projectsForMove})
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
	if !inProjects {
		parts = append(parts, columnHeader())
	}
	parts = append(parts, body)
	if pv := m.previewLine(); pv != "" {
		parts = append(parts, pv)
	}
	if inProjects {
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
