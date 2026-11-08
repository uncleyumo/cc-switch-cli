# ccsc

[English](README.md) | [中文](README_zh.md)

把 Claude Code 的模型配置收成一个分组库，全局或按项目应用。单文件 Go 二进制，
只有一个名字：`ccsc`。

```bash
ccsc group      # 先把分组建起来
ccsc status     # 看现在到底在用什么
```

从源码构建需要 Go 1.24.5+。

## 安装

### 预编译二进制

从 [GitHub Releases](https://github.com/uncleyumo/cc-switch-cli/releases)
下载对应平台的压缩包，解压后把 `ccsc` 放进 `PATH`：

| 平台 | 资源文件 |
|---|---|
| Windows x86_64 | `ccsc_windows_amd64.zip` |
| Windows ARM64 | `ccsc_windows_arm64.zip` |
| Linux x86_64 | `ccsc_linux_amd64.zip` |
| Linux ARM64 | `ccsc_linux_arm64.zip` |
| macOS Intel | `ccsc_darwin_amd64.zip` |
| macOS Apple Silicon | `ccsc_darwin_arm64.zip` |

### 从源码构建

```bash
git clone https://github.com/uncleyumo/cc-switch-cli && cd cc-switch-cli

build.bat                     # Windows：六个平台产物到 dist/
go build -o ccsc .            # 任意系统：只出本地二进制
```

### 自更新

```bash
ccsc upgrade --check   # 有没有新版本
ccsc upgrade           # 下载、校验 sha256、替换、自检
```

替换前把当前二进制改名为 `ccsc.exe.bak.YYYYMMDD`；新二进制跑不通 `--version`
会自动回滚。

## 两个概念

只有两个词。

**分组（group）**：一个接入点 + 五个模型槽位 —— `fable`、`opus`、`sonnet`、
`haiku`、`subagent`。分组自带 `base_url`，所以多网关、多接入点天然支持：
换网关就是换分组。

**临时组合（optional）**：进项目级配置的另一条路 —— 不经过分组库，现场连一个
new-api 接入点、从它返回的列表里挑模型、写进项目。写入后一直有效，直到被
`ccsc local` 或 `ccsc local clear` 覆盖。

一个会话同时只能指向一个接入点。这是 Claude Code 的约束（只有一份
`ANTHROPIC_BASE_URL`），不是 ccsc 的限制。

## 命令

```
ccsc                          工作台，停在分组列表
├─ group                      分组：新建 / 编辑 / 应用全局 / 删除
├─ local                      把分组绑定到本项目
│  └─ clear                   清除本项目里 ccsc 写过的模型键
├─ optional                   现场连接、拉模型、写进本项目
├─ status                     当前生效配置（只读）
│    --reveal                 显示完整令牌
│    --dir <path>             看别的项目目录
├─ config                     设置面板
├─ mcp                        用户 / 本地 / 项目三层 MCP（只读）
│  ├─ test [name]             真实 initialize + tools/list 握手
│  --json
├─ skills                     用户 / 项目两层 Skills（只读）
├─ upgrade [--check]          从 GitHub Releases 自更新
├─ version
└─ completion <shell>
```

`group`、`local`、`optional`、`config` 是纯 TUI。在非终端环境里直接报错退出，
不会挂在那里等按键。只读命令是纯文本，可以管道。

## ccsc 写哪些键

ccsc 只碰一张固定的表，其余一律不动：`hooks`、`permissions`、
`enabledPlugins`、`effortLevel`、`theme`、未知键、`modelSettings` 的其他字段
都会原样保留。

| 键 | 全局 | 项目 |
|---|:--:|:--:|
| `env.ANTHROPIC_BASE_URL` | ✓ | ✓ |
| `env.ANTHROPIC_AUTH_TOKEN` | ✓ | ✓ |
| `env.ANTHROPIC_DEFAULT_{FABLE,OPUS,SONNET,HAIKU}_MODEL` | ✓ | ✓ |
| `env.ANTHROPIC_DEFAULT_*_MODEL_NAME` | ✓ | ✓ |
| `env.ANTHROPIC_DEFAULT_MODEL`（启动兜底） | ✓ | ✓ |
| `env.CLAUDE_CODE_SUBAGENT_MODEL` | ✓ | ✓ |
| `env.CLAUDE_CODE_MAX_CONTEXT_TOKENS` | ✓ | ✓ |
| `env.CLAUDE_CODE_AUTO_COMPACT_WINDOW` | 清除 | 清除 |
| `env.CLAUDE_CODE_AUTO_COMPACT_WINDOW_TOKENS` | 清除 | 清除 |
| `modelSettings.<id>.autoCompactWindow` | ✓ | — |
| 顶层 `autoCompactWindow` | — | ✓ |

写入是保序 JSON 的读改写：键序和缩进原样回去，应用一次分组只产生八行 diff，
不会把整个文件重排。

### 项目层

**项目根就是 ccsc 被执行的目录。** Claude Code 不会向上找 `.claude/`，ccsc 也
不找。`--dir` 只替换根，不触发任何向上搜索。

目标文件是 `<cwd>/.claude/settings.local.json`。如果那里没有同级的
`.claude/settings.json` 而某个祖先目录有，ccsc 会警告 —— 在 `<cwd>` 启动的
Claude Code 不会读祖先的文件，这是「看起来配好了、其实从没生效」最安静的
一种死法。

### 启动模型

四层，从高到低：

| # | 来源 | 谁写 |
|---|---|---|
| 1 | `ANTHROPIC_MODEL`（环境变量） | 你或旧配置；ccsc 只报告 |
| 2 | `/model` 的选择 → 顶层 `model` | Claude Code 自己 |
| 3 | `ANTHROPIC_DEFAULT_MODEL` | ccsc |
| 4 | Claude Code 内置默认 | — |

ccsc 只写第 3 层，绝不和 `/model` 抢开关。值一定是解析后的完整模型 ID，因为
这里写别名会被忽略。

### 上下文窗口与自动压缩

两件不同的事：

- **硬上限**（`CLAUDE_CODE_MAX_CONTEXT_TOKENS`）—— 模型有多大。它决定
  `/status` 的显示，也决定默认压缩点。
- **`autoCompactWindow`** —— 对话涨到多少开始压缩。它永远被硬上限截断，所以
  填得比硬上限大等于没填。

默认值：每个槽位的压缩点 = **有效窗口的 90%**（标了 1M 的槽是 1M，其余是分组的
硬上限）。逐槽显式填了就按你填的来。

两层故意用不同落点：

- **全局** → `modelSettings.<id>.autoCompactWindow`，逐模型，写进已经存在的那个
  条目，你的 `effortLevel` 不会丢。
- **项目** → 一个顶层 `autoCompactWindow`。Claude Code 跨文件合并同名
  `modelSettings` 条目时是整块替换还是按字段合并，文档没有保证，猜错就会把
  `effortLevel` 冲掉。槽位之间值不一致时取最小值：早压缩不会溢出。

`CLAUDE_CODE_AUTO_COMPACT_WINDOW` / `_TOKENS` 每次应用都会被清除 —— 它们压过所有
逐模型设置，留着就等于让逐模型配置变成死配置。

## 分组文件

`~/.cc-switch-cli/groups/<name>.json`，权限 `0600`：

```jsonc
{
  "schema": 1,
  "base_url": "https://new-api.example.com",
  "token": "sk-...",
  "window": 500000,             // 组级硬上限，0 表示不写
  "slots": {
    "fable":    { "model": "packy-official/deepseek-flash", "name": "DeepSeek" },
    "opus":     { "model": "packy-cc/claude-opus-5-5", "window": 1000000 },
    "sonnet":   { "model": "packy-sale/deepseek-flash" },
    "haiku":    { "model": "zen/glm-5.3-flash-free", "compact": 150000 },
    "subagent": { "model": "zen/glm-5.3-flash-free" }
  }
}
```

槽位的 `window` 只有两个取值：`0` 或 `1000000` —— `[1m]` 是 Claude Code 唯一认得
的逐模型窗口控制。别的数值属于组级 `window`。四个家族槽位必填；`subagent` 可空，
空的时候完全不写 `CLAUDE_CODE_SUBAGENT_MODEL`。

## `optional`

```
ccsc optional
```

一个面板，不开二级窗口：

1. 填 `base_url` 和令牌。
2. `ctrl+f` 调 `/v1/models`，把模型列表灌进模型行。
3. 在模型行里打字就地过滤，`enter`（或 `→`）接受高亮那条并跳到下一行。任何 ID
   都能手打 —— new-api 会转译协议，所以列表原样全给，不按
   `supported_endpoint_types` 过滤。

`subagent` 留空表示不配置；四个家族槽位留空会直接报错。

结果写进项目，一直有效，直到被 `ccsc local` 或 `ccsc local clear` 覆盖。

## `status`、`mcp`、`skills`

`ccsc status` 把用户、项目、本地三层里的每个模型键解析一遍，给出最终生效值和它
来自哪一层，再补上启动槽位、子代理模型、压缩点、MCP/Skills 计数。令牌默认脱敏，
`--reveal` 打着横幅显示完整值。

`ccsc mcp` 读三个作用域，并把输的那条标出来：

| 作用域 | 落盘位置 | 优先级 |
|---|---|---|
| user | `<CLAUDE_CONFIG_DIR 或 ~>/.claude.json` → `mcpServers` | 最低 |
| local | 同一个文件 → `projects["<绝对 cwd>"].mcpServers` | 最高 |
| project | `<cwd>/.mcp.json` → `mcpServers` | 中间 |

`settings.json` 里没有服务定义 —— 只有审批键，看板读它来判断某个 `.mcp.json`
服务是已批准还是待批准。

`ccsc mcp test [name]` 启动服务、走一遍 `initialize` 和 `tools/list`，打印工具数
和延迟。零写操作。

`ccsc skills` 合并 `~/.claude/skills`、`.claude/skills` 以及两者的 `commands`
目录，项目覆盖用户。

## 键位

列表：`↑↓` / `j` `k` / `←→` 移动，`enter` 选定，`q` 退出。分组列表另有 `n` 新建、
`u` 应用全局、`d` 删除。

面板（一份配置一个屏，不开二级窗口）：

| 键 | 作用 |
|---|---|
| `↑↓` `j` `k` `tab` `shift+tab` | 上下移动 |
| `←` / `→` | 上一项 / 下一项 |
| `enter` | 接受高亮建议，并跳到下一项 |
| `ctrl+f` | 从接入点拉模型列表 |
| `v` | 显示密钥原文 10 秒 |
| `ctrl+u` | 清空当前行 |
| `ctrl+s` | 保存（有非法行时拒绝） |
| `esc` | 取消 |

在模型行打字就地过滤建议；在密钥行打字会整体替换旧值。

上下文窗口接受 `500k`、`567k`、`1m`、`2m` 或纯数字，回显也用同一套短写法。

## 输出

ccsc 打出来的一切都是纯 ASCII 英文：没有对勾、箭头、框线或 emoji。任何终端、
管道和日志里都一样可读。

## 文件

```
~/.cc-switch-cli/
├─ config.toml                 默认值：base_url、令牌、启动槽位、硬上限
├─ groups/<name>.json          分组库（明文令牌，0600）
├─ cache/models/*.json         模型列表，按 host + 令牌哈希缓存
├─ state.json                  谁被应用到了哪、ccsc 拥有哪些模型条目
├─ backups/<target>/*.json     每个被写文件最近十份快照
└─ audit.jsonl                 追加式历史，5 MB × 3
```

删掉这个目录和二进制，就等于从没装过。

设 `CCSC_HOME` 可以整体搬走。

## 令牌

明文存放，权限 `0600`。这是**脱敏显示，不是加密存储**：处处显示
`sk-tes******mnop`。面板里按 `v` 显示原文 10 秒；`ccsc status --reveal`
会先打一条警告再打印一次。

凭据只有一个概念。ccsc 写 `ANTHROPIC_AUTH_TOKEN`，网关按
`Authorization: Bearer` 收。`ANTHROPIC_API_KEY`（`x-api-key` 头）不作为选项
暴露；目标文件里如果已经有一条同值的，会被当成冗余删掉并在回执里说明。

## v1 → v2

第一次运行 TUI 命令时自动迁移一次：

```bash
ccsc group     # 先迁移，再打开分组库
```

`~/.cc-switch-cli/models/*.json` 每个变成一个分组。原文件移到
`~/.cc-switch-cli/backups/legacy-models-<日期>/`，`.migrated-v2` 标记保证幂等，
已存在的同名分组绝不覆盖。

| v1 | v2 |
|---|---|
| `ccsc use <name>` | `ccsc group` → `u` |
| `ccsc model *` | `ccsc group` |
| `ccsc local [name]` | `ccsc local` → `enter` |
| `ccsc local clear --model` | `ccsc local clear` |
| `ccsc mcp add/remove/show/tools` | 取消 —— MCP 只读 |
| `ccsc export` / `ccsc import` | 取消 |
| `ccsc backup` / `ccsc current` / `ccsc init` | 取消，状态改为隐式 |
| `ccsc model test` | 取消 —— 用 `ccsc status` 看最终生效配置 |

## 不做

- 不做代理、不做模型路由：一个会话一个接入点。
- 不做 MCP / Skills 的任何写操作，只有只读看板加一个连通性探测。
- 不做 `modelPicker`。
- 不做 `--no-tui` 降级：没有终端就报错退出。
- `group` 和 `local` **不自动拉取模型列表**。自动拉取只存在于 `optional`，
  因为那里本来就是在跟 new-api 打交道。

## 构建矩阵

`build.bat` 交叉编译六个静态二进制到 `dist/`
（`ccsc_<version>_<os>_<arch>`）。`.goreleaser.yml` 产出发布资产
`ccsc_<os>_<arch>.zip` 加 `checksums.txt` —— 正是 `ccsc upgrade` 期望的名字。
