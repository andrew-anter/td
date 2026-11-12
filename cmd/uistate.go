/*
Copyright © 2025 Andrew Anter <andrew.anter@gmail.com>
*/
package cmd

import (
	"encoding/json"
	"os"
	"path/filepath"
)

// uiState remembers where the TUI was left: which project and tab,
// and the text of the selected task so the cursor can find it again.
type uiState struct {
	Project string `json:"project"`
	View    string `json:"view"`
	Task    string `json:"task,omitempty"`
}

// uiStatePath resolves the shared state file; keyed by data file so
// alternate -f stores keep their own memory. XDG_STATE_HOME (or a
// test override) decides the base directory.
func uiStatePath() (string, error) {
	if dir := os.Getenv("XDG_STATE_HOME"); dir != "" {
		return filepath.Join(dir, "td", "ui.json"), nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".local", "state", "td", "ui.json"), nil
}

func stateKey(dataFile string) string {
	if a, err := filepath.Abs(dataFile); err == nil {
		return a
	}
	return dataFile
}

func readUIStates() map[string]uiState {
	p, err := uiStatePath()
	if err != nil {
		return nil
	}
	raw, err := os.ReadFile(p)
	if err != nil {
		return nil
	}
	var all map[string]uiState
	if json.Unmarshal(raw, &all) != nil {
		return nil
	}
	return all
}

func readUIState(dataFile string) (uiState, bool) {
	st, ok := readUIStates()[stateKey(dataFile)]
	return st, ok
}

// writeUIState is best-effort: losing UI memory must never fail a
// quit path.
func writeUIState(dataFile string, st uiState) {
	p, err := uiStatePath()
	if err != nil {
		return
	}
	all := readUIStates()
	if all == nil {
		all = map[string]uiState{}
	}
	all[stateKey(dataFile)] = st
	raw, err := json.MarshalIndent(all, "", "  ")
	if err != nil {
		return
	}
	if os.MkdirAll(filepath.Dir(p), 0o755) != nil {
		return
	}
	_ = os.WriteFile(p, raw, 0o644)
}

// restoreUIState routes a fresh model back to the project/tab it was
// left in; an explicit -P keeps priority over the remembered project,
// and unknown names or tabs fall back to defaults.
func restoreUIState(m *uiModel, st uiState, projectFlagSet bool) {
	if st.Project != "" && !projectFlagSet && m.store.Project(st.Project) != nil {
		m.setProject(st.Project)
	}
	switch st.View {
	case "all":
		m.view = viewAll
	case "completed":
		m.view = viewDone
	}
	m.refresh("")
	m.selectTaskText(st.Task)
}

// selectTaskText parks the cursor on the first row with this text;
// deleted or renamed tasks simply leave the default selection.
func (m *uiModel) selectTaskText(text string) {
	if text == "" {
		return
	}
	for k, it := range m.list.Items() {
		if u, ok := it.(uiItem); ok && u.row.item.Text == text {
			m.list.Select(k)
			return
		}
	}
}

func uiStateFrom(m uiModel) uiState {
	st := uiState{Project: m.project, View: m.view.label()}
	if it, ok := m.selected(); ok {
		st.Task = it.row.item.Text
	}
	return st
}
