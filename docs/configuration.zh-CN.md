# 配置说明

## 主配置文件

主配置文件通常是：

```text
config/qqbot.json
```

建议从下面的示例开始：

```text
config/qqbot.example.json
```

## 顶层字段

- `defaultAccountId`：默认 QQ 机器人账号
- `defaultTimezone`：bridge 的默认时区
- `accounts`：QQ 机器人凭据映射
- `bridge`：bridge 的行为、路由、项目列表、超时和路径设置

## 重要 bridge 字段

### `allowedTargets`

控制哪些用户或群可以使用 bridge。

推荐格式：

- `default:user:<openid>`
- `default:group:<group_openid>`

### `projects`

定义哪些本地仓库可以通过 QQ 使用。

每个项目建议提供：

- alias
- 绝对路径
- 可选描述

### `defaultRunMode`

设置新建会话的默认模式：

- `write`
- `read`

### `implicitMessageMode`

控制普通消息默认按什么模式解释：

- `write`
- `read`

### 路径字段

建议显式设置：

- `dataDir`
- `stateFile`
- `auditFile`

这样可以避免状态文件落到不直观的默认路径。
