# qq-codex-go

一个面向 **官方 QQ 机器人 → 本机 Codex CLI** 的 Go 版桥接器。

它的目标不是做一个简单的“消息转命令”玩具，而是尽量让你在 QQ 里获得接近“直接使用本机 Codex”的体验：

- 支持多个 QQ 机器人账号
- 支持多个项目目录切换
- 支持本地持久会话
- 支持 Codex 原生审批转发到 QQ
- 默认不把 thinking、工具噪声、MCP 启动信息刷到 QQ

---

## 1. 项目定位

`qq-codex-go` 的核心能力是把一条 QQ 会话映射成：

`QQ 会话 -> 项目目录 -> 本地桥接会话 -> Codex thread`

这意味着：

- 你在 QQ 里不是每条消息都重新开一个全新 Codex 上下文
- 你可以在同一个 QQ 会话里连续追问、继续改代码、继续审批
- 你可以切项目，也可以手动切本地会话
- 当 Codex 需要审批时，桥接会把审批请求转发给你，你可以在 QQ 里回复“同意/拒绝”或 `/approve` `/deny`

和旧版“每条消息直接 `codex exec` 一次”的桥接不同，这个项目更接近“会话型远程操作”。

---

## 2. 功能概览

当前版本已经支持：

- 官方 QQ 机器人网关接入，不走 OneBot
- 多机器人账号并行监听
- 多项目目录切换
- 本地会话持久化
- 普通消息直接进入 Codex
- `/ask` 显式只读分析
- `/run` 显式工作区可写执行
- `/project list|current|use <alias>` 项目切换
- `/session list|current|new|switch` 会话管理
- `/mode write|read` 当前会话默认模式切换
- `/clear` 开新会话
- 桥接层高风险确认 `/confirm`
- Codex 原生审批 `/approve` `/deny`
- 审批自然语言回复：`同意` / `拒绝` / `yes` / `no`
- QQ 侧默认静默，只同步最终回复和审批提示
- 审计日志、状态文件持久化

---

## 3. 先理解 6 个核心概念

### 3.1 机器人账号 `accountId`

你可以同时接多个官方 QQ 机器人，每个机器人在配置里都有一个逻辑 ID，例如：

- `default`
- `bot2`

这个 ID 不等于 QQ 号，而是桥接器内部区分不同机器人的名字。

### 3.2 目标 `target`

目标是“消息要回给谁”。当前主要有两种：

- 私聊：`user:<openid>`
- 群聊：`group:<group_openid>`

桥接白名单也是基于这个维度判断的。

### 3.3 项目 `project`

项目就是一个本地工作目录，例如：

- `/Users/ming/ai/feishu-connect`
- `/Users/ming/ai/feishu-connect/qq-codex-go`

你在 QQ 里发的需求，最终都会落到某个项目目录下执行。

### 3.4 本地桥接会话 `session`

桥接器自己维护一层“本地会话”，例如：

- `s1`
- `s2`
- `s3`

这层会话负责记住：

- 当前 QQ 会话属于哪个项目
- 当前项目下绑定的是哪个 Codex thread
- 当前默认模式是 `write` 还是 `read`

### 3.5 Codex thread

这是 Codex 自己的线程 ID，例如：

- `019ccb74-9163-7ef1-84fb-23bf86d7e181`

桥接器会把它保存在本地 `session` 上，这样后续消息可以继续接着原上下文跑。

### 3.6 审批分两层

这一点非常重要：

#### 第一层：桥接层高风险确认

例如你发了明显高风险任务，桥接器可能要求你先 `/confirm`。

这是桥接器自己的保护，不是 Codex 原生审批。

#### 第二层：Codex 原生审批

当 Codex 真正执行过程中，遇到需要审批的操作时，会生成原生审批请求。

这时你需要回复：

- `/approve` 或 `/deny`
- 或者直接回复：`同意` / `拒绝`

这是和 Codex 原生审批一一对应的。

**不要把 `/confirm` 和 `/approve` 混为一谈。**

---

## 4. 运行前准备

### 4.1 环境要求

建议至少满足：

