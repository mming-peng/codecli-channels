# 多平台通道内核重构与飞书/个人微信接入设计

## 1. 背景

当前 `codecli-channels` 的主链路是：

```text
QQ Gateway -> bridge.Service -> store -> Codex/Claude runner
```

这个实现已经验证了“聊天端驱动本地 Code CLI”的核心价值，但它仍然保留了明显的单平台假设：

- `bridge.Service` 直接依赖 `qq.APIClient`、`qq.Gateway`、`qq.IncomingMessage`
- 配置模型里的 `accounts` 实际上等价于 QQ 凭据
- 会话 key、ACL、回复路径都隐含了 `QQ + user/group` 语义
- 新增平台必须修改 bridge 核心，而不是新增适配器

用户目标不是复刻 `cc-connect` 的总体架构，而是参考它在飞书、个人微信接入上的实现经验，设计一套更适合 `codecli-channels` 的、可继续扩展到多个平台、多个渠道、多个实例的架构。

## 2. 目标

### 2.1 本期目标

- 将现有“QQ 特化 bridge”重构为“领域内核 + 通道适配层”
- 保持现有 Codex / Claude backend 行为、命令集合、审批流的主体逻辑不变
- 为后续接入 Feishu、个人微信建立稳定扩展点
- 在配置层支持多个平台、多个渠道实例
- 在状态层支持不同平台的不同会话作用域
- 兼容现有 QQ 配置，避免破坏当前可用路径

### 2.2 非目标

- 本期不追求完整复刻 `cc-connect` 的 platform engine、card navigation、relay、cron 等体系
- 本期不要求飞书卡片、富媒体、微信 CDN 媒体链路一次到位
- 本期不引入过重的内部事件总线
- 本期不重写 Codex / Claude runner 抽象

## 3. 设计原则

1. **内核只处理领域问题**
   - 命令、会话、审批、任务编排、审计属于 bridge core
   - SDK、协议、长连接、轮询、reply token 属于 channel driver

2. **平台差异留在适配层**
   - 飞书线程隔离、微信群聊建联、微信 `context_token` 都不能泄漏到 bridge core

3. **扩展以实例为中心，而不是以平台名为中心**
   - 未来可能同时运行多个 QQ、多个飞书、多个微信实例
   - 核心标识应使用 `channel instance id`，而不是仅使用 `platform`

4. **抽象只服务已知扩展**
   - 当前只为 `QQ / Feishu / Weixin` 文本 MVP 抽象
   - 卡片、媒体、消息更新使用可选能力扩展，不进入第一版核心接口

5. **兼容优先**
   - 旧 `accounts + accountIds + allowedTargets` 配置应可自动归一化到新模型

## 4. 目标架构

```text
cmd/codecli-channels
  -> config.Load + Normalize
  -> channel.Registry
  -> bridge.CoreService
  -> backend runners
  -> start enabled channel drivers

bridge.CoreService
  -> command parsing
  -> session/project/backend resolution
  -> danger confirm + native approval
  -> task execution orchestration
  -> status/history rendering

channel.Registry
  -> create drivers from config
  -> keep enabled driver instances

channel.Driver
  -> Start(messageSink)
  -> Reply(outboundRef, content)
  -> Send(outboundRef, content)
  -> optional capability methods

store.Store
  -> core conversation/session mapping
  -> current project/backend/mode
  -> running task / approval state

channel private state
  -> driver-specific files under dataDir/channel-state/<channel-id>/
  -> e.g. weixin cursor / context_token
```

## 5. 领域模型

### 5.1 统一入站消息

新增通用消息结构 `channel.Message`，替代 `qq.IncomingMessage` 进入 bridge core。

建议字段：

```go
type Message struct {
    ChannelID   string
    Platform    string
    Scope       ConversationScope
    Sender      Sender
    MessageID   string
    Text        string
    Timestamp   string
    ReplyRef    any
    Metadata    map[string]string
}
```

说明：

- `ChannelID` 是配置中的实例别名，例如 `default`、`feishu-main`
- `Platform` 是协议类型，例如 `qq`、`feishu`、`weixin`
- `ReplyRef` 是 driver 私有回复句柄，core 不解析
- `Metadata` 用于放平台附加信息，例如飞书 `chat_id/root_id`、微信 `context_token`

### 5.2 会话作用域

