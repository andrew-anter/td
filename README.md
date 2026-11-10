# td

A small todo list for the terminal written in Go: a full interactive TUI
when run from a shell, plain scriptable commands when piped. Tasks live
as a JSON array in one file; configuration layers through CLI flags,
`TODO_`-prefixed environment variables, and a YAML config file.

[![license](https://img.shields.io/badge/license-Apache%202.0-blue)](LICENSE)
![go](https://img.shields.io/badge/Go-1.25-00ADD8)

## Features

- Interactive terminal UI (Bubble Tea) — browse, add, edit, toggle, filter
- Plain CLI subcommands for scripts; bare `td` piped to a file falls
  back to printing the pending list
- Single binary (`td`), single JSON data file, atomic crash-safe writes
- Inter-process locking — concurrent `td` commands never lose updates
- Priority levels (1 high, 2 default, 3 low) with stable pending-first sorting
- Item numbers are globally consistent: any number you see — in the TUI
  or `td list` — is directly actionable by `complete`, `edit`, `reopen`,
  and `delete`
- Machine-readable output (`--json`, `--count`) for scripts and status bars
- Bash / zsh / fish completions

## Requirements

| Purpose      | Need          |
| ------------ | ------------- |
| Build & test | Go ≥ 1.25     |

## Build

```sh
go build -o td .    # or: make build
```

## Install

```sh
make install
```

Installs the binary and shell completions:

- binary: `~/.local/bin/td`
- bash:   `~/.local/share/bash-completion/completions/td`
- zsh:    `~/.local/share/zsh/site-functions/_td` — put that directory in
  `$fpath` *before* `compinit`, e.g.
  `fpath=(~/.local/share/zsh/site-functions $fpath)` in `.zshrc`
- fish:   `~/.local/share/fish/vendor_completions.d/td.fish`

`make uninstall` removes everything listed above.

## Usage

```sh
td                       # open the interactive TUI
td add "buy milk"
td add "write report" --priority 1
td list                  # alias: ls — pending items
td list --all            # everything, pending and completed
td completed             # completed items only
td edit 2 "buy oat milk" # replace item #2's text
td complete 1            # aliases: c, comp — mark item #1 done
td reopen 1              # alias: uncomplete — mark item #1 pending
td delete 1              # aliases: rm, del — remove item #1
td delete --completed    # remove every completed task
```

### Commands

| Command           | Aliases      | Description                                                  |
| ----------------- | ------------ | ------------------------------------------------------------ |
| `add <text>…`     | —            | Append one or more items (at least one required).             |
| `list`            | `ls`         | Print items in display order (pending first, then priority).  |
| `completed`       | —            | Print only completed items.                                   |
| `edit <n> <text>` | —            | Replace the text of item `n`.                                 |
| `complete <n>`    | `c`, `comp`  | Mark item `n` as done.                                        |
| `reopen <n>`      | `uncomplete` | Mark item `n` as pending (silent no-op if already pending).    |
| `delete <n>`      | `rm`, `del`  | Delete item `n`, or with `--completed` delete every done item. |

Running `td` with no subcommand opens the terminal UI (see below). When
stdout is not a terminal — pipes, redirects, cron — bare `td` prints the
pending list instead, exactly like `td list`.

### Flags

| Flag                    | Applies to | Default              | Description                                            |
| ----------------------- | ---------- | -------------------- | ------------------------------------------------------ |
| `-p, --priority N`      | `add`      | `2`                  | Priority: `1` high, `2` normal, `3` low. Anything else is rejected and nothing is written. |
| `-a, --all`             | `list`     | `false`              | Include completed items.                               |
| `-d, --completed`       | `list`     | `false`              | Show only completed items.                             |
| `--json`                | `list`     | `false`              | JSON array with a global 1-based `Index` per item.     |
| `--count`               | `list`     | `false`              | One-line `{"pending":N,"completed":N,"total":N}`. Ignores filters. |
| `--completed`           | `delete`   | `false`              | Batch-delete all completed tasks instead of one item.  |
| `-f, --datafile <path>` | global     | `$HOME/.tasks.json`  | Data file location.                                    |
| `-c, --config <path>`   | global     | `$HOME/.todo.yaml`   | Config file location.                                  |

## Item numbers and ordering

Display order is: pending items first, then completed; within each group,
priority ascending (`1` before `3`); otherwise stable insertion order.

The number shown next to an item is its position in that **full sorted
list**, identical in every view (`list`, `list --all`, `completed`,
`--json`). A number you see anywhere can be pasted straight into
`complete`, `edit`, `reopen`, or `delete` and will hit exactly that item.
Numbers shift when items are added or deleted, so re-read the list after
making changes.

The human table prints `(1)` / `(3)` next to items whose priority is
high or low; normal-priority items have no marker.

## JSON output

`td list --json` emits the data-file shape plus the global `Index`:

```sh
$ td list --json
[{"Index":1,"Text":"write report","Priority":1,"Done":false},
 {"Index":2,"Text":"buy milk","Priority":2,"Done":false}]
```

Filters (`--all`, `--completed`) apply to `--json` output as well; item
indices stay global.

`td list --count` is a single line for status bars:

```sh
$ td list --count
{"pending":2,"completed":0,"total":2}
```

## Configuration

Resolution precedence: **CLI flag → environment variable → config file →
default**.

- Config file: YAML at `$HOME/.todo.yaml`, created automatically on first
  run (override with `--config`).
- Environment: `TODO_` prefix, key names uppercased —
  `TODO_DATAFILE=/tmp/x.json td list`.

Example `.todo.yaml`:

```yaml
datafile: /home/you/.tasks.json
```

## Data file

Tasks are stored as a JSON array:

```json
[
  { "Text": "buy milk",     "Priority": 2, "Done": false },
  { "Text": "write report", "Priority": 1, "Done": false }
]
```

Two safety guarantees:

- **Atomic writes** — contents go to a sibling `*.tmp-*` file and are
  renamed into place; a crash mid-write cannot corrupt the list.
- **Locked mutations** — every mutating command holds an exclusive
  `flock` on a `<datafile>.lock` sidecar across its whole
  read-modify-write cycle, so parallel `td` invocations (e.g. a TUI
  save racing a CLI save) cannot lose updates.

The data file and its lock/config files are gitignored; they are yours
and never leave your machine via this repo.

## Terminal UI

Run `td` with no arguments:

```
 td ─ pending (3) · all (8) · completed (5)
 ▸  1.      urgent call
    2.  X   old task A
    3.      write blog
    add ▸ █
    added "buy milk"
    a: add · enter: toggle done · e: edit · d: delete · 1/2/3: priority
```

| Key          | Action                                              |
| ------------ | --------------------------------------------------- |
| `j`/`k`, arrows | move cursor                                       |
| `enter`/`space` | toggle the selected item done ↔ pending           |
| `a`          | add a new task (enter commits, esc cancels)          |
| `e`          | edit the selected item's text                        |
| `d`          | delete the selected item                             |
| `1` / `2` / `3` | set the selected item's priority                  |
| `tab` / `shift+tab` | cycle views: pending → all → completed        |
| `/`          | fuzzy-filter the current view; `esc` clears          |
| `r`          | reload the data file from disk                       |
| `?`          | toggle the full keymap                               |
| `q`, `esc`, `ctrl+c` | quit                                         |

Every mutation is locked and saved immediately, and the numbers shown are
the same global numbers the CLI accepts — a row labeled `4.` in the
completed view is `td delete 4` / `td reopen 4`. Because the TUI holds a
full snapshot, changes made by *other* processes while the UI is open are
picked up by pressing `r` (the last save wins).

**Waybar / status bars:** for a count in any status bar, exec the CLI —
e.g. waybar:

```json
"custom/td": { "exec": "td list --count | jq -r '.pending'", "interval": 30 }
```

## Makefile

| Target            | What it does                                          |
| ----------------- | ----------------------------------------------------- |
| `build`           | Compile `./td`                                          |
| `run`             | Launch the TUI via `go run .`                           |
| `test` / `vet` / `fmt` / `tidy` | Go test, vet, format, module tidy         |
| `install`         | Build, install binary + shell completions               |
| `uninstall`       | Remove everything `install` placed                      |
| `clean`           | Delete the local `td` binary                            |

## Project layout

```
main.go              # entry point: calls cmd.Execute()
cmd/                 # cobra commands (flag parsing, output, orchestration)
  root.go            # root command: TUI launcher, CLI fallback, config bootstrap
  ui.go              # the Bubble Tea terminal UI (views, keymap, delegate)
  add.go             # `add`
  list.go            # `list` / `ls`, shared runList + JSON/count modes
  complete.go        # `complete` / `c` / `comp`
  reopen.go          # `reopen` / `uncomplete`
  completed.go       # `completed`
  edit.go            # `edit`
  delete.go          # `delete` / `rm` / `del`, single and --completed
task/                # domain types and persistence
  task.go            # Item, sort order, SaveItems, ReadItems
  lock.go            # advisory flock on a .lock sidecar (unix; no-op elsewhere)
```

## Tests

```sh
go test ./...    # or: make test
```

## License

Released under the [Apache License 2.0](LICENSE). Copyright 2025 Andrew Anter.
