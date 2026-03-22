# Multi-Channel Core Refactor Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 将当前 QQ 特化桥接服务重构为可扩展的多平台通道内核，并保持现有 QQ 能力可用，为后续接入 Feishu / Weixin 建立稳定扩展点。

**Architecture:** 引入薄的 `channel.Driver` 抽象与 registry，将平台协议、回包路径、driver 私有状态留在适配层；bridge core 只消费统一 `channel.Message`、统一 scope key，并通过 driver 接口进行回复。配置层升级到 `channels` 模型，同时兼容旧 `accounts`/`accountIds`。

**Tech Stack:** Go 1.25、标准库、`gorilla/websocket`、现有 Codex/Claude runners、JSON 配置与本地文件状态持久化

---

### Task 1: 建立通道抽象与测试基座

**Files:**
- Create: `internal/channel/types.go`
- Create: `internal/channel/registry.go`
- Create: `internal/channel/fake_driver_test.go`
- Modify: `internal/store/store_test.go`

- [ ] **Step 1: 写失败测试，定义 registry 与通用消息/作用域最小行为**

```go
func TestRegistryCreatesDriverByID(t *testing.T) {
    reg := NewRegistry()
    reg.Register("fake", func(id string, cfg config.ChannelConfig, dir string) (Driver, error) {
        return &fakeDriver{id: id}, nil
    })
    driver, err := reg.Build("demo", config.ChannelConfig{Type: "fake"}, t.TempDir())
    if err != nil || driver.ID() != "demo" {
        t.Fatalf("unexpected driver: %#v err=%v", driver, err)
    }
}
```

- [ ] **Step 2: 运行测试，确认失败**

Run: `go test ./internal/channel ./internal/store -run 'TestRegistryCreatesDriverByID|TestConversationKey'`
Expected: FAIL，提示 `NewRegistry` / `ChannelConfig` / `Driver` 等符号不存在

- [ ] **Step 3: 写最小实现**

```go
type Driver interface {
    ID() string
    Platform() string
    Start(context.Context, MessageSink) error
    Reply(context.Context, any, string) error
    Send(context.Context, any, string) error
    Stop(context.Context) error
}
```

- [ ] **Step 4: 增加通用 scope key 测试并通过**

Run: `go test ./internal/channel ./internal/store`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/channel/types.go internal/channel/registry.go internal/channel/fake_driver_test.go internal/store/store_test.go
git commit -m "refactor: add channel registry and message model"
```

### Task 2: 升级配置模型并保持旧配置兼容

**Files:**
- Modify: `internal/config/config.go`
- Modify: `internal/config/config_test.go`
- Modify: `config/codecli-channels.example.json`
- Modify: `docs/configuration.md`
- Modify: `docs/configuration.zh-CN.md`

- [ ] **Step 1: 写失败测试，覆盖 `channels` 新模型与旧 `accounts` 自动归一化**

```go
func TestNormalizeLegacyAccountsIntoChannels(t *testing.T) {
    cfg := &Config{
        Accounts: map[string]Account{
            "default": {Enabled: true, AppID: "app", ClientSecret: "secret"},
        },
        Bridge: BridgeConfig{
            AccountIDs:     []string{"default"},
            AllowedTargets: []string{"default:user:u1"},
            Projects:       map[string]ProjectConfig{"demo": {Path: "."}},
            DefaultProject: "demo",
        },
    }
    if err := cfg.Normalize("."); err != nil {
        t.Fatal(err)
    }
    if _, ok := cfg.Channels["default"]; !ok {
        t.Fatal("expected legacy qq account to become channel")
    }
}
```

- [ ] **Step 2: 运行测试，确认失败**

Run: `go test ./internal/config -run 'TestNormalizeLegacyAccountsIntoChannels|TestNormalizeReplyCharSettings'`
Expected: FAIL，提示 `Channels` / `ChannelIDs` / `AllowedScopes` 缺失

- [ ] **Step 3: 写最小实现**

```go
type ChannelConfig struct {
    Alias   string         `json:"-"`
    Type    string         `json:"type"`
    Enabled bool           `json:"enabled"`
    Options map[string]any `json:"options"`
}
```

- [ ] **Step 4: 更新示例配置和文档**

Run: `go test ./internal/config`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/config/config.go internal/config/config_test.go config/codecli-channels.example.json docs/configuration.md docs/configuration.zh-CN.md
git commit -m "refactor: add channel-based configuration model"
```

### Task 3: 将 bridge core 改为依赖通用通道接口

**Files:**
- Modify: `internal/bridge/service.go`
- Modify: `internal/bridge/service_test.go`
- Modify: `internal/bridge/views.go`
- Modify: `internal/bridge/commands.go`
- Modify: `internal/store/store.go`
- Modify: `internal/store/store_test.go`

- [ ] **Step 1: 写失败测试，使用 fake driver 驱动 bridge 处理统一消息**

```go
func TestServiceHandlesGenericChannelMessage(t *testing.T) {
    msg := channel.Message{
        ChannelID: "qq-default",
        Platform:  "qq",
        Scope:     channel.ConversationScope{Key: "qq-default:user:u1", Kind: "dm"},
        Sender:    channel.Sender{ID: "u1"},
        Text:      "/status",
        ReplyRef:  fakeReplyRef("m1"),
    }
    // 断言 service 通过 driver 回复状态文本，而不是依赖 qq.IncomingMessage
}
```

- [ ] **Step 2: 运行测试，确认失败**

Run: `go test ./internal/bridge ./internal/store -run 'TestServiceHandlesGenericChannelMessage|TestBuildConversationStatusUsesSessionSummary'`
Expected: FAIL，提示 `qq.IncomingMessage` 仍然是入口类型