新增 `ConversationScope`，由 driver 根据平台规则生成，bridge core 只消费其稳定 key 与作用域类型。

```go
type ConversationScope struct {
    Key  string
    Kind string
}
```

关键点：

- `Key` 是 core 的唯一主键，不再手写字符串拼接协议
- `Kind` 用于状态展示，例如 `dm`、`group`、`thread`
- 不同平台可以用不同策略构造 scope，但只要 key 稳定即可
- 原始 `chat_id`、`user_id`、`thread_id` 不再作为 core 的结构化字段出现，但允许被封装进 opaque `scope key`、`Message.Metadata` 或 driver 私有 `ReplyRef`

建议的 key 风格：

- QQ：`<channel-id>:group:<group_openid>` / `<channel-id>:user:<user_openid>`
- Feishu：`<channel-id>:p2p:<chat_id>`、`<channel-id>:group:<chat_id>`、`<channel-id>:group-user:<chat_id>:<open_id>`、`<channel-id>:group-thread:<chat_id>:<root_id>`
- Weixin：`weixin-main:dm:<peer_user_id>`

作用域规则必须固定下来，不能在每个平台实现里临时决定：

- QQ
  - 私聊：`<channel-id>:user:<user_openid>`
  - 群聊：`<channel-id>:group:<group_openid>`
- Feishu
  - 私聊默认：`<channel-id>:p2p:<chat_id>`
  - 群聊若 `threadIsolation=true` 且存在 root/thread：`<channel-id>:group-thread:<chat_id>:<root_id>`
  - 否则若 `shareSessionInChannel=true`：`<channel-id>:group:<chat_id>`
  - 否则默认按群内用户隔离：`<channel-id>:group-user:<chat_id>:<open_id>`
  - 冲突优先级：`threadIsolation` > `shareSessionInChannel` > 默认按用户隔离
- Weixin
  - 始终按单聊 peer：`<channel-id>:dm:<peer_user_id>`

### 5.3 回复目标

第一版统一使用入站消息携带的 `ReplyRef`，用于 bridge core 在**当前任务生命周期内**向 driver 回包或主动发消息。

核心要求：

- 它必须是 driver 可识别的 opaque value
- 它来自原始入站消息
- 第一版 core **不依赖** “由 scope 恢复 reply ref” 能力

这允许：

- QQ 继续按当前 target 路径回包
- 飞书按 `chat_id/root_id/message_id` 回复
- 微信按 `peer + context_token` 发送

第一版边界明确为：

- `Reply(ctx, ref, content)`：用于“回复当前消息”
- `Send(ctx, ref, content)`：用于“在同一任务生命周期内继续向同一作用域发后续消息”，例如进度消息、分段回复、审批提示
- `/status`、审批、进度消息、最终回复都发生在当前任务生命周期内，因此直接复用入站消息提供的 `ReplyRef`
- 重启后恢复主动发送、定时任务通知、跨任务 scope 反查发送能力不在本期范围
- 如果未来需要“仅凭 scope 发消息”，再增加可选 capability，例如 `ScopeRefResolver`

## 6. 核心接口

第一版保持接口极薄，只承载文本链路。

```go
type Driver interface {
    ID() string
    Platform() string
    Start(ctx context.Context, sink MessageSink) error
    Reply(ctx context.Context, ref any, content string) error
    Send(ctx context.Context, ref any, content string) error
    Stop(ctx context.Context) error
}

type MessageSink func(context.Context, Message)
```

第一版不强行抽出图片、卡片、消息更新接口；如果后续需要，可引入可选 capability：

- `ScopeRefResolver`：根据 scope 恢复主动消息上下文
- `FormattingHintProvider`：为不同平台提供格式提示
- `RichReplyDriver`：第二期再支持卡片、附件

## 7. 配置模型

### 7.1 新模型

保留 `bridge` 作为核心配置，新增 `channels` 作为多平台实例配置。

建议结构：

