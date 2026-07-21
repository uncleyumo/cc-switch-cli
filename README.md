# cc-switch-cli

Manage a library of Claude Code model provider configs and swap them into
`~/.claude/settings.json` on demand. Ships as a single Go binary with a short
alias `ccs`.

- **Fast switches** — one command replaces the active provider atomically.
- **Project-scoped configs** — `ccs local <model>` writes to
  `.claude/settings.local.json` in the current project without touching
  anything else in it (permissions, hooks, MCP config are preserved).
- **Interactive TUI** — bare `ccs model` opens a Bubble Tea manager to
  add / edit / test / delete models from one screen.
- **Provider availability check** — `ccs model test` streams a real Anthropic
  Messages request against each provider (streaming + tool call).

Requires Go 1.24.5+ to build.

---

## Install

```powershell
git clone <this repo>
cd cc-switch-cli
build.bat        # produces cc-switch-cli.exe and ccs.exe
```

Non-Windows builds:

```bash
go build -ldflags="-s -w" -o ccs .
```

Add the binary to `PATH`. First-time setup:

```bash
ccs init
```

This creates `~/.cc-switch-cli/`, the `models/` subdirectory and a default
`config.toml` pointing at `~/.claude/settings.json`.

Drop provider JSON files into `~/.cc-switch-cli/models/`. Each file is a
complete Claude Code `settings.json`. Example:

```json
{
  "env": {
    "ANTHROPIC_AUTH_TOKEN": "your-api-key",
    "ANTHROPIC_BASE_URL": "https://api.anthropic.com",
    "ANTHROPIC_MODEL": "claude-sonnet-4-6",
    "ANTHROPIC_DEFAULT_HAIKU_MODEL": "claude-haiku-4-5-20251001",
    "ANTHROPIC_DEFAULT_OPUS_MODEL": "claude-opus-4-7",
    "ANTHROPIC_DEFAULT_SONNET_MODEL": "claude-sonnet-4-6"
  },
  "model": "sonnet"
}
```

---

## Command tree

```
ccs
├─ init                              create ~/.cc-switch-cli and config.toml
├─ use [model]                       activate globally; no arg → TUI picker
├─ current                           print the active model
├─ local [model]                     write .claude/settings.local.json (merge)
│    --overwrite                     replace the file wholesale
│    --path <dir|file>               target another directory or explicit file
├─ backup                            snapshot the current settings.json
├─ model, m                          interactive TUI (no subcommand)
│    ├─ list, ls                     list models with active + last test status
│    ├─ show <name> [--reveal]       print JSON; token masked unless --reveal
│    ├─ add <name> [--from x] [--no-edit]
│    ├─ edit [name]                  structured / raw JSON editor
│    ├─ rename <old> <new>
│    ├─ remove, rm <name> [-y]
│    ├─ test [name...]               run Anthropic streaming + tool checks
│    │   --stream / --tool           select one check
│    │   --timeout 30s               per-provider timeout
│    │   --concurrency 4             max models tested in parallel
│    │   --auth auto|bearer|x-api-key
│    ├─ doctor                       show resolved vs on-the-wire model id
│    └─ tui                          explicit TUI (same as bare `model`)
└─ config, cfg
     ├─ show                         resolved paths + current model
     └─ set-target <path>            change target settings.json path
```

`--version` / `-v` prints the build-stamped version.

---

## Typical workflows

Activate a provider globally:

```bash
ccs use packy-bridge     # or omit the arg in a terminal to open the picker
ccs current              # → packy-bridge
```

Scope a provider to a project:

```bash
cd my-project
ccs local packy-bridge   # merges into ./.claude/settings.local.json
```

Adding a new provider:

```bash
ccs model add my-proxy        # creates a template and opens the editor
ccs model edit my-proxy       # come back later to tweak
ccs model rename my-proxy new
```

Sanity-check every provider you have:

```bash
ccs model test               # tests all providers, streaming + tool
ccs model test my-proxy --stream --timeout 20s
```

Results are cached in `~/.cc-switch-cli/test-status.json` and shown after each
model name in `ccs model list`. Testing issues real Anthropic API calls
against each provider — expect a small token cost.

### Reading the results

Each check reports one of three statuses, and `model list` collapses a
provider's checks to a single verdict:

- **OK** — every requested check passed; the provider is fully usable.
- **OK~** — streaming works (the provider *is* usable for normal Claude Code
  chat) but the optional tool-call probe could not confirm a `tool_use` block
  this run. This is a caveat, not an outage: it covers gateways whose tool
  support is limited and transient tool-request hiccups alike.
