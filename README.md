# td

A small, dependency-light command-line todo list written in Go. Tasks live
as a JSON array in one file; configuration layers through CLI flags,
`TODO_`-prefixed environment variables, and a YAML config file.

[![license](https://img.shields.io/badge/license-Apache%202.0-blue)](LICENSE)
![go](https://img.shields.io/badge/Go-1.25-00ADD8)

## Features

- Single binary (`td`), single JSON data file, atomic crash-safe writes
- Inter-process locking — concurrent `td` commands never lose updates
- Priority levels (1 high, 2 default, 3 low) with stable pending-first sorting
- Item numbers are globally consistent: any number you see is directly
  actionable by `complete`, `edit`, `reopen`, and `delete`
- Machine-readable output (`--json`, `--count`) for scripts and status bars
- fzf picker + waybar module out of the box (optional integration scripts)
- Bash / zsh / fish completions

## Requirements

| Purpose          | Need                                        |
| ---------------- | ------------------------------------------- |
| Build & test     | Go ≥ 1.25                                   |
| fzf integration  | `fzf`, `jq`, `gum`, `notify-send`           |
| Waybar module    | waybar ≥ 0.9 (custom `exec` module)         |

## Build

```sh
go build -o td .    # or: make build
```

## Install

```sh
make install
```

Installs the binary, shell completions, and the fzf/waybar scripts:

- binary: `~/.local/bin/td`
- bash:   `~/.local/share/bash-completion/completions/td`
- zsh:    `~/.local/share/zsh/site-functions/_td` — put that directory in
  `$fpath` *before* `compinit`, e.g.
  `fpath=(~/.local/share/zsh/site-functions $fpath)` in `.zshrc`
- fish:   `~/.local/share/fish/vendor_completions.d/td.fish`
- scripts: `~/.local/bin/td-{fzf,waybar,render,reload,cycle-view,act,add}`

`make fzf-integration` installs only the scripts; `make uninstall` removes
everything listed above.

## Usage

```sh
td                       # what you should be working on (pending, sorted)
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

Running `td` with no subcommand is equivalent to `td list`.

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
  read-modify-write cycle, so parallel `td` invocations (e.g. rapid
  clicks in the fzf picker) cannot lose updates.

The data file and its lock/config files are gitignored; they are yours
and never leave your machine via this repo.

## fzf / waybar integration

Scripts in `fzf/` turn `td` into a desktop workflow: waybar shows the
pending count with a Nerd Font icon, and clicking it opens an fzf picker
(enter toggles done, ctrl-d deletes, ctrl-a adds via gum, ctrl-t cycles
pending → all → completed). The picker uses `--json` indices, so its
numbers are actionable against the same global order.

```sh
make fzf-integration    # or included in `make install`
```

Waybar config snippets, CSS, and full keybinding details:
[`fzf/README.md`](fzf/README.md).

## Makefile

| Target            | What it does                                          |
| ----------------- | ----------------------------------------------------- |
| `build`           | Compile `./td`                                          |
| `run`             | `go run .`                                              |
| `test` / `vet` / `fmt` / `tidy` | Go test, vet, format, module tidy         |
| `install`         | Build, install binary + completions + fzf scripts       |
| `fzf-integration` | Install only the fzf/waybar scripts to `~/.local/bin`   |
| `uninstall`       | Remove everything `install` placed                      |
| `clean`           | Delete the local `td` binary                            |

## Project layout

```
main.go              # entry point: calls cmd.Execute()
cmd/                 # cobra commands (flag parsing, output, orchestration)
  root.go            # root command, config bootstrap, data-file bootstrap
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
fzf/                 # fzf / waybar integration (see fzf/README.md)
  td-waybar          # waybar exec — emits count + tooltip as JSON
  td-fzf             # main fzf picker
  td-render          # render the list for a view
  td-reload          # re-render after an action
  td-cycle-view      # bump the current view in the state file
  td-act             # run an action + desktop notification
  td-add             # gum prompt for adding a new task
```

## Tests

```sh
go test ./...    # or: make test
```

## License

Released under the [Apache License 2.0](LICENSE). Copyright 2025 Andrew Anter.
