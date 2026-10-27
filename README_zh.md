# cc-switch-cli

[English](README.md) | [中文](README_zh.md)

把 Claude Code 的模型提供商配置收成库，一条命令原子切换到
`~/.claude/settings.json`。单文件 Go 二进制；放到 `PATH` 后可改名为 **`ccsc`**
方便使用。

```bash
ccsc use packy-bridge   # 切换全局提供商（字段合并，原子写入，自动备份）
ccsc current            # → packy-bridge
```

从源码构建需要 Go 1.24.5+。

## 安装

### 预编译二进制（推荐）

从 [GitHub Releases](https://github.com/<owner>/cc-switch-cli/releases)
（或你自己的镜像）下载对应平台资源，放到 `PATH`，需要短名就自行改名：

| 平台 | 资源文件 |
|---|---|
| Windows x86_64 | `cc-switch-cli_<version>_windows_amd64.exe` |
| Windows ARM64 | `cc-switch-cli_<version>_windows_arm64.exe` |
| Linux x86_64 | `cc-switch-cli_<version>_linux_amd64` |
| Linux ARM64 | `cc-switch-cli_<version>_linux_arm64` |
| macOS Intel | `cc-switch-cli_<version>_darwin_amd64` |
| macOS Apple Silicon | `cc-switch-cli_<version>_darwin_arm64` |

```bash
# Linux / macOS 示例
chmod +x cc-switch-cli_*_linux_amd64
mv cc-switch-cli_*_linux_amd64 ~/.local/bin/ccsc

# Windows（PowerShell）示例
# Move-Item cc-switch-cli_*_windows_amd64.exe $env:USERPROFILE\bin\ccsc.exe
```

### 从源码构建

```bash
git clone <repo> && cd cc-switch-cli

# Windows：在 dist/ 下交叉编译多平台发布产物
build.bat
# 可选：set VERSION=2.0.1 && build.bat

# 任意系统 — 仅本地二进制
go build -ldflags="-s -w" -o ccsc .
```

然后初始化配置目录：

```bash
ccsc init               # 创建 ~/.cc-switch-cli/ + config.toml
```

`~/.cc-switch-cli/models/` 下每个文件就是一份完整的 Claude Code `settings.json`：

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

仓库 `models/` 目录附带模板（`claude`、`deepseek`、`mimo`），`init` 后可拷贝或改写。

切换是合并语义：provider 声明的顶层键覆盖 `settings.json` 同名键，未声明的键
（hooks、permissions 等）原样保留，Claude Code 或插件在两次切换之间写入的设置
不会丢失。`env` 逐键合并，新 provider 未声明的 `ANTHROPIC_*` / `CLAUDE_CODE_*`
键会被清除。

## 命令

```
ccsc (cc-switch-cli)
├─ init                          创建 ~/.cc-switch-cli + config.toml
├─ use [model]                   全局激活；无参数 → TUI 选择器
├─ current                       打印当前激活的模型
├─ local [model]                 写入 ./.claude/settings.local.json（深度合并）
│    --overwrite                 改为整文件覆盖
│    --path <dir|file>           指定其它目录 / 文件
│  ├─ show [--reveal]            打印项目级模型 + MCP
│  └─ clear [--model]            删除该文件（或只清模型 / env 键）
├─ backup                        快照当前 settings.json
├─ model, m                      管理器 TUI（无子命令时）
│  ├─ list, ls                   列出模型 + 激活标记 + 最近测试状态
│  ├─ show <name> [--reveal]     打印 JSON（默认遮罩 token）
│  ├─ add <name> [--from x] [--no-edit]
│  ├─ edit [name]                结构化 / 原始 JSON 编辑器
│  ├─ rename <old> <new>
│  ├─ remove, rm <name> [-y]
│  ├─ test [name...]             对每个已配置的模型做流式 + 工具探测
│  │   --stream / --tool         只跑其中一项
│  │   --timeout 30s  --concurrency 4  --auth auto|bearer|x-api-key
│  ├─ doctor                     打印每个已配置模型的解析结果 / 实际上线 id（只读）
│  └─ tui                        显式打开管理器（等同裸 `model`）
├─ mcp                           MCP 管理器 TUI（无子命令时）
│  ├─ list, ls [--scope S]       列出服务器（user / local / project）
│  ├─ show <name> [--scope S] [--reveal]
│  ├─ add <name> [--scope local] [-t stdio|http|sse|ws] [-e K=V] [-H "K: V"] -- <cmd|url>
│  ├─ add-json <name> <json> [--scope S]
│  ├─ remove, rm <name> [--scope S] [-y]
│  ├─ test [name...]             initialize + tools/list 探测
│  │   --scope S  --timeout 30s  --concurrency 4
│  ├─ tools <name> [--scope S]   列出服务器暴露的工具
│  └─ tui                        显式打开管理器（等同裸 `mcp`）
├─ export                        加密导出模型 + 用户级 MCP
│    -o FILE  --models all|none|a,b  --mcps all|none|x,y  --secret-file F
├─ import <file>                 解密并导入
│    --secret-file F  --force
└─ config, cfg
   ├─ show                       解析后的路径 + 当前模型
   └─ set-target <path>          修改目标 settings.json
```

`--version` / `-v` 打印构建时写入的版本信息。

## 典型用法

```bash
ccsc model add my-proxy           # 模板 + 打开编辑器
ccsc model test                   # 对每个提供商里的每个已配置模型做可用性检查
ccsc use my-proxy                 # 全局激活
cd my-project && ccsc local my-proxy   # 仅作用于当前项目
ccsc local show                        # 查看项目级模型 + MCP
ccsc local clear                       # 取消项目级覆盖
ccsc model show my-proxy --reveal # 带密钥的完整 JSON

ccsc mcp list                     # 查看 MCP 服务器
ccsc mcp test fetch-mcp           # 探测单个服务器
ccsc mcp                          # 交互式 MCP 管理器
ccsc export --models all --mcps all --secret-file secret.txt -o backup.json
ccsc import backup.json --secret-file secret.txt
```

## `ccsc local` — 默认深度合并

`ccsc local <name>` 会深度合并进 `./.claude/settings.local.json`：提供商的
`env` 键与顶层 `model` 覆盖同名项，**其余字段全部保留**（permissions、hooks、
MCP、未知键等）。

- `--overwrite` — 整文件覆盖（丢弃项目专属键）。
- `--path <dir>` / `--path <file>` — 写 / 查看 / 清除当前目录以外的位置。
- `ccsc local show` — 汇总项目级模型覆盖，以及 project 作用域（`.mcp.json`）和 local 作用域（`~/.claude.json` → 当前项目）的 MCP 服务器。默认遮罩 token，加 `--reveal` 才显示原文。
- `ccsc local clear` — 删除 `settings.local.json`，项目回退到全局提供商。`ccsc local clear --model` 只剥掉提供商的 `env` / 顶层 `model`，保留 permissions、hooks 和自定义 env。剥完若文件变空则一并删除。MCP 文件不会被改动。

若提供商名字恰好叫 `clear` 或 `show`，用 `ccsc local -- <name>` 写入。

## `ccsc model test` — 可用性

会对每个提供商里每个已配置的 model id 真实调用 Anthropic Messages（流式 + tool-call 探测），会有少量 token 消耗。
结果缓存到 `~/.cc-switch-cli/test-status.json`；`ccsc model list` 显示该提供商最差的模型结论和模型数量。

| 状态 | 含义 |
|---|---|
| **OK** | 请求的检查全部通过 — 可完整使用。 |
| **OK~** | 流式可用（聊天可用），但本轮 tool 探测未确认 `tool_use` — 提示而非故障。 |
| **FAIL** | 流式失败（坏 token / 余额 / 未知模型 / 不可达）— 不可用。 |

流式检查是权威来源：流式通过就不会判 FAIL，即使 tool 探测短暂失败。Tool 探测
对齐 Claude Code 行为 — 发送 tools 列表但不带 `tool_choice`，因为强制
`"any"` 会被不少能正常转发 tools 的网关拒绝。

## 上下文窗口

提供商上的 **context window** 是整组默认窗口。录入写成 `128k` / `256k` /
`500k` / `1m` / `2m`，再打开编辑器也按这个写法回显。不允许纯数字（`500`、
`500000`）。落盘时写成 Claude Code 要的整数
（`CLAUDE_CODE_MAX_CONTEXT_TOKENS`）。窗口不超过 1M 且组内没有任何 id 带
`[1m]` 时，同时写入 `CLAUDE_CODE_AUTO_COMPACT_WINDOW`。

对未识别 / 网关 id，Claude Code 只把尾部 `[1m]` 当成 1M 窗口，并忽略该 id
上的 `CLAUDE_CODE_MAX_CONTEXT_TOKENS`。`[200k]` 这类后缀只在发请求时剥离，
不改变假定窗口。常见写法：组填 `500k`，真正是 1M 的那个 id 再加 `[1m]`。

`ccsc local` 在新提供商未声明这三项时会删掉旧值。`ccsc model doctor` 打印
解析后的 id、实际上线 id 和组窗口。
## `ccsc mcp` — MCP 服务器

管理 Claude Code 加载的 MCP 服务器：

| 范围 | 存储位置 |
|---|---|
| **user** | `~/.claude.json` 顶层 `mcpServers` |
| **local**（`add` 默认） | `~/.claude.json` → `projects.<cwd>.mcpServers` |
| **project** | `<cwd>/.mcp.json` |

`ccsc mcp test` 会真实执行 initialize 握手与 `tools/list`（stdio / Streamable HTTP，
兼容旧版 SSE）。结果缓存到 `~/.cc-switch-cli/mcp-test-status.json`。WebSocket
可配置但当前不探测。敏感值默认脱敏，`--reveal` 可查看原文。

写入会直接修改 `~/.claude.json`（并在 `~/.cc-switch-cli/backups/` 留时间戳备份）。
建议在 Claude Code 关闭时编辑 — 运行中的会话退出时可能回写该文件。

## `ccsc export` / `ccsc import`

在机器间迁移**全局**模型库与**用户级** MCP 服务器。导出文件为 AES-256-GCM
加密信封（scrypt 派生密钥）；导入前会校验格式版本，不兼容时在索要密钥前失败。
终端下可交互勾选，也可用 `--models` / `--mcps`（`all` / `none` / 逗号列表）。
导入时已存在条目默认跳过，除非 `--force`。

## TUI 快捷键

管理器（裸 `ccsc model`）：

| 键 | 动作 |
|---|---|
| `↑↓` `j` `k` | 导航 |
| `/` | 搜索 / 过滤 |
| `enter` `u` | 全局激活 |
| `a` / `n` / `e` / `d` | 新增 / 重命名 / 编辑 / 删除 |
| `v` `space` | 预览 JSON（遮罩） |
| `t` / `T` | 测试当前 / 测试全部 |
| `r` | 重载模型与状态 |
| `q` `esc` | 退出 |

MCP 管理器（裸 `ccsc mcp`）：

| 键 | 动作 |
|---|---|
| `enter` `l` | 列出选中服务器的工具 |
| `a` / `e` / `n` / `d` | 新增 / 编辑 / 重命名 / 删除 |
| `v` `space` | 预览 JSON（脱敏） |
| `t` / `T` | 探测当前 / 探测可见全部 |
| `s` | 循环 scope 过滤（all/user/local/project） |
| `r` | 重载 |
| `q` `esc` | 退出 |

编辑器（`ccsc model edit <name>` 或 MCP 的 `e`）：`tab`/`shift+tab` 切换字段，
`ctrl+j` 切换原始 JSON，`ctrl+s` 保存，`esc` 取消。

非 TTY 调用（`use`、`local`、`edit`、`model`）必须显式给出名称；在无终端
环境下会拒绝打开 TUI，让脚本尽早失败。

## 文件布局

```
~/.cc-switch-cli/
├─ config.toml                  目标 settings.json 路径
├─ models/*.json                提供商库
├─ <provider>.current           当前激活标记
├─ default*.json / backup-*.json  首次切换 + 手动备份
├─ model-switch.logs            追加式切换审计日志
└─ test-status.json             各提供商最近一次可用性结果
```

`ANTHROPIC_AUTH_TOKEN` / `ANTHROPIC_API_KEY` 在 `model show` 与 TUI 预览中默认
遮罩（除非 `--reveal`）；状态缓存从不保存 token 或请求体。

## 构建矩阵

`build.bat` 交叉编译 6 个静态二进制到 `dist/`：

```
cc-switch-cli_<version>_windows_amd64.exe
cc-switch-cli_<version>_windows_arm64.exe
cc-switch-cli_<version>_linux_amd64
cc-switch-cli_<version>_linux_arm64
cc-switch-cli_<version>_darwin_amd64
cc-switch-cli_<version>_darwin_arm64
```

命名遵循常见 Go 发布约定：`name_version_os_arch`。
`darwin` 即 macOS（Go 工具链对 Darwin 内核的称呼）。

## v1 → v2

| v1 | v2 |
|---|---|
| `cc-switch-cli list` | `ccsc model list` |
| `cc-switch-cli cat <name>` | `ccsc model show <name>` |
| `cc-switch-cli model set <name>` | `ccsc use <name>` |
| `cc-switch-cli model current` | `ccsc current` |
| `cc-switch-cli model backup` | `ccsc backup` |
| `cc-switch-cli local <name> [path] --force` | `ccsc local <name> [--path <p>] [--overwrite]` |

`local` 现在默认写到 `.claude/settings.local.json` 并做合并（v1 写在目录根且
整文件覆盖）。需要 v1 行为时加 `--overwrite`。