```json
{
  "channels": {
    "default": {
      "type": "qq",
      "enabled": true,
      "options": {
        "appId": "xxx",
        "clientSecret": "xxx"
      }
    },
    "feishu-main": {
      "type": "feishu",
      "enabled": false,
      "options": {
        "appId": "cli_xxx",
        "appSecret": "sec_xxx",
        "allowFrom": ["ou_xxx"],
        "shareSessionInChannel": false,
        "threadIsolation": true
      }
    },
    "weixin-main": {
      "type": "weixin",
      "enabled": false,
      "options": {
        "token": "xxx",
        "baseUrl": "https://ilinkai.weixin.qq.com",
        "allowFrom": ["user@im.wechat"]
      }
    }
  },
  "bridge": {
    "enabled": true,
    "backend": "codex",
    "channelIds": ["default"],
    "allowedScopes": ["default:user:YOUR_OPENID"],
    "...": "沿用现有通用 bridge 字段"
  }
}
```

### 7.2 兼容策略

为避免破坏现有用户，保留旧字段并在 `Normalize()` 中做转换：

- `accounts` -> 自动映射成 `type=qq` 的 `channels`
- `bridge.accountIds` -> 自动映射成 `bridge.channelIds`
- `bridge.allowedTargets` -> 自动映射成 `bridge.allowedScopes`
- `qqMaxReplyChars` 继续兼容，但落到通用 `maxReplyChars`

这样旧 QQ 用户无需立即迁移配置。

### 7.3 状态兼容策略

除了配置兼容，还必须保证已落盘状态兼容。

策略如下：

- 当 `channels` 由旧 `accounts` 自动归一化而来时，生成的 `channel alias` 必须等于原 `account id`
  - 例如旧配置 `default` 账号归一化后仍为 `channel id = default`
  - 这样原有 QQ 会话 key `default:user:u1`、`default:group:g1` 可以原样复用
- 因此，对“未手工改名的旧 QQ 用户”，`state.json` 不需要一次性迁移
- 只有在用户主动改用新的 channel alias 时，才视为开启了一个新的会话命名空间
- 文档和配置示例必须优先使用兼容 alias，而不是默认改成 `qq-default`

## 8. Store 与状态分层

### 8.1 核心状态

现有 `store.Store` 继续保存：

- `conversation -> project alias`
- `conversation -> backend`
- `active session`
- `session records`

但 `conversationKey` 的来源改为 `Message.Scope.Key`，而不是 `accountID:chatType:targetID`。

### 8.2 运行态

以下运行态仍由 bridge core 内存维护：

- busy
- running task
- pending confirmation
- native approval

这些状态的 map key 统一改用 `scope.Key`。

### 8.3 driver 私有状态

driver 私有协议状态不进入 `state.json`，避免污染 core 模型。

建议目录：

```text
<dataDir>/channel-state/<channel-id>/
```

示例：

- Weixin：
  - `get_updates.buf`
  - `context_tokens.json`
- Feishu：
  - 首期无需额外协议持久化文件

## 9. 准入与 ACL 分层

准入逻辑必须分层，避免分散在多个平台实现中失控。

### 9.1 driver 层负责

- 协议合法性校验
- 实例是否启用
- 该平台特有触发条件
  - 例如 Feishu 群里必须 `@bot`
  - 例如微信必须拿到可用 `context_token`
- 平台侧发送者白名单
  - 例如 `allowFrom`

driver 层拦截后，未通过的消息不进入 bridge core。

### 9.2 core 层负责

- `bridge.channelIds`：决定哪些 channel instance 参与 bridge 编排
- `bridge.allowAllTargets` / `bridge.allowedScopes`：决定哪些 conversation scope 允许进入执行
- 统一的命令解析、项目/会话/审批/任务编排

兼容映射：

- 旧 `allowAllTargets=true` 继续视为 core 层全放行
- 旧 `allowedTargets` 在归一化后写入 `allowedScopes`

这意味着：

- `allowFrom` 是“谁能和 bot 说话”
- `allowedScopes` 是“哪些聊天作用域允许执行 bridge 逻辑”
- 两者不是替代关系，而是两个层面的门禁

## 10. Bridge Core 重构方案

### 10.1 保留不变的部分

以下逻辑应尽量原样保留：

- `ParseBridgeCommand`
- `BuildHelpText`
- `status/history` 文本视图
- `codex / claude` runner 选择逻辑
- `dangerous task confirmation`
- `native approval` 状态机

### 10.2 必须拆分的部分

`internal/bridge/service.go` 当前是明显的 god service，应拆分为：

- `core_service.go`
  - 服务装配与启动
- `message_handler.go`
  - 统一入站处理
- `task_dispatch.go`
  - 任务执行与 runner 交互
- `approval.go`
  - 审批流
