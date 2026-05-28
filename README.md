# cc-switch-cli

A CLI tool to switch Claude Code model provider configuration without affecting other settings (plugins, permissions, MCP servers, etc.).

## Installation

```bash
go build -o cc-switch-cli.exe .
```

Or use the build script:

```bash
build.bat
```

## Quick Start

```bash
# 1. Initialize configuration
cc-switch-cli init

# 2. Add your model provider JSON files to ~/.cc-switch-cli/models/
#    (sample files are included in the models/ directory)

# 3. List available providers
cc-switch-cli list

# 4. View a provider's config
cc-switch-cli cat deepseek

# 5. Switch to a provider
cc-switch-cli model set deepseek

# 6. Check current provider
cc-switch-cli model current

# 7. Create a local settings file for a project
cc-switch-cli local deepseek .
```

## Viewing Provider Config

Use `cat` to quickly inspect a provider's configuration without navigating to the models directory:

```bash
# Print deepseek provider config
cc-switch-cli cat deepseek

# Print mimo provider config
cc-switch-cli cat mimo
```

## Creating Local Project Settings

Use `local` to create a `settings.local.json` file for project-level Claude Code configuration:

```bash
# Create settings.local.json in current directory
cc-switch-cli local deepseek .

# Create in a specific directory
cc-switch-cli local deepseek ./.claude/

# Create with a custom filename
cc-switch-cli local deepseek ./.claude/settings.json.bk

# Default: creates settings.local.json in current directory
cc-switch-cli local deepseek

# Overwrite existing file
cc-switch-cli local deepseek . --force
```

## Commands

| Command | Description |
|---------|-------------|
| `cc-switch-cli init` | First-time setup: creates config directory and files |
| `cc-switch-cli list` | List available model providers |
| `cc-switch-cli cat <provider>` | Print provider configuration content |
| `cc-switch-cli local <provider> [path]` | Create a local settings file from provider config |
| `cc-switch-cli model set <provider>` | Switch to a model provider |
| `cc-switch-cli model current` | Show active model provider |
| `cc-switch-cli model backup` | Create manual backup of current settings |

## How It Works

1. Each model provider is a complete `settings.json` stored in `~/.cc-switch-cli/models/`
2. When you switch, the tool replaces `~/.claude/settings.json` with the provider's config
3. On first switch, the original settings are backed up as `default.json`
4. A `.current` marker file tracks which provider is active
5. All switches are logged in `model-switch.logs`

## File Structure

```
~/.cc-switch-cli/
  config.toml              # Target file path configuration
  models/                  # Model provider JSON files
    xiaomi-mimo.json
    deepseek.json
    claude.json
    gpt.json
  *.current                # Active provider marker (e.g., xiaomi-mimo.current)
  default.json             # Auto-backup of original settings
  default-YYYYMMDD-HHMMSS.json  # Timestamped backup
  model-switch.logs        # Switch history log
```

## Adding a New Provider

1. Copy an existing JSON from `~/.cc-switch-cli/models/`
2. Edit the `env` section with your API credentials and model names
3. Save as `<provider-name>.json`
4. Run `cc-switch-cli list` to verify it appears

Example provider JSON:

```json
{
  "env": {
    "ANTHROPIC_AUTH_TOKEN": "your-api-key",
    "ANTHROPIC_BASE_URL": "https://api.example.com/anthropic",
    "ANTHROPIC_DEFAULT_HAIKU_MODEL": "model-haiku",
    "ANTHROPIC_DEFAULT_OPUS_MODEL": "model-opus",
    "ANTHROPIC_DEFAULT_SONNET_MODEL": "model-sonnet",
    "ANTHROPIC_MODEL": "model-sonnet"
  },
  "enabledPlugins": {},
  "extraKnownMarketplaces": {},
  "alwaysThinkingEnabled": false,
  "skipDangerousModePermissionPrompt": true,
  "model": "sonnet"
}
```

## Restoring Original Settings

If you need to restore the settings from before your first switch:

```bash
# Copy the backup to the target path
cp ~/.cc-switch-cli/default.json ~/.claude/settings.json
```

## Configuration

The `config.toml` file stores the target settings path:

```toml
[target]
path = "C:\\Users\\<username>\\.claude\\settings.json"
```

Edit this if your Claude Code settings are in a non-default location.
