# todo

A small, dependency-light command-line todo list written in Go. Tasks are stored
as JSON in a single file, and configuration is layered through flags, a YAML
config, and environment variables.

## Build

```sh
go build -o td .
```

The resulting binary is `td` (the project also runs as `go run .`).

## Install

```sh
make install
```

This builds the binary and installs it, plus generated shell completions, to:

- binary: `~/.local/bin/td`
- bash:   `~/.local/share/bash-completion/completions/td`
- zsh:    `~/.local/share/zsh/site-functions/_td` (ensure that directory is in
  your `$fpath` before `compinit`, e.g.
  `fpath=(~/.local/share/zsh/site-functions $fpath)` in your `.zshrc`)
- fish:   `~/.local/share/fish/vendor_completions.d/td.fish`

`make uninstall` removes all of the above.

## Usage

```sh
td                      # show pending work (what you should be doing)
td add "buy milk"
td add "write report" --priority 1
td list                 # alias: ls; show pending items
td list --all           # everything, complete or not
td completed            # show only completed items
td complete 1           # aliases: c, comp; mark item #1 as complete
td reopen 1             # aliases: uncomplete; mark item #1 as not complete
td delete 1            # aliases: rm, del; remove item #1
td delete --completed  # remove every completed task
```

### Commands

| Command         | Aliases            | Description                                       |
| --------------- | ------------------ | ------------------------------------------------- |
| `add <text>…`   | —                  | Append one or more items to the list.             |
| `list`          | `ls`               | Print items, sorted by priority.                  |
| `completed`     | —                  | Print only completed items.                       |
| `complete <n>`  | `c`, `comp`        | Mark the nth item (as shown by `list`) as complete. |
| `reopen <n>`    | `uncomplete`       | Mark the nth item as not complete (no-op if already pending). |
| `delete <n>`    | `rm`, `del`        | Delete the nth item, or `--completed` to remove all completed tasks. |

### Flags

| Flag                  | Applies to | Default                   | Description                |
| --------------------- | ---------- | ------------------------- | -------------------------- |
| `-p, --priority N`    | `add`      | `2`                       | Priority `1` (high), `2`, `3` (low). |
| `-a, --all`           | `list`     | `false`                   | Show complete and pending items. |
| `-d, --completed`     | `list`     | `false`                   | Show only completed items. |
| `--json`              | `list`     | `false`                   | Emit items as a JSON array (display order, with 1-based `Index`). |
| `--count`             | `list`     | `false`                   | Emit `{"pending","completed","total"}` as JSON. Ignores filter flags. |
| `--completed`         | `delete`   | `false`                   | Delete every completed task (instead of a single item). |
| `-f, --datafile <path>` | global    | `$HOME/.tasks.json`       | Where tasks are stored.    |
| `-c, --config <path>`  | global    | `$HOME/.todo.yaml`        | Path to the config file.   |

### JSON output

`td list --json` is useful for scripts and status bars (e.g. waybar). The
array mirrors the data-file shape plus a 1-based `Index` matching the
human list:

```sh
$ td list --json
[{"Index":1,"Text":"write report","Priority":1,"Done":false},
 {"Index":2,"Text":"buy milk","Priority":2,"Done":false}]
```

`td list --count` returns a single line:

```sh
$ td list --count
{"pending":2,"completed":0,"total":2}
```

## Configuration

Precedence is, in order: CLI flag → environment variable → config file → default.

The config file is YAML, looked up at `$HOME/.todo.yaml` (override with
`--config`). It is created automatically on first run if missing.

Environment variables use the `TODO_` prefix and map to the same keys, e.g.
`TODO_DATAFILE=/tmp/x.json td list`.

Example `.todo.yaml`:

```yaml
datafile: /home/you/.tasks.json
```

## Data File

Tasks are stored as a JSON array of objects:

```json
[
  { "Text": "buy milk",   "Priority": 2, "Done": false },
  { "Text": "write report", "Priority": 1, "Done": false }
]
```

Writes are atomic: the new contents go to a sibling `*.tmp-*` file and are
renamed into place, so a crash mid-write cannot corrupt the list.

## fzf / waybar integration

A small set of shell scripts in `fzf/` turns `td` into a clickable
waybar module backed by an interactive fzf picker. The waybar module
shows the pending count and opens a fuzzy picker on click (enter to
toggle, ctrl-d to delete, ctrl-a to add, ctrl-t to cycle view).

Install the scripts:

```sh
make fzf-integration
```

This copies the scripts to `~/.local/bin/`. `make install` runs it
automatically. See [`fzf/README.md`](fzf/README.md) for the waybar
config snippets and full usage.

## Project Layout

```
main.go              # entry point: calls cmd.Execute()
cmd/                 # cobra commands
  root.go            # root command, config bootstrap, data-file bootstrap
  add.go             # `add` command
  list.go            # `list` / `ls` command
  complete.go        # `complete` / `c` / `comp` command
  reopen.go          # `reopen` / `uncomplete` command
  completed.go       # `completed` command
  delete.go          # `delete` / `rm` / `del` command
task/                # domain types and persistence
  task.go            # Item, sort order, SaveItems, ReadItems
  task_test.go       # unit tests
fzf/                 # fzf / waybar integration (see fzf/README.md)
  td-waybar          # waybar exec — emits count + tooltip as JSON
  td-fzf             # main fzf picker
  td-render          # render the list for a view
  td-reload          # re-render after an action
  td-cycle-view      # bump the current view in the state file
  td-act             # run an action + notify
  td-add             # gum prompt for adding a new task
```

## Tests

```sh
go test ./...
```