- `status.go`
  - `/status`、`/history`
- `session_commands.go`
  - `/project`、`/session`、`/backend`、`/mode`

第一期不强制彻底拆文件，但至少要先完成“依赖抽象化”：

- `Service` 不再持有 `qq.APIClient`
- `Service` 不再持有 `map[string]*qq.Gateway`
- `handleMessage` 入参改为 `channel.Message`
- `reply/proactive` 改为经 `Driver` 回包

## 11. QQ / Feishu / Weixin 的 driver 策略

### 11.1 QQ

QQ 已经作为第一批迁移对象完成迁移：

- QQ 实现已收口到 `internal/channel/qq`
- 保留当前 token、gateway、reply 逻辑
- 将协议事件映射为统一 `channel.Message`

### 11.2 Feishu

参考 `cc-connect` 的实现经验，但不沿用其总架构。

需要吸收的点：

- 官方机器人长连接模型适合独立 driver 循环
- 需要区分群聊、私聊、thread scope
- 群聊触发通常要同时考虑 `@bot` 与 `allowFrom`
- 回复上下文可由 `chat_id`、`root_id`、`message_id` 组成

第一期只做文本 MVP：

- 接收文本消息
- 群里 `@bot` 触发
- 私聊文本回复
- 审批、命令、进度消息打通

暂不进入：

- 卡片
- 图片文件
- 原地消息更新

### 11.3 Weixin

参考 `cc-connect` 的实现经验，但 driver 设计应更轻。

必须吸收的点：

- 独立 `getUpdates` 长轮询循环
- `context_token` 必须持久化
- 首次需要用户先发消息建联
- 协议错误与过期恢复需要 driver 自己处理

第一期只做文本 MVP：

- 长轮询收消息
- 文本回复
- `context_token` 缓存与恢复
- 审批、命令、进度消息打通

暂不进入：

- CDN 媒体上传下载
- 语音 / 文件链路

## 12. 实施顺序

### Phase 1: 通道内核抽象

- 新增 `internal/channel` 公共包
- 新增 registry / driver 接口 / message 模型
- 配置升级到 `channels`
- 旧配置自动归一化

### Phase 2: QQ 迁移

- QQ driver 接入 registry
- bridge core 改为只依赖通用接口
- 现有测试迁移到通用消息模型

### Phase 3: Feishu 文本 MVP

- 新增 Feishu driver
- 支持私聊、群 @、thread scope 配置

### Phase 4: Weixin 文本 MVP

- 新增 Weixin driver
- 支持长轮询、cursor、`context_token`

### Phase 5: 第二期能力

- Feishu 卡片 / 富消息
- Weixin 媒体
- 平台格式提示和 richer capability

## 13. 测试策略

### 13.1 单元测试

- `config.Normalize()` 对新旧配置的兼容测试
- `channel.Scope` / `allowedScopes` / key 生成测试
- `bridge.Service` 基于 fake driver 的命令、审批、会话测试
- Weixin 私有状态持久化测试

### 13.2 集成测试

- QQ 迁移后回归测试必须保持现有行为
- fake driver 场景下覆盖：
  - `/project`
  - `/session`
  - `/backend`
  - `/status`
  - `/approve` / `/deny`

### 13.3 平台 smoke test

- Feishu：文本入站、回复、群 @ 作用域
- Weixin：长轮询、首条建联、`context_token` 恢复

## 14. 风险与取舍

### 风险

- 现有 `service.go` 过大，重构时容易引入回归
- 旧配置兼容逻辑会增加 `config.Normalize()` 复杂度
- Weixin 的 driver 私有状态如果和 core state 混存，会使模型迅速失控

### 取舍

- 第一版优先建立清晰边界，不抢着做所有富能力
- 允许部分平台能力以 optional capability 形式晚一点抽象
- 不抄 `cc-connect` 的完整 engine，只吸收其 Feishu/Weixin driver 经验

## 15. 最终决策

采用“**领域内核 + 通道适配层**”方案：

- `bridge core` 稳定化
- `channel driver` 插件化
- `config` 与 `store key` 通用化
- `QQ` 先迁移为 driver
- `Feishu / Weixin` 基于 driver 模式接入文本 MVP

这条路径能在不引入过度设计的前提下，把当前项目从“单平台桥接”升级为“可扩展的多平台通道内核”。
