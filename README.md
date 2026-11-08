# ccsc

[English](README.md) | [中文](README_zh.md)

Model configuration for Claude Code, as a library of groups you can apply
globally or per project. One Go binary; `ccsc` is the only name.

```bash
ccsc group      # build a group once
ccsc status     # see what is actually in effect
```

Requires Go 1.24.5+ to build from source.

## Install

### Prebuilt binaries

Grab the archive for your platform from
[GitHub Releases](https://github.com/uncleyumo/cc-switch-cli/releases), unpack
it and put `ccsc` on your `PATH`:

| Platform | Asset |
|---|---|
| Windows x86_64 | `ccsc_windows_amd64.zip` |
| Windows ARM64 | `ccsc_windows_arm64.zip` |
| Linux x86_64 | `ccsc_linux_amd64.zip` |
| Linux ARM64 | `ccsc_linux_arm64.zip` |
| macOS Intel | `ccsc_darwin_amd64.zip` |
| macOS Apple Silicon | `ccsc_darwin_arm64.zip` |

### Build from source

```bash
git clone https://github.com/uncleyumo/cc-switch-cli && cd cc-switch-cli

build.bat                     # Windows: six targets into dist/
go build -o ccsc .            # any OS: local binary only
```

### Self-update

```bash
ccsc upgrade --check   # is there a newer release?
ccsc upgrade           # download, verify sha256, replace, self-check
```

The running binary is renamed to `ccsc.exe.bak.YYYYMMDD` before the new one is
written. If the new binary fails `--version` it is rolled back automatically.

## Concepts

Only two words matter.

**Group** — one endpoint plus five model slots: `fable`, `opus`, `sonnet`,
`haiku`, `subagent`. A group carries its own `base_url`, so multiple gateways
and multiple endpoints are supported simply by having multiple groups.

**Optional** — the other way into a *project-level* configuration: no group
library, connect to a new-api endpoint now, pick models from the list it
returns, write the result into the project. It stays in effect until
`ccsc local` or `ccsc local clear` overrides it.

A session can only talk to one endpoint at a time. That is a Claude Code
constraint (there is one `ANTHROPIC_BASE_URL`), not a ccsc one.

## Commands

```
ccsc                          workbench, opens on the group list
├─ group                      groups: new / edit / apply globally / delete
├─ local                      bind a group to this project
│  └─ clear                   remove every model key ccsc wrote here
├─ optional                   connect now, fetch models, write to this project
├─ status                     what is in effect (read-only)
│    --reveal                 show the full token
│    --dir <path>             look at another project directory
├─ config                     settings panel
├─ mcp                        servers across user / local / project (read-only)
│  ├─ test [name]             real initialize + tools/list handshake
│  --json
├─ skills                     skills across user / project (read-only)
├─ upgrade [--check]          self-update from GitHub Releases
├─ version
└─ completion <shell>
```

`group`, `local`, `optional` and `config` are TUI-only. They refuse to start
outside a terminal instead of hanging on a pipe. The read-only commands are
plain text and pipe cleanly.

## What ccsc writes

ccsc owns a fixed set of keys and touches nothing else. `hooks`, `permissions`,
`enabledPlugins`, `effortLevel`, `theme`, unknown keys and other
`modelSettings` fields survive byte for byte.

| Key | Global | Project |
|---|:--:|:--:|
| `env.ANTHROPIC_BASE_URL` | ✓ | ✓ |
| `env.ANTHROPIC_AUTH_TOKEN` | ✓ | ✓ |
| `env.ANTHROPIC_DEFAULT_{FABLE,OPUS,SONNET,HAIKU}_MODEL` | ✓ | ✓ |
| `env.ANTHROPIC_DEFAULT_*_MODEL_NAME` | ✓ | ✓ |
| `env.ANTHROPIC_DEFAULT_MODEL` (startup fallback) | ✓ | ✓ |
| `env.CLAUDE_CODE_SUBAGENT_MODEL` | ✓ | ✓ |
| `env.CLAUDE_CODE_MAX_CONTEXT_TOKENS` | ✓ | ✓ |
| `env.CLAUDE_CODE_AUTO_COMPACT_WINDOW` | cleared | cleared |
| `env.CLAUDE_CODE_AUTO_COMPACT_WINDOW_TOKENS` | cleared | cleared |
| `modelSettings.<id>.autoCompactWindow` | ✓ | — |
| top-level `autoCompactWindow` | — | ✓ |

Writes are read-modify-write on an order-preserving JSON tree: key order and
indent come back exactly as they went in, so applying a group produces a diff of
eight lines instead of a rewritten file.

### Project layer

**The project root is the directory ccsc was run in.** Claude Code does not walk
upward looking for `.claude/`, so neither does ccsc. `--dir` replaces the root;
it never triggers a search.

The target is `<cwd>/.claude/settings.local.json`. If that file has no sibling
`.claude/settings.json` but an ancestor directory does, ccsc warns — Claude Code
started in `<cwd>` will not read the ancestor's file, which is the quietest way
to end up with a configuration that looks applied and is never used.

### Startup model

Four layers, highest first:

| # | Source | Written by |
|---|---|---|
| 1 | `ANTHROPIC_MODEL` (env) | you or an old config; ccsc only reports it |
| 2 | `/model` choice → top-level `model` | Claude Code itself |
| 3 | `ANTHROPIC_DEFAULT_MODEL` | ccsc |
| 4 | Claude Code's built-in default | — |

ccsc writes layer 3 and nothing else, so it never fights `/model`. The value is
always a resolved model id, because Claude Code ignores an alias there.

### Context window and auto-compact

Two different things:

- **Hard cap** (`CLAUDE_CODE_MAX_CONTEXT_TOKENS`) — how big the model is. It
  drives `/status` and the default compaction point.
- **`autoCompactWindow`** — when compaction actually starts. It is capped by
  the hard cap, so a larger number is the same as no number.

Defaults: per-slot compact = **90 % of the effective window** (1M if the slot is
marked 1M, otherwise the group's hard cap). Set it explicitly per slot to
override.

The two layers store it differently on purpose:

- **Global** → `modelSettings.<id>.autoCompactWindow`, per model, merged into the
  entry that already exists so your `effortLevel` survives.
- **Project** → one top-level `autoCompactWindow`. Whether Claude Code merges
  same-model entries across settings files field by field is undocumented, and a
  wrong guess would wipe `effortLevel`. When slots disagree, the smallest value
  wins: compacting early never overflows.

`CLAUDE_CODE_AUTO_COMPACT_WINDOW` / `_TOKENS` are always cleared on apply —
they outrank every per-model value and would turn them into dead configuration.

## Group file

`~/.cc-switch-cli/groups/<name>.json`, mode `0600`:

```jsonc
{
  "schema": 1,
  "base_url": "https://new-api.example.com",
  "token": "sk-...",
  "window": 500000,             // group-wide hard cap, 0 = don't write
  "slots": {
    "fable":    { "model": "packy-official/deepseek-flash", "name": "DeepSeek" },
    "opus":     { "model": "packy-cc/claude-opus-5-5", "window": 1000000 },
    "sonnet":   { "model": "packy-sale/deepseek-flash" },
    "haiku":    { "model": "zen/glm-5.3-flash-free", "compact": 150000 },
    "subagent": { "model": "zen/glm-5.3-flash-free" }
  }
}
```

A slot's `window` is either `0` or `1000000` — `[1m]` is the only per-model
window Claude Code recognises. Anything else belongs in the group-level
`window`. The four family slots are required; `subagent` is optional and, when
empty, `CLAUDE_CODE_SUBAGENT_MODEL` is not written at all.

## `optional`

```
ccsc optional
```

One panel, no sub-screens:

1. Fill in `base_url` and the token.
2. `ctrl+f` calls `/v1/models` and loads the list into the model rows.
3. Type to filter a row, `enter` (or `→`) accepts the highlighted id and moves
   on. Any id can be typed by hand — new-api translates protocols, so the list
   is shown in full and never filtered by `supported_endpoint_types`.

Leaving the `subagent` row empty means it is not configured. Leaving any of the
four family rows empty is an error.

The result is written into the project and stays in effect until `ccsc local`
or `ccsc local clear` overrides it.

## `status`, `mcp`, `skills`

`ccsc status` resolves every model key across the user, project and local
layers, shows where the winning value came from, then the startup slot, the
subagent model, the compaction point, and MCP/skills counts. Tokens are masked;
`--reveal` prints one with a banner.

`ccsc mcp` reads all three scopes and marks losers:

| Scope | Storage | Precedence |
|---|---|---|
| user | `<CLAUDE_CONFIG_DIR or ~>/.claude.json` → `mcpServers` | lowest |
| local | the same file → `projects["<abs cwd>"].mcpServers` | highest |
| project | `<cwd>/.mcp.json` → `mcpServers` | middle |

`settings.json` files hold no server definitions — only approval keys, which is
what the dashboard reads to label a `.mcp.json` server approved or pending.

`ccsc mcp test [name]` spawns the server, performs `initialize` and
`tools/list`, and prints tool count and latency. It writes nothing.

`ccsc skills` merges `~/.claude/skills`, `.claude/skills`, and the `commands`
directories of both, project over user.

## Keys

Lists: `↑↓` / `j` `k` / `←→` move, `enter` select, `q` quit. The group list
also binds `n` new, `u` apply globally, `d` delete.

Panels (one screen per configuration, no nested windows):

| Key | Action |
|---|---|
| `↑↓` `j` `k` `tab` `shift+tab` | move between rows |
| `←` / `→` | previous / next row |
| `enter` | accept the highlighted suggestion, then next row |
| `ctrl+f` | fetch the model list from the endpoint |
| `v` | reveal a secret row for 10 seconds |
| `ctrl+u` | clear the current row |
| `ctrl+s` | save (refused while a row is invalid) |
| `esc` | cancel |

Typing in a model row filters its suggestions in place. Typing in a secret row
replaces the stored value.

Context windows accept `500k`, `567k`, `1m`, `2m` or a bare number, and are
shown back in the same short form.

## Output

Everything ccsc prints is plain English ASCII: no ticks, arrows, box drawing or
emoji. It stays readable in any terminal, in a pipe, and in a log.

## Files

```
~/.cc-switch-cli/
├─ config.toml                 defaults: base_url, token, startup slot, window
├─ groups/<name>.json          the group library (plaintext token, 0600)
├─ cache/models/*.json         model lists, keyed by host + token hash
├─ state.json                  what ccsc applied where, and which models it owns
├─ backups/<target>/*.json     ten most recent snapshots of each file it wrote
└─ audit.jsonl                 append-only history, 5 MB × 3
```

Deleting this directory and the binary leaves no trace.

Set `CCSC_HOME` to move the whole thing.

## Tokens

Stored in plaintext at mode `0600`. This is masked *display*, not encryption:
`sk-tes******mnop` everywhere. Inside a panel, `v` reveals the value for ten
seconds; `ccsc status --reveal` prints it once, with a warning first.

Only one credential concept exists. ccsc writes `ANTHROPIC_AUTH_TOKEN`, which
gateways accept as `Authorization: Bearer`. `ANTHROPIC_API_KEY` (the `x-api-key`
header) is not exposed as a choice; if the target file already has one with the
same value, it is removed as redundant and the receipt says so.

## v1 → v2

v1 provider files are migrated once, on the first TUI command:

```bash
ccsc group     # migrates, then opens the library
```

Each `~/.cc-switch-cli/models/*.json` becomes a group. The originals are moved
to `~/.cc-switch-cli/backups/legacy-models-<date>/`, a `.migrated-v2` marker
makes it idempotent, and groups that already exist are never overwritten.

| v1 | v2 |
|---|---|
| `ccsc use <name>` | `ccsc group` → `u` |
| `ccsc model *` | `ccsc group` |
| `ccsc local [name]` | `ccsc local` → `enter` |
| `ccsc local clear --model` | `ccsc local clear` |
| `ccsc mcp add/remove/show/tools` | removed — MCP is read-only now |
| `ccsc export` / `ccsc import` | removed |
| `ccsc backup` / `ccsc current` / `ccsc init` | removed; state is implicit |
| `ccsc model test` | removed — use `ccsc status` to see the effective config |

## Not in scope

- No proxy or model router: one session, one endpoint.
- No MCP or skills editing. Read-only dashboards, plus a connectivity probe.
- No `modelPicker`.
- No `--no-tui` fallback. No terminal, no TUI: the command exits.
- Automatic model-list fetching is **off** in `group` and `local`. It only
  exists in `optional`, where the whole point is talking to a new-api endpoint.

## Build matrix

`build.bat` cross-compiles six static binaries into `dist/`
(`ccsc_<version>_<os>_<arch>`). `.goreleaser.yml` produces the release assets
`ccsc_<os>_<arch>.zip` plus `checksums.txt` — the names `ccsc upgrade` expects.
