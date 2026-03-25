# 微信新人快速上手

这篇文档只讲一件事：

**怎么先拿到个人微信的 token，再把它接到 `codecli-channels`。**

如果你是第一次接这个项目，真正的关键不是配置字段，而是：

**你要先拿到微信侧可用的 token。**

在这个项目里，最推荐、也最省事的方式不是手工找 cookie 或抓包，而是直接用仓库内置命令：

- `weixin setup`

这个命令本身就是“获取 token + 回写配置”的正式入口。

## 1. 先准备好这些东西

- 一台能运行本项目的机器
- Go `1.25+`
- 已安装并登录好的 `codex`，或者已安装并登录好的 `claude`
- 手机微信，能够完成扫码确认
- 你的本地项目绝对路径
- 当前机器可以访问 `https://ilinkai.weixin.qq.com`

## 2. 先知道 token 应该怎么拿

在本项目里，推荐你只记住两种方式：

### 2.1 最推荐：直接扫码拿 token

```bash
go run ./cmd/codecli-channels weixin setup -config ./config/codecli-channels.json
```

这是新人第一次最应该用的方式，因为它会自动完成：

1. 申请二维码
2. 打印二维码
3. 等你扫码确认
4. 写入 token
5. 顺手补齐 bridge 里需要的微信配置

### 2.2 备选：你已经有 token，再手工绑定

```bash
go run ./cmd/codecli-channels weixin bind -config ./config/codecli-channels.json -token '<你的token>'
```

这个适合已经有人把 token 给你，或者你之前已经拿到过 token 的情况。

所以对新人来说，最重要的结论其实是：

- QQ 的关键是去平台拿 `appId` / `clientSecret`
- 飞书的关键是去后台拿 `App ID` / `App Secret`
- 微信的关键就是先跑 `weixin setup`

## 3. 复制配置文件

在仓库根目录执行：

```bash
cp config/codecli-channels.example.json config/codecli-channels.json
```

## 4. 先准备一个最小骨架配置

第一次联调时，你只需要先把项目和后端配好，微信 token 先不用手填：

```json
{
  "defaultTimezone": "Asia/Shanghai",
  "channels": {
    "weixin-main": {
      "type": "weixin",
      "enabled": false,
      "options": {}
    }
  },
  "bridge": {
    "enabled": true,
    "backend": "codex",
    "channelIds": [],
    "allowAllTargets": false,
    "allowedScopes": [],
    "projects": {
      "my-project": {
        "path": "/绝对路径/你的项目",
        "description": "微信第一次联调"
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

## 5. 用内建命令扫码接入

直接执行：

```bash
go run ./cmd/codecli-channels weixin setup -config ./config/codecli-channels.json
```

这个命令会自动做几件事：

1. 申请二维码
2. 在终端打印二维码
3. 等你扫码并在手机上确认
4. 把 token 和相关 bridge 配置回写到 `config/codecli-channels.json`

## 6. 扫码成功后，应该看到哪些配置被自动补齐

正常情况下，会自动写入这些内容：

- `channels.weixin-main.enabled = true`
- `channels.weixin-main.options.token`
- `channels.weixin-main.options.baseUrl`
- `channels.weixin-main.options.allowFrom`
- `bridge.channelIds` 里的 `weixin-main`
- `bridge.allowedScopes` 里的 `weixin-main:dm:<你的微信用户ID>`

如果这些字段都出现了，说明微信接入前半段已经成功。

## 7. 启动服务

```bash
go run ./cmd/codecli-channels -config ./config/codecli-channels.json
```

或者：

```bash
./bin/codecli-channels -config ./config/codecli-channels.json
```

## 8. 第一次验证一定按这个顺序

微信和 QQ 最大的不同，是回复时依赖运行期拿到的 `context_token`。

所以第一次联调建议严格按下面顺序：

1. 先完成 `weixin setup`
2. 再启动 bridge
3. 用扫码成功的那个微信账号，先发一条普通文字消息
4. 然后再发 `/help`
5. 再发一条真正的任务消息

为什么不能跳过第 3 步：

- 程序需要从入站消息里拿到 `context_token`
- 这个 token 会被本地缓存
- 没有它，后续回复无法发出去

## 9. 如果你已经有 token

如果你不是扫码，而是已经拿到一个可用 token，可以直接绑定：

```bash
go run ./cmd/codecli-channels weixin bind \
  -config ./config/codecli-channels.json \
  -token '<你的token>'
```

但要注意：

- `bind` 会校验 token 是否可用
- `bind` 会写入微信 channel 和 `bridge.channelIds`
- `bind` 不一定知道应该放行哪个微信用户

所以如果你走 `bind`，通常还要自己补一条：

```json
{
  "bridge": {
    "allowedScopes": [
      "weixin-main:dm:user@im.wechat"
    ]
  }
}
```

## 10. 微信侧怎么用最省心

新人第一次联调建议这样做：

- 只用一个微信账号测试
- 只发纯文本，不要上来就发图片或文件
- 先发 `/help`
- 再发简单普通消息
- 如果要改方向，用 `/stop`

## 11. 常见问题

### 11.1 根本不知道去哪里拿 token

对这个项目来说，最直接的答案就是：

```bash
go run ./cmd/codecli-channels weixin setup -config ./config/codecli-channels.json
```

不要先去尝试账号密码、cookie、网页抓包这些更绕的路径。

### 11.2 二维码总是过期

优先检查：

- 当前机器能不能访问 `https://ilinkai.weixin.qq.com`
- 扫码后有没有在手机上确认
- 网络是不是不稳定

最直接的处理方式是重新执行：

```bash
go run ./cmd/codecli-channels weixin setup -config ./config/codecli-channels.json
```

### 11.3 token 已经写进配置，但还是不回消息

优先检查：

- 你有没有先发过一条普通文字消息，让程序拿到 `context_token`
- `channels.weixin-main.enabled` 是否为 `true`
- `bridge.channelIds` 是否包含 `weixin-main`
- `bridge.allowedScopes` 是否包含你的微信用户
- `channels.weixin-main.options.allowFrom` 有没有把你挡掉

### 11.4 `weixin bind` 失败

优先检查：

- token 是不是真正可用的 ilink Bearer Token
- token 是否已经过期
- `baseUrl` 是否正确

### 11.5 发图片后没有正常处理

当前微信 driver 仍然以文本链路为主。

新人第一次联调时，请先只发文字。

## 12. 推荐的第一次联调命令

扫码：

```bash
go run ./cmd/codecli-channels weixin setup -config ./config/codecli-channels.json
```

启动：

```bash
go run ./cmd/codecli-channels -config ./config/codecli-channels.json
```

微信里依次发送：

```text
你好
/help
解释一下这个项目是做什么的
```
