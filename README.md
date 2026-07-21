# cc-switch-cli

A library of Claude Code provider configs, swapped into `~/.claude/settings.json`
with one command. Single Go binary, short alias **`ccsc`**.

```bash
ccsc use packy-bridge   # swap the global provider (atomic, auto-backup)
ccsc current            # → packy-bridge
```

Requires Go 1.24.5+ to build.

## Install

```bash
git clone <repo> && cd cc-switch-cli
build.bat              # Windows: cc-switch-cli.exe + ccsc.exe
# or, any OS:
go build -ldflags="-s -w" -o ccsc .
```

Put the binary on `PATH`, then:

```bash
ccsc init               # create ~/.cc-switch-cli/ + config.toml
```

Each file in `~/.cc-switch-cli/models/` is one complete Claude Code `settings.json`:

```json
{
  "env": {
    "ANTHROPIC_AUTH_TOKEN": "your-api-key",
    "ANTHROPIC_BASE_URL": "https://api.anthropic.com",
    "ANTHROPIC_MODEL": "claude-sonnet-4-6"
  },
  "model": "sonnet"
}
```

## Commands

```
ccsc (cc-switch-cli)
├─ init                          create ~/.cc-switch-cli + config.toml
├─ use [model]                   activate globally; no arg → TUI picker
├─ current                       print the active model
├─ local [model]                 write ./.claude/settings.local.json (deep-merge)
│    --overwrite                 copy the provider file verbatim instead
│    --path <dir|file>           target another dir / explicit file
├─ backup                        snapshot the current settings.json
├─ model, m                      manager TUI (no subcommand)
│  ├─ list, ls                   list models + active flag + last test status
│  ├─ show <name> [--reveal]     print JSON (token masked unless --reveal)
│  ├─ add <name> [--from x] [--no-edit]
│  ├─ edit [name]                structured / raw JSON editor
│  ├─ rename <old> <new>
│  ├─ remove, rm <name> [-y]
│  ├─ test [name...]             streaming + tool checks (see below)
│  │   --stream / --tool         run one check only
│  │   --timeout 30s  --concurrency 4  --auth auto|bearer|x-api-key
│  ├─ doctor                     show resolved vs on-the-wire model id (read-only)
│  └─ tui                        explicit manager (same as bare `model`)
└─ config, cfg
   ├─ show                       resolved paths + current model
   └─ set-target <path>          change the target settings.json
```

`--version` / `-v` prints the build-stamped version.

## Typical use

```bash
ccsc model add my-proxy           # template + open the editor
ccsc model test                   # availability-check every provider
ccsc use my-proxy                 # activate globally
cd my-project && ccsc local my-proxy   # scope it to one project
ccsc model show my-proxy --reveal # full JSON with secrets
```

## `ccsc local` — merge by default

`ccsc local <name>` deep-merges into `./.claude/settings.local.json`: the
provider's `env` keys and top-level `model` override same-name values, and
**everything else is preserved** (permissions, hooks, MCP config, unknown keys).

- `--overwrite` — copy the provider file verbatim (drops project-specific keys).
- `--path <dir>` / `--path <file>` — write elsewhere than the current directory.

## `ccsc model test` — availability

Runs real Anthropic Messages calls (streaming + a tool-call probe), so expect a
small token cost. Results are cached in `~/.cc-switch-cli/test-status.json` and
shown per model in `ccsc model list`.

| Status | Meaning |
|---|---|
| **OK** | All requested checks passed — fully usable. |
| **OK~** | Streaming works (usable for chat) but the tool probe didn't confirm `tool_use` this run — a caveat, not an outage. |
| **FAIL** | Streaming failed (bad token / no balance / unknown model / unreachable) — not usable. |

Streaming is the source of truth: a provider whose stream check passed is never
FAIL, even if the tool probe errored transiently. The tool probe mirrors Claude
Code — it sends the tool list but omits `tool_choice`, since forced directives
like `"any"` are rejected by several gateways that relay tools fine.

## The `[1m]` / `[200k]` context marker

A trailing marker like `claude-opus-4-8[1m]` is **not** corruption — it selects
the larger context window. Keep it in your config; Claude Code and `ccsc model
test` strip it only when sending the request, because proxies reject it in the
body. `ccsc model doctor` prints, per provider, the resolved id and the
on-the-wire id (marker removed). It is read-only — use `ccsc model edit` to change
fields. Tier aliases (`opus`/`sonnet`/`haiku`) in `ANTHROPIC_MODEL` or `model`
expand to the matching `ANTHROPIC_DEFAULT_*_MODEL`.

## TUI keys

Manager (bare `ccsc model`):

| Key | Action |
|---|---|
| `↑↓` `j` `k` | navigate |
| `/` | search / filter |
| `enter` `u` | activate globally |
| `a` / `n` / `e` / `d` | add / rename / edit / delete |
| `v` `space` | preview JSON (masked) |
| `t` / `T` | test selected / test all |
| `r` | reload models & status |
| `q` `esc` | quit |

Editor (`ccsc model edit <name>`): `tab`/`shift+tab` move fields, `ctrl+j`
toggle raw JSON, `ctrl+s` save, `esc` cancel.

Non-TTY invocations (`use`, `local`, `edit`, `model`) require an explicit name;
the tool refuses to open the TUI outside a terminal so scripts fail loudly.

## Files

```
~/.cc-switch-cli/
├─ config.toml                  target settings.json path
├─ models/*.json                provider library
├─ <provider>.current           marker for the active provider
├─ default*.json / backup-*.json  first-switch + manual backups
├─ model-switch.logs            append-only switch audit log
└─ test-status.json             last availability check per provider
```

`ANTHROPIC_AUTH_TOKEN` / `ANTHROPIC_API_KEY` are masked in `model show` and TUI
previews unless `--reveal`; the status cache never stores tokens or request bodies.

## v1 → v2

| v1 | v2 |
|---|---|
| `cc-switch-cli list` | `ccsc model list` |
| `cc-switch-cli cat <name>` | `ccsc model show <name>` |
| `cc-switch-cli model set <name>` | `ccsc use <name>` |
| `cc-switch-cli model current` | `ccsc current` |
| `cc-switch-cli model backup` | `ccsc backup` |
| `cc-switch-cli local <name> [path] --force` | `ccsc local <name> [--path <p>] [--overwrite]` |

`local` now defaults to `.claude/settings.local.json` and merges (v1 wrote to the
directory root and byte-copied). Pass `--overwrite` for v1 behaviour.
