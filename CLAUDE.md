# Project constraints

## Never run `ccsc` against this repo

This working tree is a real Claude Code project. Its `.claude/settings.local.json`,
`.mcp.json`, and the matching `projects.<cwd>` slice of `~/.claude.json` are
live config, not fixtures.

**Do not** invoke a built `ccsc` / `cc-switch-cli` binary (or `go run .`)
from this repository's working directory to try out, smoke-test, or demonstrate
any command that reads or writes Claude Code / cc-switch-cli state. That includes
at least:

- `local`, `local show`, `local clear`
- `use`, `current`, `backup`
- `mcp …` (list / add / remove / test)
- `import` / `export`
- `model test` / `mcp test` (they also touch status caches under `~/.cc-switch-cli`)

A previous session deleted this repo's real `settings.local.json` by running
`./ccsc.exe local clear` here. Do not repeat that.

### How to test instead

- Prefer `go test` with `t.TempDir()`. Isolate `HOME` / `USERPROFILE` /
  `CLAUDE_CONFIG_DIR` when the code under test resolves user-level paths.
- If a binary smoke test is required: `t.Chdir` / `Set-Location` into a temp
  directory first, pass `--path` at that temp dir, and point home-related env
  vars at another temp dir. Never use this repo as the implicit cwd target.
- After a binary smoke test, delete leftover artifacts (`ccsc.exe` / `ccsc`,
  `cc-switch-cli`, stray settings / MCP files). `t.TempDir()` already cleans
  itself; a manually created temp dir must be removed.

`go test ./...` from the repo is fine. Building a binary to inspect `--help` is
fine. Mutating or even casually executing stateful `ccsc` commands against this
checkout is not.
