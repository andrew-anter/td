# td

A small todo list for the terminal written in Go: a full interactive TUI
when run from a shell, plain scriptable commands when piped. Tasks live
inside **projects** — namespaces each with its own list and numbering —
in one JSON file; configuration layers through CLI flags,
`TODO_`-prefixed environment variables, and a YAML config file.

[![license](https://img.shields.io/badge/license-Apache%202.0-blue)](LICENSE)
![go](https://img.shields.io/badge/Go-1.25-00ADD8)

## Features

- Interactive terminal UI (Bubble Tea) — browse, add, edit, toggle, filter
- Projects as namespaces: `td project create gym`, then
  `td add -P gym "squat"`; every command targets one project (`-P`)
- Plain CLI subcommands for scripts; bare `td` piped to a file falls
  back to printing the pending list
- Single binary (`td`), single JSON data file, atomic crash-safe writes
- Inter-process locking — concurrent `td` commands never lose updates
- Priority levels (1 high, 2 default, 3 low) with stable pending-first sorting
- Item numbers are consistent within a project: any number you see —
  in the TUI or `td list` — is directly actionable by `complete`, `edit`,
  `reopen`, and `delete`
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
td project create gym    # new namespace
td add -P gym "squat"    # add into a project
td list                  # alias: ls — pending items (default project)
td list -P gym           # pending items of "gym"
td list --all            # everything, pending and completed
td completed             # completed items only
td project               # list projects with counts (alias: projects)
td move 2 gym            # aliases: mv — item #2 of the current project → gym
td edit 2 "buy oat milk" # replace item #2's text
td complete 1            # aliases: c, comp — mark item #1 done
td reopen 1              # alias: uncomplete — mark item #1 pending
td delete 1              # aliases: rm, del — remove item #1
td delete --completed    # remove every completed task
```

### Commands

| Command           | Aliases      | Description                                                  |
| ----------------- | ------------ | ------------------------------------------------------------ |
| `add <text>…`     | —            | Append one or more items to the current project (at least one required). |
| `list`            | `ls`         | Print items in display order (pending first, then priority).  |
| `completed`       | —            | Print only completed items.                                   |
| `edit <n> <text>` | —            | Replace the text of item `n`.                                 |
| `complete <n>`    | `c`, `comp`  | Mark item `n` as done.                                        |
| `reopen <n>`      | `uncomplete` | Mark item `n` as pending (silent no-op if already pending).    |
| `delete <n>`      | `rm`, `del`  | Delete item `n`, or with `--completed` delete every done item. |
| `move <n> <proj>` | `mv`         | Move item `n` from the current project into `<proj>` (must exist; state is kept). |
| `project`         | `projects`   | List projects with pending/done/total counts; the active one is marked `*`. |
| `project create <name>` | —      | Create a new project. Names may contain spaces (quote them on the CLI); leading/trailing whitespace is trimmed, control characters are rejected. |
| `project delete <name>` | —     | Delete a project and its items (`--force` required when it still has items). |
| `project rename <old> <new>` | — | Rename a project, keeping its items.                          |

Running `td` with no subcommand opens the terminal UI (see below). When
stdout is not a terminal — pipes, redirects, cron — bare `td` prints the
pending list instead, exactly like `td list`.

### Flags

| Flag                    | Applies to | Default              | Description                                            |
| ----------------------- | ---------- | -------------------- | ------------------------------------------------------ |
| `-P, --project <name>`  | all item commands | `inbox`        | Project namespace to operate on. Unknown names error (except `inbox`, which always exists); `td list --count` ignores it. |
| `-p, --priority N`      | `add`      | `2`                  | Priority: `1` high, `2` normal, `3` low. Anything else is rejected and nothing is written. |
| `-a, --all`             | `list`     | `false`              | Include completed items.                               |
| `-d, --completed`       | `list`     | `false`              | Show only completed items.                             |
| `--json`                | `list`     | `false`              | JSON array with a 1-based `Index` per item.            |
| `--count`               | `list`     | `false`              | One-line `{"pending":N,"completed":N,"total":N}` across **all projects**. Ignores filters and `-P`. |
| `--completed`           | `delete`   | `false`              | Batch-delete all completed tasks instead of one item.  |
| `--force`               | `project delete` | `false`        | Delete the project even when it still contains items.  |
| `-f, --datafile <path>` | global     | `$HOME/.tasks.json`  | Data file location.                                    |
| `-c, --config <path>`   | global     | `$HOME/.todo.yaml`   | Config file location.                                  |

## Projects and item numbers

Projects are namespaces. Every item command (`add`, `list`, `complete`,
`edit`, `reopen`, `delete`, `completed`, `move`) operates on exactly one
project, chosen with `-P` / `--project` (or the `project` config key);
when omitted, the default project `inbox` is used. Item `3` of `gym`
and item `3` of `inbox` are different, unrelated items.

`add -P gym "squat"` requires `gym` to exist — `td project create gym`
first. Listing an unknown project errors; `inbox` always resolves, even
on a fresh data file.

Display order within a project is: pending items first, then completed;
within each group, priority ascending (`1` before `3`); otherwise
stable insertion order.

The number shown next to an item is its position in that **full sorted
list of its project**, identical in every view (`list`, `list --all`,
`completed`, `--json`). A number you see anywhere can be pasted straight
into `complete`, `edit`, `reopen`, or `delete` — with the same `-P` —
and will hit exactly that item. Numbers shift when items are added or
deleted, so re-read the list after making changes.

The human table prints `(1)` / `(3)` next to items whose priority is
high or low; normal-priority items have no marker.

## JSON output

`td list --json` emits the selected project's items plus their
in-project `Index`:

```sh
$ td list --json
[{"Index":1,"Text":"write report","Priority":1,"Done":false},
 {"Index":2,"Text":"buy milk","Priority":2,"Done":false}]
```

Filters (`--all`, `--completed`) apply to `--json` output as well; item
indices stay consistent across views of the same project.

`td list --count` is a single line for status bars, summed across
**every** project regardless of `-P` or filters:

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
  `TODO_DATAFILE=/tmp/x.json td list`, `TODO_PROJECT=gym td list`.

Example `.todo.yaml`:

```yaml
datafile: /home/you/.tasks.json
project: gym   # default namespace when no -P is given (default: inbox)
```

## Data file

Tasks are stored as projects, each holding its items:

```json
{
  "projects": [
    { "name": "inbox", "items": [
      { "Text": "buy milk",     "Priority": 2, "Done": false },
      { "Text": "write report", "Priority": 1, "Done": false }
    ] },
    { "name": "gym", "items": [
      { "Text": "squat", "Priority": 2, "Done": false }
    ] }
  ]
}
```

Old pre-project files — a bare JSON array of items — keep working: on
read they migrate transparently into the `inbox` project, and the new
shape is written back on the first save. Reading never mutates the file.

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
 td ─ [inbox] · pending (3) · all (8) · completed (5)
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
| `m`          | move the selected item to another project (opens the projects screen; enter picks the destination) |
| `p`          | projects screen: enter select, `a` new, `e` rename, `d` delete (empty only), esc back |
| `[` / `]`    | switch to the previous / next project                |
| `tab` / `shift+tab` | cycle views: pending → all → completed        |
| `/`          | fuzzy-filter the current view; `esc` clears          |
| `r`          | reload the data file from disk                       |
| `?`          | toggle the full keymap                               |
| `q`, `esc`, `ctrl+c` | quit                                         |

The project chip in the header shows which namespace the list, the
numbers, and every mutation belong to — with a position counter (`2/4`)
when more than one project exists. Press `p` for the full-page projects
screen — just the project rows (name, pending/done counts, current
marked), no tab bar — where the same verbs keep their meaning: `a`
creates a project, `e` renames the cursor's project, `d` deletes it
(only once empty, so a keystroke can never destroy a list), and enter
selects one and drops back to its tasks. Pressing `m` on an item opens
the same screen as a destination picker. Every mutation is locked and
saved immediately, and the numbers shown are the same per-project
numbers the CLI accepts with the same `-P` — a row labeled `4.` in the
completed view is `td delete 4` / `td reopen 4` in that project.
Because the TUI holds a full snapshot, changes made by *other*
processes while the UI is open are picked up by pressing `r` (the last
save wins).

**Waybar / status bars:** for a count in any status bar, exec the CLI —
e.g. waybar (the `format` glyph is FA Free "hat-wizard", U+F6E8 — use
any glyph your bar font actually ships):

```jsonc
"custom/td": {
    "exec": "td list --count | jq -r '.pending'",
    "interval": 30,
    "format": " {}",
    "on-click": "kitty -e td"
}
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
  ui.go              # the Bubble Tea terminal UI (views, projects, keymap, delegate)
  add.go             # `add`
  list.go            # `list` / `ls`, shared runList + JSON/count modes
  project.go         # `project` list/create/delete/rename + project resolution
  move.go            # `move` / `mv` — cross-project item relocation
  complete.go        # `complete` / `c` / `comp`
  reopen.go          # `reopen` / `uncomplete`
  completed.go       # `completed`
  edit.go            # `edit`
  delete.go          # `delete` / `rm` / `del`, single and --completed
task/                # domain types and persistence
  task.go            # Item, Project, Store, sort order, SaveStore, ReadStore (+ legacy migration)
  lock.go            # advisory flock on a .lock sidecar (unix; no-op elsewhere)
```

## Tests

```sh
go test ./...    # or: make test
```

## License

Released under the [Apache License 2.0](LICENSE). Copyright 2025 Andrew Anter.
