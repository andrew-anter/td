/*
Copyright © 2025 Andrew Anter <andrew.anter@gmail.com>
*/
package cmd

import (
	"fmt"
	"io"
	"strings"

	"charm.land/bubbles/v2/help"
	"charm.land/bubbles/v2/key"
	"charm.land/bubbles/v2/list"
	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

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

// rowDelegate renders a row: global number, done marker, priority,
// text.
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
	if it.row.item.Done {
		text = dimStyle().Strikethrough(true).Render(text)
	}
	num := numStyle().Render(fmt.Sprintf("%4d.", it.row.num))
	done := doneStyle().Render(strings.TrimSpace(it.row.item.PrettyDone()))
	pri := priStyle(it.row.item.Priority).Render(strings.TrimSpace(it.row.item.PrettyP()))
	line := fmt.Sprintf("%s %s %s %s", num, done, pri, text)
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
}

func deleteItem(items []task.Item, fileIdx int) []task.Item {
	return append(items[:fileIdx], items[fileIdx+1:]...)
}

type inputMode int

const (
	modeNormal inputMode = iota
	modeAdd
	modeEdit
	modePicker     // project picker overlay
	modeNewProject // textinput for a new project name
	modeMove       // move-selected-item-to-project overlay
)

type uiKeys struct {
	Quit        key.Binding
	Toggle      key.Binding
	Add         key.Binding
	Edit        key.Binding
	Delete      key.Binding
	Pri1        key.Binding
	Pri2        key.Binding
	Pri3        key.Binding
	PrevProject key.Binding
	NextProject key.Binding
	Pick        key.Binding
	Move        key.Binding
	Cycle       key.Binding
	CycleRev    key.Binding
	Reload      key.Binding
	Filter      key.Binding
	Help        key.Binding
}

func (k uiKeys) ShortHelp() []key.Binding {
	return []key.Binding{k.Add, k.Toggle, k.Edit, k.Delete, k.Move, k.Pick, k.Cycle, k.Filter, k.Quit}
}

func (k uiKeys) FullHelp() [][]key.Binding {
	return [][]key.Binding{
		{k.Add, k.Edit, k.Delete, k.Toggle},
		{k.Pri1, k.Pri2, k.Pri3, k.Move, k.Pick, k.PrevProject, k.NextProject},
		{k.Cycle, k.CycleRev, k.Filter, k.Reload, k.Help, k.Quit},
	}
}