- macOS / Linux
- Go 1.22+
- 已安装可用的 `codex` CLI
- `codex` 已完成登录
- 你已经在 QQ 官方机器人后台拿到：
  - `appId`
  - `clientSecret`

### 4.2 你需要先确认的事实

在正式使用前，先确认：

- 本机命令行能直接运行 `codex`
- 当前系统用户有权限访问你的项目目录
- 机器人账号已经配置到 QQ 官方平台
- 你知道自己的私聊 `openid`，或者你知道群的 `group_openid`

如果你不知道 `openid` 是什么：

- 它是 QQ 官方机器人事件里标识“这个用户”的唯一 ID
- 它不是 QQ 号
- 桥接白名单里配置的是这个 `openid`

---

## 5. 配置文件说明

### 5.1 配置文件位置

- 示例配置：`/Users/ming/ai/feishu-connect/qq-codex-go/config/qqbot.example.json`
- 实际配置：`/Users/ming/ai/feishu-connect/qq-codex-go/config/qqbot.json`

### 5.2 一个典型配置

```json
{
  "defaultAccountId": "default",
  "defaultTimezone": "Asia/Shanghai",
  "accounts": {
    "default": {
      "enabled": true,
      "appId": "你的AppID",
      "clientSecret": "你的AppSecret"
    },
    "bot2": {
      "enabled": true,
      "appId": "第二个机器人AppID",
      "clientSecret": "第二个机器人AppSecret"
    }
  },
  "bridge": {
    "enabled": true,
    "accountIds": ["default", "bot2"],
    "allowAllTargets": false,
    "allowedTargets": [
      "default:user:你的openid",
      "bot2:user:第二个机器人的openid"
    ],
    "requireCommandPrefix": true,
    "readOnlyPrefixes": ["/ask", "/read", "问问"],
    "writePrefixes": ["/run", "/exec", "执行"],
    "confirmPrefixes": ["/confirm", "/确认"],
    "projects": {
      "feishu-connect": {
        "path": "/Users/ming/ai/feishu-connect",
        "description": "当前主项目"
      },
      "qq-codex-go": {
        "path": "/Users/ming/ai/feishu-connect/qq-codex-go",
        "description": "Go 版 QQ-Codex 持久会话桥接"
      }
    },
    "defaultProject": "qq-codex-go",
    "codexTimeoutMs": 600000,
    "qqMaxReplyChars": 1500,
    "maxPromptChars": 4000,
    "readOnlyCodexSandbox": "read-only",
    "writeCodexSandbox": "workspace-write",
    "codexModel": null,
    "confirmationTtlMs": 600000,
    "auditEnabled": true,
    "defaultRunMode": "write",
    "implicitMessageMode": "write"
  }
}
```

### 5.3 顶层字段说明

#### `defaultAccountId`

默认机器人账号 ID。

#### `defaultTimezone`

默认时区，通常填 `Asia/Shanghai`。

### 5.4 `accounts` 字段说明

每个账号都对应一个官方 QQ 机器人：

- `enabled`：是否启用
- `appId`：机器人 AppID
- `clientSecret`：机器人密钥

### 5.5 `bridge.allowedTargets` 白名单写法

这是最容易配错的一项。

支持写法：

- `default:user:<openid>`
- `bot2:user:<openid>`
- `default:group:<group_openid>`
- `user:<openid>`
- `group:<group_openid>`

推荐始终写带 `accountId` 的完整形式，这样不会串账号。

#### 私聊白名单示例

```json
"allowedTargets": [
  "default:user:F1B37818960F8CAF813D4BF43A6F4547"
]
```

#### 群聊白名单示例

```json
"allowedTargets": [
  "default:group:你的group_openid"
]
```

### 5.6 `projects` 项目配置

每个项目都要有：

- 别名
- 本地绝对路径
- 可选描述

例如：

```json
"projects": {
  "repo-a": {
    "path": "/Users/ming/work/repo-a",
    "description": "主仓库"
  },
  "repo-b": {
    "path": "/Users/ming/work/repo-b",
    "description": "实验仓库"
  }
}
```

### 5.7 模式相关字段

#### `defaultRunMode`

当前会话第一次创建时的默认模式：

- `write`
- `read`

