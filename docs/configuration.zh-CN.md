# 配置说明

## 主配置文件

主配置文件通常是：

```text
config/codecli-channels.json
```

建议从下面的示例开始：

```text
config/codecli-channels.example.json
```

兼容旧路径：

```text
config/qqbot.json
config/qqbot.example.json
```

## 顶层字段

- `defaultAccountId`：旧 QQ 配置兼容字段；当使用新 `channels` 模型时可忽略
- `defaultTimezone`：bridge 的默认时区
- `channels`：多平台、多实例 channel 配置
- `accounts`：旧 QQ 凭据映射，仍兼容，会在加载时自动归一化到 `channels`
- `bridge`：bridge 的行为、路由、项目列表、超时和路径设置

## 重要 bridge 字段

### `channelIds`

控制哪些 channel 实例参与 bridge 编排。

示例：

- `default`
- `feishu-main`
- `weixin-main`

### `allowedScopes`

控制哪些用户或群可以使用 bridge。

推荐格式：

- `default:user:<openid>`
- `default:group:<group_openid>`
- `feishu-main:p2p:<chat_id>`
- `weixin-main:dm:<peer_user_id>`

兼容说明：

- 旧字段 `allowedTargets` 仍然兼容，并会自动映射到 `allowedScopes`
- 旧字段 `accountIds` 仍然兼容，并会自动映射到 `channelIds`

### `projects`

定义哪些本地仓库可以通过当前 channel 使用。

每个项目建议提供：

- alias
- 绝对路径
- 可选描述

### `defaultRunMode`

设置新建会话的默认模式：

- `write`
- `read`

### `implicitMessageMode`

控制普通消息首次自动建会话时采用什么初始模式；后续普通消息会沿用当前会话模式：

- `write`
- `read`

### 路径字段

建议显式设置：

- `dataDir`
- `stateFile`
- `auditFile`

这样可以避免状态文件落到不直观的默认路径。

### `maxReplyChars`

推荐使用的聊天回复分段长度限制。

旧字段 `qqMaxReplyChars` 仍然兼容，并会自动映射到 `maxReplyChars`。