- [ ] **Step 3: 写最小实现**

```go
type Service struct {
    drivers map[string]channel.Driver
    // ...
}
```

- [ ] **Step 4: 将 `conversationKey` 全部切换到 `scope.Key`，并保持 status/history/approval 行为不变**

Run: `go test ./internal/bridge ./internal/store`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/bridge/service.go internal/bridge/service_test.go internal/bridge/views.go internal/bridge/commands.go internal/store/store.go internal/store/store_test.go
git commit -m "refactor: decouple bridge core from qq transport"
```

### Task 4: 迁移 QQ 为第一批 driver，保持现有行为

**Files:**
- Create: `internal/channel/qq/driver.go`
- Create: `internal/channel/qq/api.go`
- Create: `internal/channel/qq/gateway.go`
- Create: `internal/channel/qq/driver_test.go`
- Modify: `cmd/codecli-channels/main.go`
- Modify: `README.md`
- Modify: `README.zh-CN.md`
- Modify: `docs/architecture.md`
- Modify: `docs/architecture.zh-CN.md`

- [ ] **Step 1: 写失败测试，断言 QQ driver 能把协议事件转换为统一消息，并能回包**

```go
func TestQQDriverBuildsScopeKey(t *testing.T) {
    msg := incomingToMessage("qq-default", incomingUserMessage("u1", "/ping"))
    if msg.Scope.Key != "qq-default:user:u1" {
        t.Fatalf("unexpected scope key: %s", msg.Scope.Key)
    }
}
```

- [ ] **Step 2: 运行测试，确认失败**

Run: `go test ./internal/channel/qq`
Expected: FAIL，提示 `incomingToMessage` / `Driver` 不存在

- [ ] **Step 3: 迁移现有 QQ API/gateway 逻辑到新 driver 包**

```go
func (d *Driver) Start(ctx context.Context, sink channel.MessageSink) error {
    d.gateway = NewGateway(d.cfg, d.api, d.channelID, d.logger, func(ctx context.Context, msg IncomingMessage) {
        sink(ctx, toChannelMessage(d.channelID, msg))
    })
    d.gateway.Start(ctx)
    return nil
}
```

- [ ] **Step 4: 启动入口改为 registry 装配 driver，更新文档中的架构表述**

Run: `go test ./internal/channel/qq ./internal/bridge ./cmd/codecli-channels`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/channel/qq/driver.go internal/channel/qq/api.go internal/channel/qq/gateway.go internal/channel/qq/driver_test.go cmd/codecli-channels/main.go README.md README.zh-CN.md docs/architecture.md docs/architecture.zh-CN.md
git commit -m "refactor: migrate qq transport to channel driver"
```

### Task 5: 为 Feishu / Weixin 预留实例配置与 driver 骨架

**Files:**
- Create: `internal/channel/feishu/driver.go`
- Create: `internal/channel/weixin/driver.go`
- Create: `internal/channel/weixin/state.go`
- Create: `internal/channel/weixin/state_test.go`
- Modify: `docs/2026-03-22-multi-channel-core-design.md`
- Modify: `README.zh-CN.md`

- [ ] **Step 1: 写失败测试，覆盖 Weixin 私有状态目录与 cursor/token 持久化**

```go
func TestWeixinStatePersistsCursorAndContextToken(t *testing.T) {
    state := NewState(filepath.Join(t.TempDir(), "weixin-main"))
    require.NoError(t, state.SaveCursor("cursor-1"))
    require.NoError(t, state.SaveContextToken("user@im.wechat", "ctx-1"))
}
```

- [ ] **Step 2: 运行测试，确认失败**

Run: `go test ./internal/channel/weixin -run TestWeixinStatePersistsCursorAndContextToken`
Expected: FAIL，提示状态对象不存在

- [ ] **Step 3: 写最小实现，并为 Feishu / Weixin 注册占位 driver**

```go
func NewDriver(id string, cfg config.ChannelConfig, dataDir string) (channel.Driver, error) {
    return &Driver{id: id, cfg: cfg, dataDir: dataDir}, nil
}
```

- [ ] **Step 4: 文档声明当前阶段已完成架构接入点，文本协议链路将按 driver 独立推进**

Run: `go test ./internal/channel/weixin ./internal/channel/...`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/channel/feishu/driver.go internal/channel/weixin/driver.go internal/channel/weixin/state.go internal/channel/weixin/state_test.go docs/2026-03-22-multi-channel-core-design.md README.zh-CN.md
git commit -m "feat: add feishu and weixin driver scaffolds"
```

### Task 6: 全量回归与收尾

**Files:**
- Modify: `docs/troubleshooting.md`
- Modify: `docs/troubleshooting.zh-CN.md`
- Modify: `docs/README.md`
- Modify: `docs/README.zh-CN.md`

- [ ] **Step 1: 跑全量测试**

Run: `go test ./...`
Expected: PASS

- [ ] **Step 2: 手工检查示例配置与 README 描述是否仍然匹配当前能力**

Run: `rg -n "QQ 会话|accountIds|allowedTargets|qqMaxReplyChars" README*.md docs/ config/ internal/`
Expected: 仅剩兼容说明或明确的 legacy 注释

- [ ] **Step 3: 更新排障与文档入口**

```text
明确说明：
- 当前核心已是多平台架构
- QQ 已迁移为 driver
- Feishu / Weixin 已进入项目结构并具备接入位点
```

- [ ] **Step 4: 再次跑测试确认**

Run: `go test ./...`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add docs/troubleshooting.md docs/troubleshooting.zh-CN.md docs/README.md docs/README.zh-CN.md
git commit -m "docs: document multi-channel core refactor"
```