func newUIKeys() uiKeys {
	return uiKeys{
		Quit:        key.NewBinding(key.WithKeys("q", "esc", "ctrl+c"), key.WithHelp("q", "quit")),
		Toggle:      key.NewBinding(key.WithKeys("enter", "space"), key.WithHelp("enter", "toggle done")),
		Add:         key.NewBinding(key.WithKeys("a"), key.WithHelp("a", "add")),
		Edit:        key.NewBinding(key.WithKeys("e"), key.WithHelp("e", "edit")),
		Delete:      key.NewBinding(key.WithKeys("d"), key.WithHelp("d", "delete")),
		Pri1:        key.NewBinding(key.WithKeys("1"), key.WithHelp("1", "high pri")),
		Pri2:        key.NewBinding(key.WithKeys("2"), key.WithHelp("2", "med pri")),
		Pri3:        key.NewBinding(key.WithKeys("3"), key.WithHelp("3", "low pri")),
		PrevProject: key.NewBinding(key.WithKeys("["), key.WithHelp("[", "prev project")),
		NextProject: key.NewBinding(key.WithKeys("]"), key.WithHelp("]", "next project")),
		Pick:        key.NewBinding(key.WithKeys("p"), key.WithHelp("p", "projects")),
		Move:        key.NewBinding(key.WithKeys("m"), key.WithHelp("m", "move to project")),
		Cycle:       key.NewBinding(key.WithKeys("tab"), key.WithHelp("tab", "next view")),
		CycleRev:    key.NewBinding(key.WithKeys("shift+tab"), key.WithHelp("shift+tab", "prev view")),
		Reload:      key.NewBinding(key.WithKeys("r"), key.WithHelp("r", "reload")),
		Filter:      key.NewBinding(key.WithKeys("/"), key.WithHelp("/", "filter")),
		Help:        key.NewBinding(key.WithKeys("?", "f1"), key.WithHelp("?", "help")),
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
	// pickerIdx is the cursor row in the modePicker overlay.
	pickerIdx int

	list   list.Model
	input  textinput.Model
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

	ti := textinput.New()
	ti.Prompt = "❯ "
	ti.SetWidth(60)

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

// createProject validates and adds a namespace, switches to it, and
// saves — so a project made in the TUI survives even without items.
func (m *uiModel) createProject(name string) error {
	if err := task.ValidateProjectName(name); err != nil {
		return err
	}
	if m.store.Project(name) != nil {
		return fmt.Errorf("project %q already exists", name)
	}
	m.setProject(name)
	m.persist(fmt.Sprintf("created project %q", name))
	return nil
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

// panelNames is the row set the overlay panel shows for the current
// mode: all projects for the picker, the other projects for move.
func (m uiModel) panelNames() []string {
	if m.mode == modeMove {
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

func (m uiModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		m.help.SetWidth(msg.Width)
		m.input.SetWidth(msg.Width - 6)
		m.resizeList()
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

	// Input modes own the keyboard: enter commits, esc cancels.
	if m.mode == modeAdd || m.mode == modeEdit || m.mode == modeNewProject {
		switch keyName {
		case "enter":
			text := strings.TrimSpace(m.input.Value())
			m.input.Blur()
			switch m.mode {
			case modeNewProject:
				if text == "" {
					m.mode = modePicker
					m.refresh("cancelled")
					return m, nil
				}
				if err := m.createProject(text); err != nil {
					m.status = err.Error()
					m.mode = modePicker
					return m, nil
				}
				m.mode = modeNormal
				m.resizeList()
				return m, nil
			default:
				if text != "" && m.mode == modeAdd {
					m.items = append(m.items, task.Item{Text: text, Priority: 2})
					m.persist(fmt.Sprintf("added %q", text))
				} else if text != "" && m.editingFileIdx >= 0 && m.editingFileIdx < len(m.items) {
					m.items[m.editingFileIdx].Text = text
					m.persist(fmt.Sprintf("edited %q", text))
				} else {
					m.refresh("cancelled")
				}
				m.mode = modeNormal
				return m, nil
			}
		case "esc":
			m.input.Blur()
			if m.mode == modeNewProject {
				m.mode = modePicker
				return m, nil
			}
			m.mode = modeNormal
			m.refresh("cancelled")
			return m, nil
		}
		var cmd tea.Cmd
		m.input, cmd = m.input.Update(msg)
		return m, cmd
	}

	// The project picker overlay owns the keyboard while open.
	if m.mode == modePicker {
		names := m.projectNames()
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
				m.setProject(names[m.pickerIdx])
			}
			m.mode = modeNormal
			m.resizeList()
			m.refresh("")
		case "n":
			m.mode = modeNewProject
			m.input.SetValue("")
			m.input.Placeholder = "new project name"
			m.input.Focus()
		case "esc", "q":
			m.mode = modeNormal
			m.resizeList()
		}
		return m, nil
	}

	// The move overlay works like the picker but lists only the other
	// projects; enter relocates the item that was selected when it
	// opened.
	if m.mode == modeMove {
		names := m.moveTargets()
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
				m.moveSelectedTo(names[m.pickerIdx])
			}
			m.mode = modeNormal
			m.resizeList()
		case "esc", "q":
			m.mode = modeNormal
			m.resizeList()
		}
		return m, nil
	}

	// While filtering, the list owns every key (typing, arrows, esc
	// to clear).
	if m.list.FilterState() != list.Unfiltered {
		var cmd tea.Cmd
		m.list, cmd = m.list.Update(msg)
		return m, cmd
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
		m.mode = modePicker
		m.pickerIdx = 0
		for i, n := range m.projectNames() {
			if n == m.project {
				m.pickerIdx = i
				break
			}
		}
		m.resizeList()
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
		m.mode = modeMove
		m.pickerIdx = 0
		m.resizeList()
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
		m.input.Placeholder = "new task"
		m.input.Focus()
		return m, nil

	case key.Matches(msg, m.keys.Edit):
		if it, ok := m.selected(); ok {
			m.mode = modeEdit
			m.editingFileIdx = it.row.fileIdx
			m.input.SetValue(it.row.item.Text)
			m.input.Placeholder = ""
			m.input.Focus()
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
			m.items = deleteItem(m.items, it.row.fileIdx)
			m.persist(fmt.Sprintf("deleted %q", it.row.item.Text))
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

// resizeList fits the list into the space below header, picker panel
// (when open), status/input line, and help.
func (m *uiModel) resizeList() {
	overhead := 4
	if m.help.ShowAll {
		overhead = 9
	}
	if m.mode == modePicker || m.mode == modeNewProject || m.mode == modeMove {
		overhead += len(m.panelNames()) + 6
	}
	h := m.height - overhead
	if h < 1 {
		h = 1
	}
	m.list.SetSize(m.width, h)
}

// pickerView renders the project panel: one row per namespace with
// its pending count, a cursor, and key hints.
func (m uiModel) pickerView() string {
	names := m.panelNames()
	title := "PROJECTS"
	hintText := "enter select · n new · esc close"
	if m.mode == modeMove {
		title = "MOVE TO PROJECT"
		hintText = "enter move · esc cancel"
	}
	nameW := 0
	for _, n := range names {
		if len(n) > nameW {
			nameW = len(n)
		}
	}
	rows := make([]string, 0, len(names))
	for i, n := range names {
		pending := 0
		if p := m.store.Project(n); p != nil {
			for _, it := range p.Items {
				if !it.Done {
					pending++
				}
			}
		}
		cursor := "  "
		if i == m.pickerIdx {
			cursor = "▸ "
		}
		padded := fmt.Sprintf("%-*s", nameW, n)
		if n == m.project {
			padded = projectStyle().Render(padded)
		}
		count := dimStyle().Render(fmt.Sprintf("%d pending", pending))
		rows = append(rows, cursor+padded+"  "+count)
	}
	hint := dimStyle().Render(hintText)
	panel := lipgloss.JoinVertical(lipgloss.Left, append(rows, hint)...)
	styledTitle := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("244")).Render(title)
	return lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("240")).
		Padding(0, 1).
		Render(lipgloss.JoinVertical(lipgloss.Left, styledTitle, panel))
}

func (m uiModel) View() tea.View {
	var v tea.View
	v.AltScreen = true

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
	header := lipgloss.JoinHorizontal(lipgloss.Left, append([]string{chip}, tabs...)...)

	body := m.list.View()

	var inputLine string
	switch m.mode {
	case modeAdd:
		inputLine = "add  " + m.input.View()
	case modeEdit:
		inputLine = "edit " + m.input.View()
	case modeNewProject:
		inputLine = "project " + m.input.View()
	case modePicker:
		if m.status != "" {
			inputLine = statusStyle(m.isDark).Render(m.status)
		}
	default:
		if m.status != "" {
			inputLine = statusStyle(m.isDark).Render(m.status)
		}
	}

	footer := m.help.View(m.keys)

	parts := []string{header}
	if m.mode == modePicker || m.mode == modeNewProject || m.mode == modeMove {
		parts = append(parts, m.pickerView())
	}
	parts = append(parts, body)
	if inputLine != "" {
		parts = append(parts, inputLine)
	}
	parts = append(parts, footer)
	v.SetContent(lipgloss.JoinVertical(lipgloss.Left, parts...))
	return v
}

// Styles — colors from the ANSI 16 palette so they adapt to themes.

func numStyle() lipgloss.Style {
	return lipgloss.NewStyle().Foreground(lipgloss.Color("240"))
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
