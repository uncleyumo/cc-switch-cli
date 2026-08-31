# cc-switch-cli

[English](README.md) | [中文](README_zh.md)

A library of Claude Code provider configs, merged into `~/.claude/settings.json`
with one command. Single Go binary; rename it to a short name such as **`ccsc`**
(or **`ccs`**) on your `PATH`.

```bash
ccsc use packy-bridge   # switch the global provider (field-merge, atomic, auto-backup)
ccsc current            # → packy-bridge
```

Requires Go 1.24.5+ to build from source.

## Install

### Prebuilt binaries (recommended)

Download the asset for your platform from the
[[GitHub Releases](https://github.com/<owner>/cc-switch-cli/releases)](https://github.com/uncleyumo/cc-switch-cli/releases) page
(or your own mirror), then put it on `PATH` and rename if you want a short name:

| Platform | Asset |
|---|---|
| Windows x86_64 | `cc-switch-cli_<version>_windows_amd64.exe` |
| Windows ARM64 | `cc-switch-cli_<version>_windows_arm64.exe` |
| Linux x86_64 | `cc-switch-cli_<version>_linux_amd64` |
| Linux ARM64 | `cc-switch-cli_<version>_linux_arm64` |
| macOS Intel | `cc-switch-cli_<version>_darwin_amd64` |
| macOS Apple Silicon | `cc-switch-cli_<version>_darwin_arm64` |

```bash
# Linux / macOS example
chmod +x cc-switch-cli_*_linux_amd64
mv cc-switch-cli_*_linux_amd64 ~/.local/bin/ccsc

# Windows (PowerShell) example
# Move-Item cc-switch-cli_*_windows_amd64.exe $env:USERPROFILE\bin\ccsc.exe
```

### Build from source

```bash
git clone <repo> && cd cc-switch-cli

# Windows: multi-platform release artifacts under dist/
build.bat
# optional: set VERSION=2.0.1 && build.bat

# any OS — local binary only
go build -ldflags="-s -w" -o ccsc .
```

Then initialize the config directory:

```bash
ccsc init               # create ~/.cc-switch-cli/ + config.toml
```

Each file in `~/.cc-switch-cli/models/` is one complete Claude Code `settings.json`:

```json
{
  "env": {
    "ANTHROPIC_AUTH_TOKEN": "your-api-key",
    "ANTHROPIC_BASE_URL": "https://api.anthropic.com",
    "ANTHROPIC_MODEL": "opus",
    "CLAUDE_CODE_MAX_CONTEXT_TOKENS": "500000"
  }
}
```

Bundled templates live in the repo under `models/` (`claude`, `deepseek`, `mimo`) —
copy or adapt them after `init`.

Switching merges the provider into the live `settings.json`: top-level keys the
provider declares override the file; keys it does not declare (hooks,
permissions, ...) are preserved, so settings written by Claude Code or plugins
survive switches. `env` is merged per key, and stale `ANTHROPIC_*` /
`CLAUDE_CODE_*` keys the new provider does not set are dropped.

## Commands

```
ccsc (cc-switch-cli)
├─ init                          create ~/.cc-switch-cli + config.toml
├─ use [model]                   activate globally; no arg → TUI picker
├─ current                       print the active model
├─ local [model]                 write ./.claude/settings.local.json (deep-merge)
│    --overwrite                 copy the provider file verbatim instead
│    --path <dir|file>           target another dir / explicit file
│  ├─ show [--reveal]            print project-level model + MCP
│  └─ clear [--model]            delete the file (or strip only model/env keys)
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
├─ mcp                           MCP manager TUI (no subcommand)
│  ├─ list, ls [--scope S]       list servers (user / local / project)
│  ├─ show <name> [--scope S] [--reveal]
│  ├─ add <name> [--scope local] [-t stdio|http|sse|ws] [-e K=V] [-H "K: V"] -- <cmd|url>
│  ├─ add-json <name> <json> [--scope S]
│  ├─ remove, rm <name> [--scope S] [-y]
│  ├─ test [name...]             initialize + tools/list probe
│  │   --scope S  --timeout 30s  --concurrency 4
│  ├─ tools <name> [--scope S]   list tools exposed by a server
│  └─ tui                        explicit manager (same as bare `mcp`)
├─ export                        encrypted export of models + user-scope MCPs
│    -o FILE  --models all|none|a,b  --mcps all|none|x,y  --secret-file F
├─ import <file>                 decrypt and apply an export
│    --secret-file F  --force
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
ccsc local show                        # project-level model + MCP
ccsc local clear                       # drop the project-level override
ccsc model show my-proxy --reveal # full JSON with secrets

ccsc mcp list                     # show MCP servers (all scopes)
ccsc mcp test fetch-mcp           # probe one server
ccsc mcp                          # interactive MCP manager
ccsc export --models all --mcps all --secret-file secret.txt -o backup.json
ccsc import backup.json --secret-file secret.txt
```

## `ccsc local` — merge by default

`ccsc local <name>` deep-merges into `./.claude/settings.local.json`: the
provider's `env` keys and top-level `model` override same-name values, and
**everything else is preserved** (permissions, hooks, MCP config, unknown keys).

- `--overwrite` — copy the provider file verbatim (drops project-specific keys).
- `--path <dir>` / `--path <file>` — write / inspect / clear elsewhere than the current directory.
- `ccsc local show` — summarize the project-local model override plus project-scope (`.mcp.json`) and local-scope (`~/.claude.json` → this project) MCP servers. Tokens are masked unless `--reveal`.
- `ccsc local clear` — delete `settings.local.json` so the project falls back to the global provider. `ccsc local clear --model` strips only the provider `env` / top-level `model` keys and keeps permissions, hooks and custom env. A leftover-empty file is removed. MCP files are never touched.

If a provider is literally named `clear` or `show`, write it with `ccsc local -- <name>`.

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

## Context window

The **context window** field on a provider is the group default. Type it as
`128k` / `256k` / `500k` / `1m` / `2m` — the same spelling is shown when you
reopen the editor. Bare numbers (`500`, `500000`) are rejected. On disk the
value is stored as a plain integer for Claude Code
(`CLAUDE_CODE_MAX_CONTEXT_TOKENS`). `CLAUDE_CODE_AUTO_COMPACT_WINDOW` is
written only when the size is at most 1M and no id in the group has a `[1m]`
suffix.

On an unrecognized / gateway id, Claude Code treats a trailing `[1m]` as a 1M
window and then ignores `CLAUDE_CODE_MAX_CONTEXT_TOKENS` for that id. Other
suffixes such as `[200k]` are stripped on the wire but do not change the
assumed window. Typical layout: group `500k`, plus `[1m]` on the one id that
is actually 1M.

`ccsc local` drops the three window keys when the incoming provider does not
set them. `ccsc model doctor` prints the resolved id, the on-the-wire id, and
the group window.
## `ccsc mcp` — MCP servers

Manages the MCP servers Claude Code loads from:

| Scope | Storage |
|---|---|
| **user** | `~/.claude.json` top-level `mcpServers` |
| **local** (default for `add`) | `~/.claude.json` → `projects.<cwd>.mcpServers` |
| **project** | `<cwd>/.mcp.json` |

`ccsc mcp test` runs a real initialize handshake and `tools/list` over stdio /
Streamable HTTP (with legacy SSE fallback). Results cache in
`~/.cc-switch-cli/mcp-test-status.json`. WebSocket is stored but not probed.
Sensitive values (tokens, DSN passwords) are masked unless `--reveal`.

Edits write `~/.claude.json` directly (with a timestamped backup under
`~/.cc-switch-cli/backups/`). Prefer editing while Claude Code is closed —
a running session may rewrite the file on exit.

## `ccsc export` / `ccsc import`

Moves **global** model library files and **user-scope** MCP servers between
machines. The file is an authenticated AES-256-GCM envelope (scrypt-derived key);
format version is checked before the secret is requested so incompatible
exports fail early. Selection is interactive in a TTY, or via
`--models` / `--mcps` (`all` / `none` / comma list). Existing entries are skipped
on import unless `--force`.

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

MCP manager (bare `ccsc mcp`):

| Key | Action |
|---|---|
| `enter` `l` | list tools of selected server |
| `a` / `e` / `n` / `d` | add / edit / rename / delete |
| `v` `space` | view JSON (masked) |
| `t` / `T` | probe selected / all visible |
| `s` | cycle scope filter (all/user/local/project) |
| `r` | reload |
| `q` `esc` | quit |

Editor (`ccsc model edit <name>` or MCP `e`): `tab`/`shift+tab` move fields,
`ctrl+j` toggle raw JSON, `ctrl+s` save, `esc` cancel.

Non-TTY invocations (`use`, `local`, `edit`, `model`, `mcp`) require an explicit
name or subcommand; the tool refuses to open the TUI outside a terminal so
scripts fail loudly.

## Files

```
~/.cc-switch-cli/
├─ config.toml                  target settings.json path
├─ models/*.json                provider library
├─ test-status.json             model probe cache
├─ mcp-test-status.json         MCP probe cache
├─ backups/claude-json-*.json   pre-mutation snapshots of ~/.claude.json
├─ <provider>.current           marker for the active provider
├─ default*.json / backup-*.json  first-switch + manual backups
├─ model-switch.logs            append-only switch audit log
└─ test-status.json             last availability check per provider
```

`ANTHROPIC_AUTH_TOKEN` / `ANTHROPIC_API_KEY` are masked in `model show` and TUI
previews unless `--reveal`; the status cache never stores tokens or request bodies.

## Build matrix

`build.bat` cross-compiles six static binaries into `dist/`:

```
cc-switch-cli_<version>_windows_amd64.exe
cc-switch-cli_<version>_windows_arm64.exe
cc-switch-cli_<version>_linux_amd64
cc-switch-cli_<version>_linux_arm64
cc-switch-cli_<version>_darwin_amd64
cc-switch-cli_<version>_darwin_arm64
```

Naming follows the common Go release convention: `name_version_os_arch`.
`darwin` is macOS (Go toolchain name for the Darwin kernel).

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
