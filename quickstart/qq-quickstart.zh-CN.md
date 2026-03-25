# QQ 新人快速上手

这篇文档只讲一件事：

**怎么先拿到 QQ 机器人的 `appId` / `clientSecret`，再把它接到 `codecli-channels`。**

如果你是第一次接这个项目，最关键的不是“配置文件怎么写”，而是：

**你要先知道去哪里创建 QQ 机器人，去哪里拿到机器人 ID 和 Secret。**

按当前官方入口，最直接的页面是：

- [QQ 机器人快速创建入口](https://q.qq.com/qqbot/openclaw/login.html)

我根据这个官方页面当前展示的四步来整理：

1. QQ 账号登录
2. 创建 QQ 机器人
3. 配置接入密钥
4. 打开 QQ 开始对话

其中第 3 步里提到的“机器人的 ID 和 Secret”，在本项目配置里对应填写为：

- `channels.<你的channel>.options.appId`
- `channels.<你的channel>.options.clientSecret`

这一步是根据官方页面“ID / Secret”的表述，与本项目现有配置字段做的对应整理。

## 1. 先准备好这些东西

- 一台能运行本项目的机器
- Go `1.25+`
- 已安装并登录好的 `codex`，或者已安装并登录好的 `claude`
- 你想通过聊天操作的本地项目路径

如果你现在只想先跑通，建议先验证 QQ 私聊，不要一上来就测群聊。

## 2. 先去官方页面拿 `appId` 和 `clientSecret`

打开：

- [https://q.qq.com/qqbot/openclaw/login.html](https://q.qq.com/qqbot/openclaw/login.html)

你会看到官方页面标题是“快速注册创建QQ机器人”，页面上会直接写出四个步骤：

1. `QQ账号登录`
2. `创建QQ机器人`
3. `配置接入密钥`
4. `打开QQ开始对话`

新人第一次最推荐的走法，就是严格按这四步来，不要先从 `q.qq.com` 大首页里自己摸入口。

### 2.1 登录

在这个页面上：

- 可以直接用 QQ 扫码登录
- 登录过程也会完成开发者账号注册

如果你之前没有 QQ 开放平台账号，这个入口页本身就是最省事的起点。

### 2.2 创建 QQ 机器人

登录后，按页面引导创建你的 QQ 机器人。

这一步通常至少会涉及：

- 机器人名称
- 机器人头像

如果你只是第一次联调，名字随便起一个能认出的就行，不要在这一步纠结品牌包装。

### 2.3 配置接入密钥

这是最关键的一步。

官方页面这里写的是：

- 机器人的 `ID`
- 机器人的 `Secret`

回到本项目里时，你要这样填：

```json
{
  "channels": {
    "default": {
      "type": "qq",
      "enabled": true,
      "options": {
        "appId": "这里填机器人ID",
        "clientSecret": "这里填机器人Secret"
      }
    }
  }
}
```

也就是说：

- 页面里的 `ID` -> 配置里的 `appId`
- 页面里的 `Secret` -> 配置里的 `clientSecret`

如果你后面在平台里重置了 Secret，这里的 `clientSecret` 也要一起更新。

## 3. 复制配置文件

在仓库根目录执行：

```bash
cp config/codecli-channels.example.json config/codecli-channels.json
```

## 4. 先写一个最小可用配置

第一次跑通建议这样配：

```json
{
  "defaultTimezone": "Asia/Shanghai",
  "channels": {
    "default": {
      "type": "qq",
      "enabled": true,
      "options": {
        "appId": "你的QQ机器人AppID",
        "clientSecret": "你的QQ机器人ClientSecret"
      }
    }
  },
  "bridge": {
    "enabled": true,
    "backend": "codex",
    "channelIds": ["default"],
    "allowAllTargets": true,
    "projects": {
      "my-project": {
        "path": "/绝对路径/你的项目",
        "description": "新人第一次联调"
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

这里最关键的是：

- `channels.default.options.appId`
- `channels.default.options.clientSecret`
- `bridge.channelIds`
- `bridge.projects.my-project.path`
- `bridge.allowAllTargets`

为什么第一次建议用 `allowAllTargets=true`：

- 这样不用一开始就纠结 `openid` / `group_openid`
- 先把链路跑通，后面再收紧权限

## 5. 启动服务

```bash
go run ./cmd/codecli-channels -config ./config/codecli-channels.json
```

如果你已经编译过二进制，也可以这样：

```bash
./bin/codecli-channels -config ./config/codecli-channels.json
```

## 6. 第一次验证怎么做

按这个顺序来：

1. 看日志里是否出现 `QQ 网关 READY`
2. 用你的 QQ 给机器人发私聊消息 `/help`
3. 再发一条普通消息，例如 `解释一下当前项目是做什么的`
4. 等回复完成后，再发 `/history`

如果这四步都正常，说明 QQ 接入已经跑通了。

## 7. 跑通以后，建议马上收紧权限

第一次联调完成后，不建议长期保留 `allowAllTargets=true`。

更稳妥的做法是改成：

```json
{
  "bridge": {
    "allowAllTargets": false,
    "allowedScopes": [
      "default:user:你的openid"
    ]
  }
}
```

如果你要放行群聊，则格式通常是：

```json
{
  "bridge": {
    "allowedScopes": [
      "default:group:你的group_openid"
    ]
  }
}
```

实操建议：

- 先临时保留 `allowAllTargets=true`
- 跑一条消息
- 从运行日志里确认目标 scope 或 targetId
- 再把 `allowedScopes` 写死
- 最后把 `allowAllTargets` 改回 `false`

## 8. QQ 侧怎么用最省心

新人第一次使用时，建议遵守这几个习惯：

- 先从私聊开始，不要一上来测群聊
- 先发 `/help`，确认命令入口正常
- 再发普通消息，不要一开始就试高风险命令
- 如果任务方向错了，直接发 `/stop`
- 想确认当前项目和会话，优先用 `/project current` 和 `/session current`

## 9. 常见问题

### 9.1 根本不知道去哪里拿 `appId` / `clientSecret`

直接用这个官方入口：

- [https://q.qq.com/qqbot/openclaw/login.html](https://q.qq.com/qqbot/openclaw/login.html)

不要先从开放平台大首页里自己找。

### 9.2 启动了但 QQ 完全没回复

优先检查：

- 日志里有没有 `QQ 网关 READY`
- `appId` / `clientSecret` 是否填对
- 进程是不是还活着
- 你发消息的账号是否真的打到了这个 bot

### 9.3 `/help` 能回，普通消息不工作

优先检查：

- `bridge.projects` 里有没有配置有效项目
- `defaultProject` 是否存在
- 本地 `codex` 或 `claude` 是否能正常运行

### 9.4 群里不回复

QQ 群联调时，建议先确认：

- 你是不是在 `@` 机器人
- 你放行的是不是 `group_openid`
- `allowedScopes` 是否写成了群作用域格式

### 9.5 想切到 Claude

有两种方式：

配置里直接改默认值：

```json
{
  "bridge": {
    "backend": "claude"
  }
}
```

或者启动后在聊天里发：

```text
/backend use claude
```

## 10. 推荐的第一次联调命令

启动：

```bash
go run ./cmd/codecli-channels -config ./config/codecli-channels.json
```

QQ 私聊里依次发送：

```text
/help
解释一下这个项目是做什么的
/history
```