#### `implicitMessageMode`

普通消息（没有 `/ask` `/run` 前缀）走什么模式。

当前推荐：

- `write`：普通聊天直接进入可写执行链
- `read`：普通聊天只做只读分析

### 5.8 审批与超时字段

#### `codexTimeoutMs`

单次 Codex 任务最长执行时间，单位毫秒。

#### `confirmationTtlMs`

审批等待时间，单位毫秒。

### 5.9 回复长度字段

#### `qqMaxReplyChars`

单条 QQ 回复最大长度。超过会自动分段发送。

#### `maxPromptChars`

单次用户输入传给 Codex 的最大字符数。

### 5.10 关于状态文件路径的一个现实建议

当前版本如果你只传：

```bash
-config ./config/qqbot.json
```

而又**没有显式配置** `bridge.dataDir` / `bridge.stateFile` / `bridge.auditFile`，实际生成路径通常会落在：

- `config/config/data/state.json`
- `config/config/data/bridge-audit.jsonl`

这是当前实现的实际表现。

如果你不想记这个路径，建议在 `qqbot.json` 里显式写死：

```json
"bridge": {
  "dataDir": "/Users/ming/ai/feishu-connect/qq-codex-go/data",
  "stateFile": "/Users/ming/ai/feishu-connect/qq-codex-go/data/state.json",
  "auditFile": "/Users/ming/ai/feishu-connect/qq-codex-go/data/bridge-audit.jsonl"
}
```

这样最直观，也最不容易混乱。

---

## 6. 启动方式

### 6.1 开发调试推荐：前台运行

最稳妥：

```bash
cd /Users/ming/ai/feishu-connect/qq-codex-go
go build -o ./bin/qq-codex-go ./cmd/qq-codex-go
./bin/qq-codex-go -config ./config/qqbot.json
```

这样你能直接看到：

- 机器人是否 `READY`
- 是否收到 QQ 消息
- 是否成功回消息
- 是否发生审批/会话错误

### 6.2 直接 `go run`

也可以：

```bash
cd /Users/ming/ai/feishu-connect/qq-codex-go
go run ./cmd/qq-codex-go -config ./config/qqbot.json
```

### 6.3 为什么调试期更推荐前台运行

因为你现在做的是“真实远程操作 Codex”，不是单纯的 webhook 服务。

前台运行有 3 个好处：

- 一眼能看见消息有没有收到
- 一眼能看见桥接是不是卡在 Codex 会话上
- 一眼能看见审批有没有转发出去

---

## 7. QQ 侧完整用户手册

这一节就是给“真正的使用者”看的。

### 7.1 健康检查

先发：

```text
/ping
```

预期回复：

```text
pong，Go 版 QQ-Codex bridge 在线。
```

如果 `/ping` 没回，先别测别的，先看服务是否正常启动。

### 7.2 查看帮助

```text
/help
```

### 7.3 普通对话

如果当前配置：

- `implicitMessageMode = "write"`

那么普通消息会直接进入 Codex，例如：

```text
hi
```

```text
帮我看看这个仓库是干什么的
```

```text
修一下 README 里的错别字
```

如果当前配置：

- `implicitMessageMode = "read"`

那么普通消息默认只读分析。

### 7.4 显式只读分析

```text
/ask 帮我分析这个报错的根因
```

只读模式下，桥接会把 Codex 沙箱设成只读分析。

适合：

- 看代码
- 看日志
- 看配置
- 分析问题
- 给方案

### 7.5 显式可写执行

```text
/run 帮我修复 README 并补一段使用说明
```

适合：

- 改代码
- 改文档
- 写脚本
- 跑项目内允许的构建/测试

### 7.6 项目切换

#### 查看所有项目

```text
/project list
```

#### 查看当前项目

```text
/project current
```

#### 切换项目

```text
/project use feishu-connect
```

或者：

```text
/project use qq-codex-go
```

**建议习惯：**

- 做新任务前先 `/project current`
- 要跨仓操作时先 `/project use <alias>`

### 7.7 会话管理

#### 当前会话

```text
/session current
```

#### 查看当前项目下的所有会话

```text
/session list
```

#### 新建会话