- **FAIL** — the streaming check failed (bad token, no balance, unknown model,
  unreachable endpoint). The provider is not currently usable.

Availability is judged by **streaming**: a provider whose stream check passed is
never shown as FAIL just because a tool probe errored (the detailed table still
shows the per-check truth). The tool probe deliberately mirrors how Claude Code
calls tools — it supplies the tool list but does *not* send a forced
`tool_choice`, since directives like `"any"` are rejected by several
Anthropic-compatible gateways that relay `tool_use` perfectly under the default
mode. That mismatch was the original source of false "tool failed" verdicts.

### The `[1m]` context-window marker

A model id in your config may carry a trailing marker such as `[1m]` or
`[200k]`, e.g. `claude-opus-4-8[1m]`. This is **not** corruption — it is the
annotation Claude Code uses to select the larger context window (the same
marker you see on a running session's model id). Keep it in your config: Claude
Code, and `ccs model test`, strip it automatically *before* sending a request,
because proxies reject the marker if it leaks into the request body while they
accept the clean name. So a `[1m]` suffix in your files is expected and correct.

`ccs model doctor` prints, per provider, the resolved model id and the
on-the-wire id (with the marker removed) so you can confirm what is actually
sent. It is read-only and never rewrites your files; use `ccs model edit` to
change fields by hand.

The model id is otherwise resolved the way Claude Code resolves it: a tier alias
such as `opus`/`sonnet`/`haiku` in `ANTHROPIC_MODEL` (or the top-level `model`)
is expanded to the matching `ANTHROPIC_DEFAULT_*_MODEL`.

---

## `local` merge vs overwrite

By default `ccs local <name>` **deep-merges** into `.claude/settings.local.json`:

- Provider env keys override same-name env values in the file.
- Provider top-level `model` overrides the file's `model`.
- **Everything else in the file is preserved** — `permissions`, hooks, MCP
  configuration, unknown project keys.

Use `--overwrite` when you want the provider file copied verbatim into the
project (dropping any project-specific keys). Use `--path <dir>` or
`--path <file>` to write somewhere other than the current directory.

---

## TUI cheat sheet

Bare `ccs model` opens the manager. Keys:

| Key | Action |
|---|---|
| `↑` `↓` `j` `k` | Navigate |
| `/` | Search / filter |
| `enter` `u` | Activate globally |
| `a` | Add a new model |
| `n` | Rename the selected model |
| `e` | Edit the selected model |
| `d` | Delete (with confirmation) |
| `v` `space` | Preview JSON (token masked) |
| `t` | Test selected |
| `T` | Test all |
| `r` | Reload models & test status |
| `q` `esc` | Quit |

Editor (`ccs model edit <name>`):

| Key | Action |
|---|---|
| `tab` / `shift+tab` | Move between fields |
| `ctrl+j` | Toggle raw JSON edit mode |
| `ctrl+s` | Save |
| `esc` | Cancel / return from raw JSON to fields |

Non-terminal invocations (`ccs use`, `ccs local`, `ccs model edit`, `ccs model`)
require an explicit name; the tool refuses to open the TUI outside a TTY so
scripts fail loudly rather than hang.

---

## Files on disk

```
~/.cc-switch-cli/
├─ config.toml                    target settings.json path
├─ models/*.json                  provider library (one file per provider)
├─ default.json                   first-switch snapshot
├─ default-YYYYMMDD-HHMMSS.json   timestamped twin
├─ backup-YYYYMMDD-HHMMSS.json    manual backups from `ccs backup`
├─ <provider>.current             marker for the active provider
├─ model-switch.logs              append-only audit log of switches
└─ test-status.json               last availability check per provider
```

Sensitive values (`ANTHROPIC_AUTH_TOKEN`, `ANTHROPIC_API_KEY`) are masked in
`model show` and TUI previews. Pass `--reveal` on `model show` for raw JSON.
`test-status.json` never stores request bodies or tokens.

---

## Migration from v1

The command surface was reshaped in v2. Rough equivalents:

| v1 | v2 |
|---|---|
| `cc-switch-cli list` | `ccs model list` |
| `cc-switch-cli cat <name>` | `ccs model show <name>` |
| `cc-switch-cli model set <name>` | `ccs use <name>` |
| `cc-switch-cli model current` | `ccs current` |
| `cc-switch-cli model backup` | `ccs backup` |
| `cc-switch-cli local <name> [path] --force` | `ccs local <name> [--path <p>] [--overwrite]` |

`local` semantics changed: v2 defaults to `.claude/settings.local.json` inside
the project (v1 wrote to the current directory root) and merges by default.
Pass `--overwrite` if you want v1's byte-copy behaviour.
