# 飞书新人快速上手

这篇文档只讲一件事：

**怎么先拿到飞书应用的 `appId` / `appSecret`，再把它接到 `codecli-channels`。**

如果你是第一次接这个项目，真正的第一步不是写 JSON，而是：

**先去飞书开放平台创建应用，并在“应用凭证”页面拿到 `App ID` 和 `App Secret`。**

我已经按当前飞书开放平台页面结构核对过，稳定入口是：

- [飞书开放平台官网](https://open.feishu.cn/)
- [飞书开发者后台](https://open.feishu.cn/app?lang=zh-CN)

当前平台里的关键路径可以理解成：

1. 打开开发者后台
2. 创建企业自建应用
3. 进入应用的“凭证与基础信息”页
4. 在“应用凭证”区域查看 `App ID` 和 `App Secret`

对应到本项目配置里：

- 平台里的 `App ID` -> 配置里的 `appId`
- 平台里的 `App Secret` -> 配置里的 `appSecret`

为了让新人少踩坑，下面默认先走最简单的路径：

- 先测飞书单聊
- 先不配群聊隔离策略
- 先把链路跑通，再收紧权限

## 1. 先准备好这些东西

- 一台能运行本项目的机器
- Go `1.25+`
- 已安装并登录好的 `codex`，或者已安装并登录好的 `claude`
- 应用已经具备“收文本消息、发文本消息”的基本能力
- 你的本地项目绝对路径

这篇文档只覆盖“如何接入本项目”。
飞书平台侧如何创建应用、开权限、发布版本，不在这里展开。

## 2. 先去飞书开放平台拿 `appId` 和 `appSecret`

最直接的入口：

- [https://open.feishu.cn/app?lang=zh-CN](https://open.feishu.cn/app?lang=zh-CN)

按当前后台页面，推荐你这样走：

### 2.1 打开开发者后台

进入飞书开放平台首页后，右上角就有：

- `开发者后台`

点进去后，会进入你的应用列表页。

### 2.2 创建企业自建应用

在当前后台页面上，可以直接看到按钮：

- `创建企业自建应用`

新人第一次接 `codecli-channels`，建议优先用企业自建应用，不要一开始就走更复杂的发布路径。

### 2.3 打开“凭证与基础信息”

应用创建完成后，进入该应用详情页。

我已经核对过当前后台页面，这一页的标题就是：

- `凭证与基础信息`

而页面里会有一个明确的分区：

- `应用凭证`

在这个分区下可以直接看到：

- `App ID`
- `App Secret`

### 2.4 把平台字段映射到本项目配置

在本项目里要这样填：

```json
{
  "channels": {
    "feishu-main": {
      "type": "feishu",
      "enabled": true,
      "options": {
        "appId": "这里填平台里的App ID",
        "appSecret": "这里填平台里的App Secret"
      }
    }
  }
}
```

也就是说：

- `App ID` -> `appId`
- `App Secret` -> `appSecret`

如果你后面在后台重置了 Secret，这里的 `appSecret` 也要一起更新。

## 3. 复制配置文件

在仓库根目录执行：

```bash
cp config/codecli-channels.example.json config/codecli-channels.json
```

## 4. 先写一个最小可用配置

第一次联调建议用下面这版：

```json
{
  "defaultTimezone": "Asia/Shanghai",
  "channels": {
    "feishu-main": {
      "type": "feishu",
      "enabled": true,
      "options": {
        "appId": "cli_xxx",
        "appSecret": "sec_xxx"
      }
    }
  },
  "bridge": {
    "enabled": true,
    "backend": "codex",
    "channelIds": ["feishu-main"],
    "allowAllTargets": true,
    "projects": {
      "my-project": {
        "path": "/绝对路径/你的项目",
        "description": "飞书第一次联调"
      }
    },
    "defaultProject": "my-project",
    "defaultRunMode": "write",
    "implicitMessageMode": "write",
    "maxReplyChars": 1500,
    "readOnlyCodexSandbox": "read-only",
    "writeCodexSandbox": "workspace-write"
  }
}
```

第一次联调先不写这些高级选项：

- `allowFrom`
- `shareSessionInChannel`
- `threadIsolation`
- `groupReplyAll`

原因很简单：

- 新人最容易被权限和群聊行为绊住
- 先测单聊时，这些都不是必须项

## 5. 启动服务

```bash
go run ./cmd/codecli-channels -config ./config/codecli-channels.json
```

或者：

```bash
./bin/codecli-channels -config ./config/codecli-channels.json
```

## 6. 第一次验证怎么做

飞书最推荐的首次验证方式是：

1. 先和机器人开一个单聊
2. 发送 `/help`
3. 再发一条普通消息，例如 `解释一下这个项目是做什么的`
4. 回复完成后再发 `/history`

为什么建议先单聊：

- 群聊还会涉及 `@`、线程、共享会话这些额外变量
- 单聊更容易把问题收敛到配置本身

## 7. 跑通以后，再考虑收紧权限

第一次跑通后，不建议长期保留 `allowAllTargets=true`。

如果你只想放行飞书单聊，可以改成：

```json
{
  "bridge": {
    "allowAllTargets": false,
    "allowedScopes": [
      "feishu-main:p2p:你的chat_id"
    ]
  }
}
```

注意这里最容易写错的点：

- `allowedScopes` 里放的是 `chat_id`
- `allowFrom` 里放的通常是用户标识，例如 `ou_xxx`

也就是说：

- `allowedScopes` 管的是“哪个会话能进来”
- `allowFrom` 管的是“哪个发送者能触发”

新人第一次接入时，建议先只收紧 `allowedScopes`，等跑稳后再加 `allowFrom`。

## 8. 如果你以后要接群聊

群聊时，最常见的几个选项是：

```json
{
  "channels": {
    "feishu-main": {
      "type": "feishu",
      "enabled": true,
      "options": {
        "appId": "cli_xxx",
        "appSecret": "sec_xxx",
        "shareSessionInChannel": false,
        "threadIsolation": true
      }
    }
  }
}
```

它们的直觉理解可以先记成：

- `shareSessionInChannel=false`
  - 不同人不要共用同一个群上下文
- `threadIsolation=true`
  - 群线程尽量彼此隔离

不过这些不是新人第一次跑通的必要条件。

## 9. 常见问题

### 9.1 根本不知道去哪里拿 `appId` / `appSecret`

直接进：

- [https://open.feishu.cn/app?lang=zh-CN](https://open.feishu.cn/app?lang=zh-CN)

然后走：

- `开发者后台`
- `创建企业自建应用`
- `凭证与基础信息`
- `应用凭证`

### 9.2 进程启动了，但飞书完全没回复

优先检查：

- `appId` / `appSecret` 是否正确
- 飞书应用是否真的能接收文本消息
- 你是不是在和正确的应用单聊

### 9.3 单聊能回，群里不回

优先检查：

- 你有没有 `@` 机器人
- 你是不是需要开启 `groupReplyAll`
- 你的群会话 scope 是否被放行

### 9.4 配了 `allowFrom` 之后突然不工作

这是因为 `allowFrom` 会过滤发送者。

如果你不确定自己的用户标识，建议先去掉 `allowFrom`，先保证链路跑通，再慢慢收紧。

### 9.5 想让不同线程各自独立

后面再加：

```json
{
  "channels": {
    "feishu-main": {
      "options": {
        "threadIsolation": true
      }
    }
  }
}
```

第一次联调时不建议一上来就打开所有行为开关。

## 10. 推荐的第一次联调命令

启动：

```bash
go run ./cmd/codecli-channels -config ./config/codecli-channels.json
```

飞书单聊里依次发送：

```text
/help
解释一下这个项目是做什么的
/history
```