```text
/session new 修 README
```

#### 切换会话

```text
/session switch s3
```

### 7.8 `/clear` 的真实含义

```text
/clear
```

含义是：

- 当前项目下立即开启一个新的本地会话
- 后续消息不再继续旧 thread
- 相当于“重新开一个新上下文”

它**不会**：

- 删除你的项目文件
- 删除仓库
- 删除全局 Codex 历史

### 7.9 默认模式切换

查看当前模式：

```text
/mode
```

切成默认可写：

```text
/mode write
```

切成默认只读：

```text
/mode read
```

这只影响“当前会话”的默认行为。

### 7.10 高风险任务确认 `/confirm`

如果桥接器识别到任务文本里有明显高风险指令，例如：

- `rm -rf`
- `git reset --hard`
- `drop database`
- `truncate table`
- `sudo`

它可能先要求你确认。

这时你用：

```text
/confirm
```

### 7.11 Codex 原生审批怎么处理

当 Codex 真的在执行中需要审批，桥接会把审批请求发到 QQ。

你可以回复：

```text
/approve
```

或者：

```text
/deny
```

也可以直接回复自然语言：

- `同意`
- `拒绝`
- `yes`
- `no`
- `ok`
- `cancel`

### 7.12 长回复会自动分段

如果 Codex 最终回复很长，桥接会自动按 `qqMaxReplyChars` 分段发回 QQ。

### 7.13 多机器人怎么隔离

多个机器人之间会按：

- `accountId`
- `chatType`
- `targetId`

组合成独立会话键。

也就是说：

- `default` 机器人和 `bot2` 机器人默认不会串会话
- 同一个人用不同机器人，也会是两套会话

---

## 8. 推荐使用习惯

### 场景 A：纯聊天式操作

1. `/project current`
2. 直接发：`帮我看看这个项目结构`
3. 继续追问：`入口文件在哪`
4. 再发：`顺手把 README 里的拼写错误改一下`

### 场景 B：先分析再改

1. `/ask 帮我分析这个报错`
2. `/ask 先别改，只告诉我根因`
3. `/run 按你刚才的方案最小改动修复`

### 场景 C：切项目远程操作

1. `/project list`
2. `/project use qq-codex-go`
3. `/clear`
4. 直接发需求

### 场景 D：需要原生审批

1. 发一个会真正触发越权/敏感操作的需求
2. 等 QQ 收到审批提示
3. 回复 `同意` 或 `/approve`

---

## 9. 重要文件与运行痕迹

### 9.1 代码入口

- 程序入口：`/Users/ming/ai/feishu-connect/qq-codex-go/cmd/qq-codex-go/main.go`
- QQ 桥接核心：`/Users/ming/ai/feishu-connect/qq-codex-go/internal/bridge/service.go`
- 命令解析：`/Users/ming/ai/feishu-connect/qq-codex-go/internal/bridge/commands.go`
- Codex Runner：`/Users/ming/ai/feishu-connect/qq-codex-go/internal/codex/runner.go`
- 会话存储：`/Users/ming/ai/feishu-connect/qq-codex-go/internal/store/store.go`

### 9.2 常见状态文件

当前版本常见路径：

- 状态文件：`/Users/ming/ai/feishu-connect/qq-codex-go/config/config/data/state.json`
- 审计日志：`/Users/ming/ai/feishu-connect/qq-codex-go/config/config/data/bridge-audit.jsonl`

### 9.3 运行日志

如果你是这样启动：

```bash
./bin/qq-codex-go -config ./config/qqbot.json
```

日志就在当前终端。

如果你做了重定向，例如：

```bash
./bin/qq-codex-go -config ./config/qqbot.json > ./logs/qq-codex-go.log 2>&1
```

那日志文件就是：

- `./logs/qq-codex-go.log`

### 9.4 隔离的 Codex 运行目录

桥接不会直接把会话写进你当前这条对话的运行环境，而是为项目准备一个隔离 `CODEX_HOME`。

macOS 下一般落在：

- `$TMPDIR/qq-codex-go-codex-home/<hash>/`

里面会有：

- `sessions/`
- `log/codex-tui.log`
- `shell_snapshots/`
- `state_5.sqlite`

