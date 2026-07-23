# fzf / waybar integration

A small set of shell scripts that turn `td` into a clickable waybar
module backed by an interactive fzf picker.

## What you get

- A waybar module on the right side showing the pending-task count.
  Hover for the full list, click to open the picker.
- An fzf picker that stays open across actions:
  - **enter** — toggle (complete a pending item, reopen a done one)
  - **ctrl-d** — delete
  - **ctrl-a** — add new (gum prompt)
  - **ctrl-t** — cycle view: pending → all → completed → pending
  - **esc** — close

## Requirements

- `td` (this project) — installed and on `$PATH`
- `fzf` — fuzzy finder
- `jq` — JSON parsing
- `gum` — only needed for the "add new" prompt (`ctrl-a` in the picker)
- `notify-send` — desktop notifications (usually in `libnotify`)
- `kitty` — terminal used by the waybar click handler
  (swap for your terminal of choice in the waybar config below)

## Install

```sh
make fzf-integration
```

This copies the seven scripts in this directory to
`~/.local/bin/` (executable). `make install` runs this target
automatically.

To uninstall:

```sh
make uninstall
```

Removes `td-waybar`, `td-fzf`, `td-render`, `td-reload`,
`td-cycle-view`, `td-act`, and `td-add` from `~/.local/bin/`.

## Waybar configuration

Add a `custom/td` block to your waybar config. It must be added to
`modules-right` (or wherever you want it) and the file `style.css`
needs matching rules.

`~/.config/waybar/config.jsonc`:

```jsonc
"modules-right": [
    /* ... your other modules ... */
    "custom/td"
],

"custom/td": {
    "exec": "~/.local/bin/td-waybar",
    "interval": 30,
    "return-type": "json",
    "format": "{text}",
    "tooltip-format": "{tooltip}",
    "on-click": "kitty -e td-fzf",
    "on-click-right": "td-add"
}
```

Swap `kitty` for whatever terminal you use (`foot`, `alacritty`,
`gnome-terminal`, ...). Adjust the `modules-right` position to taste.

`~/.config/waybar/style.css`:

```css
#custom-td {
    padding: 0 12px;
    margin: 4px 2px;
    font-weight: bold;
}

#custom-td.pending { color: #f9e2af; }  /* warm while tasks are open  */
#custom-td.empty   { color: #5d6b74; }  /* dim when nothing pending   */
```

Reload waybar after editing:

```sh
killall -USR2 waybar        # SIGUSR2 = reload without restart
```

## How the pieces fit

```
waybar click  →  td-fzf  →  fzf picker
                          ├── enter    → td-act toggle  → td complete|reopen
                          ├── ctrl-d   → td-act delete  → td delete
                          ├── ctrl-a   → td-add         → gum input → td add
                          └── ctrl-t   → td-cycle-view  → bumps view in state file

After every action, fzf reloads via td-reload, which re-renders the
list for the current view (tracked in $TD_FZF_STATE).
```

The waybar exec script `td-waybar` runs every 30 s (or on reload)
and emits `{"text", "tooltip", "class"}` so waybar shows the icon +
count and a hover tooltip with the full list.

## Files

| File             | Purpose                                                      |
| ---------------- | ------------------------------------------------------------ |
| `td-waybar`      | Waybar exec — outputs the count + tooltip as JSON            |
| `td-fzf`         | Main fzf picker (interactive UI)                             |
| `td-render`      | Emit the list for a view in fzf's input format               |
| `td-reload`      | Re-render the list for the current view (reads state file)   |
| `td-cycle-view`  | Update the state file with the next view                     |
| `td-act`         | Run an action (complete / reopen / toggle / delete) + notify |
| `td-add`         | Prompt for a new task with gum and add it                    |
