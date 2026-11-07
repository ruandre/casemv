# casemv

Rename files and directories to a naming convention. Linux. Go port of
[stdrename](https://github.com/Gadiguibou/stdrename).

```sh
go install github.com/ruandre/casemv@latest
casemv [convention] [target] [flags]
```

## Conventions

| Name        | `My FILE name-2` becomes |
| ----------- | ------------------------ |
| `camel`     | `myFileName2`            |
| `kebab`     | `my-file-name-2`         |
| `lower`     | `my file name-2`         |
| `pascal`    | `MyFileName2`            |
| `screaming` | `MY_FILE_NAME_2`         |
| `sentence`  | `My file name 2`         |
| `snake`     | `my_file_name_2`         |
| `title`     | `My File Name 2`         |
| `train`     | `My-File-Name-2`         |

Words split on separators, case humps, and acronyms (`XMLHttpRequest` →
`xml http request`). `lower` only lowercases.

## Flags

| Flag               | Effect                                       |
| ------------------ | -------------------------------------------- |
| `-r, --recursive`  | Descend into subdirectories                  |
| `-D, --dir`        | Rename directories too                       |
| `-H, --hidden`     | Include dot-prefixed entries                 |
| `-e, --exclude`    | Skip glob matches and their subtrees; repeat |
| `-n, --dry-run`    | Print plan, change nothing                   |
| `-q, --quiet`      | No output                                    |
| `-x, --lower-ext`  | Lowercase file extensions too                |
| `-c, --convention` | Convention when none is given as an argument |
| `--text`           | Convert stdin lines to stdout                |
| `--config`         | Config file path                             |

Exclude patterns use `path.Match`, anchored as in `.gitignore`. A leading or
inner slash matches the path relative to the target (`/*.txt`, `docs/old`);
otherwise the base name at any depth. A trailing slash matches directories only
(`node_modules/`). `*` stays within one path segment; `**` is an error, and `!`
negates nothing.

```sh
casemv snake ./docs -r -D             # whole tree, directories too
casemv pascal ./src -n -e '*.min.js'  # preview, skip minified files
casemv kebab . -x                     # My File.TXT → my-file.txt
echo 'My File' | casemv kebab --text  # my-file
```

## Behaviour

- **Extensions are kept verbatim**: `My File.TXT` → `my-file.TXT`, or
  `my-file.txt` with `--lower-ext`. Directories have no extension:
  `1. Something` → `1-something`.
- **Hidden stays hidden.** Names with no word characters (`___.txt`) stay as
  they are. Invalid UTF-8 bytes are kept.
- **Nothing is lost.**
  - Collisions abort before any rename. This covers unchanged entries too, and
    case-folding filesystems, compared the way the kernel compares them.
  - Each rename stays inside the target. On Linux it uses `RENAME_NOREPLACE`
    and refuses symlinks on the way. Elsewhere casemv checks, then renames, so
    a change racing between the two can still replace an entry or redirect
    through a symlink.
- **Renames run deepest-first**, so a directory is renamed after its contents.
- **SIGINT/SIGTERM** stops between renames and exits 130.
- **Progress** prints each rename after it succeeds. Names that hold control
  characters or invalid UTF-8 print quoted.

## Configuration

Order: flags, then `CASEMV_<FLAG>` environment variables (e.g.
`CASEMV_DRY_RUN`), then the config file: `--config`, else `CASEMV_CONFIG`, else
`$XDG_CONFIG_HOME/casemv/casemv.yaml`. The working directory is never searched.
Every flag except `--text` has a `CASEMV_<FLAG>` variable, and every one except
`--text` and `--config` has a config key: `--text` is command line only.
Unknown keys and mistyped values are errors. `CASEMV_EXCLUDE` separates
patterns like `PATH`: with `:`, or `;` on Windows.

```yaml
convention: kebab # then a bare `casemv` or `casemv ./docs` converts to kebab
lower-ext: true
recursive: true
exclude: ["*.min.js", node_modules]
```

A convention given as an argument overrides the default. A lone argument naming
a convention is the convention, anything else is the target, so a directory
named like a convention needs a path: `casemv ./kebab`.

## Shell completion

```sh
casemv completion bash > ~/.local/share/bash-completion/completions/casemv  # needs bash-completion
casemv completion zsh > "${fpath[1]}/_casemv"
casemv completion fish > ~/.config/fish/completions/casemv.fish
```

The first argument completes conventions, plus directories when a default
convention is set. Fish gets casemv's own script, which filters directories and
`--config` files the way bash and zsh do. A directory named `completion` needs a
path: `casemv ./completion`.

## Development

[mise](https://mise.jdx.dev) pins tools and runs tasks; [hk](https://hk.jdx.dev)
runs git hooks.

```sh
mise trust && mise run setup  # tools, Go modules, hooks
mise tasks                    # list all tasks
```

| Task                                         | Runs                                            |
| -------------------------------------------- | ----------------------------------------------- |
| `check` / `fix`                              | All hk checks / fixes                           |
| `test [-- flags]`, `test-race`               | Unit and CLI tests                              |
| `test-e2e`                                   | Built binary against real trees                 |
| `test-docker`                                | All suites in a no-network container            |
| `vet-cross`                                  | `go vet` for darwin and windows                 |
| `ci`                                         | `check`, `vet-cross`, `vuln`, every test suite  |
| `fuzz [target] [duration]`, `bench`, `cover` | Fuzzing, benchmarks, coverage                   |
| `vuln`, `build`, `install`                   | govulncheck, `bin/casemv`, `~/.local/bin`       |

- **Hooks:** `pre-commit` applies fixes; `pre-push` runs `test-race`.
- **CI:** runs `check`, `vet-cross`, `vuln`, `test-race`, and `test-e2e` on
  Linux, and the unit and CLI tests on macOS and Windows.
- **Lint:** `.golangci.yml` enables all linters; each disabled one has a
  reason. `depguard` limits imports to its allowlist.
- **Case-folding tests:** these run on a `casefold` tmpfs inside `unshare -rm`,
  which needs Linux 6.13+. Elsewhere they skip.

## License

Copyright (C) 2026 Ruandré Janse van Rensburg. Port of stdrename, copyright
Gabriel Lacroix.

[GPL-3.0-or-later](LICENSE), the same as stdrename.