这套目录是桥接自己的运行痕迹。

---

## 10. 如何“从零开始”清历史

如果你怀疑桥接绑定了脏会话、旧 thread、奇怪的历史状态，可以这样做。

### 10.1 停服务

先停掉 `qq-codex-go` 服务进程。

### 10.2 删除桥接状态

删除：

```bash
rm -rf /Users/ming/ai/feishu-connect/qq-codex-go/config/config/data
```

### 10.3 删除桥接隔离的 Codex 历史

删除：

```bash
rm -rf "$TMPDIR/qq-codex-go-codex-home"
```

### 10.4 重启服务

```bash
cd /Users/ming/ai/feishu-connect/qq-codex-go
./bin/qq-codex-go -config ./config/qqbot.json
```

### 10.5 重新验证

先在 QQ 发：

```text
/ping
```

再发：

```text
hi
```

> 注意：这套“清历史”不会删除你全局的 `~/.codex`，只会删除这个桥接器自己的本地状态和隔离运行历史。

---

## 11. 常见问题排查

### 11.1 `/ping` 有回复，但普通消息没反应

优先怀疑这几种情况：

1. 旧 thread 被复用，导致 `resume` 卡住
2. 当前会话状态文件里绑定了历史脏 thread
3. 服务进程虽然启动过，但已经退出
4. 普通消息模式配置和你预期不一致

建议顺序：

1. `/clear`
2. `/session current`
3. 查看终端日志里有没有 `收到 QQ 消息`
4. 必要时按“从零开始清历史”处理

### 11.2 机器人完全不回消息

先查：

- 服务是否还在运行
- 日志里有没有 `QQ 网关 READY`
- 白名单 `allowedTargets` 是否正确
- `appId` / `clientSecret` 是否正确

### 11.3 收到“当前目标未放行 Codex 执行”

说明你的 `openid` 或 `group_openid` 不在白名单里。

去改：

- `bridge.allowedTargets`

### 11.4 审批提示没有发到 QQ

先区分两种情况：

- 是桥接层高风险确认没发出来
- 还是 Codex 原生审批没发出来

然后看服务日志：

- 是否收到消息
- 是否进入 `RunTurn`
- 是否出现审批转发的主动消息日志

### 11.5 多机器人串会话吗

按当前实现：

- 不同 `accountId` 默认是分开的
- 不同用户 `targetId` 也是分开的
- 不同项目 `projectAlias` 也分开

### 11.6 群里怎么用

当前网关处理的是：

- 私聊消息
- `GROUP_AT_MESSAGE_CREATE`

所以群里一般要 **@机器人** 才会进来。

### 11.7 后台运行为什么有时没撑住

开发调试期更推荐前台运行。

原因很简单：

- 你需要实时看日志
- 宿主环境可能回收脱离终端的后台进程
- 前台运行更容易第一时间确认“到底是没收到消息，还是 Codex 卡住了”

---

## 12. 推荐的第一次验收流程

如果你是第一次搭起来，建议按这个顺序验收：

1. 启动服务
2. 看终端出现两条 `QQ 网关 READY`
3. QQ 发 `/ping`
4. QQ 发 `/project current`
5. QQ 发 `hi`
6. QQ 发 `/ask 帮我看看当前项目目录结构`
7. QQ 发 `/clear`
8. QQ 发 `/session current`
9. 再验证一次带审批的任务

这样最容易定位问题。

---

## 13. 当前项目和旧版 Node 桥接的区别

旧版 `qqbot-mcp-server` 更像：

- 收消息
- 丢给一次性执行
- 回结果

这个 Go 版更像：

- 收消息
- 找当前项目
- 找当前桥接会话
- 绑定/续用 Codex thread
- 处理审批
- 返回结果

所以这个版本更适合“长期在 QQ 里远程操作 Codex”。

---

## 14. 一句结论

如果你真正想要的是：

- 用 QQ 像直接和 Codex 聊天一样工作
- 还能切项目、保留上下文、接审批

那你应该把这个项目当成一个“会话型 QQ 远程 Codex 终端”，而不是普通机器人问答插件。

