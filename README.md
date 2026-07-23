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
td delete 1            # aliases: rm, del; remove item #1
td delete --completed  # remove every completed task
```

### Commands

| Command         | Aliases      | Description                                       |
| --------------- | ------------ | ------------------------------------------------- |
| `add <text>…`   | —            | Append one or more items to the list.             |
| `list`          | `ls`         | Print items, sorted by priority.                  |
| `completed`     | —            | Print only completed items.                       |
| `complete <n>`  | `c`, `comp`  | Mark the nth item (as shown by `list`) as complete. |
| `delete <n>`    | `rm`, `del`  | Delete the nth item, or `--completed` to remove all completed tasks. |

### Flags

| Flag                  | Applies to | Default                   | Description                |
| --------------------- | ---------- | ------------------------- | -------------------------- |
| `-p, --priority N`    | `add`      | `2`                       | Priority `1` (high), `2`, `3` (low). |
| `-a, --all`           | `list`     | `false`                   | Show complete and pending items. |
| `-d, --completed`     | `list`     | `false`                   | Show only completed items. |
| `--completed`         | `delete`   | `false`                   | Delete every completed task (instead of a single item). |
| `-f, --datafile <path>` | global    | `$HOME/.tasks.json`       | Where tasks are stored.    |
| `-c, --config <path>`  | global    | `$HOME/.todo.yaml`        | Path to the config file.   |

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

## Project Layout

```
main.go              # entry point: calls cmd.Execute()
cmd/                 # cobra commands
  root.go            # root command, config bootstrap, data-file bootstrap
  add.go             # `add` command
  list.go            # `list` / `ls` command
  complete.go        # `complete` / `c` / `comp` command
  completed.go       # `completed` command
  delete.go          # `delete` / `rm` / `del` command
task/                # domain types and persistence
  task.go            # Item, sort order, SaveItems, ReadItems
  task_test.go       # unit tests
```

## Tests

```sh
go test ./...
```
