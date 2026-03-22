# codecli-channels 全量重命名设计说明

## 背景

当前项目名为 `qq-codex-go`，项目定位和代码结构都明显偏向“QQ + Codex 的单平台桥接器”。随着后续规划扩展到更多聊天平台，这个名字会同时限制产品表达和代码演进空间：

- 对外名字过于绑定 QQ
- Go module、命令目录和二进制名都带有 QQ
- 运行时目录、配置示例、日志和文档都在强化“单平台产品”印象

本次迭代目标是把项目身份统一重命名为 `codecli-channels`，并在不打断现有 QQ 能力的前提下，先完成“产品身份去 QQ 化”。

## 目标

- 将项目对外身份统一为 `codecli-channels`
- 将 Go module、命令入口、构建产物、运行时目录与文档统一到新名字
- 让产品叙事从“QQ 专用 bridge”变成“channels 产品，当前先支持 QQ”
- 在配置层增加更中性的通用字段，为未来多平台铺路

## 非目标

- 不在本次迭代中完成完整多平台架构抽象
- 不迁移 `internal/qq` 到全新目录结构
- 不改动现有 QQ 网关和 API 调用行为
- 不新增 Telegram/Discord 等平台实现

## 命名策略

### 1. 项目身份全量改名

以下内容统一改成 `codecli-channels`：

- `go.mod` module 名
- `cmd/codecli-channels` 目录名
- README 标题与文案中的项目名
- 启动日志与客户端标识
- LICENSE 中的项目名
- 运行时临时目录名
- 示例配置中的默认项目 alias 与路径示例

### 2. 平台实现当时暂不强拆

本设计文档形成时，QQ 实现仍然保留在 `internal/qq/` 中，继续作为首个 channel 平台实现存在。后续多平台内核重构已经将其迁移到 `internal/channel/qq/`；这里保留的是当时的重命名决策背景。

### 3. 配置层轻量去 QQ 化

在公共配置层增加更中性的命名：

- 新增 `maxReplyChars`
- 保留 `qqMaxReplyChars` 作为兼容字段
- 规范文档中的表述，改为“当前 channel 为 QQ”

这样后续扩平台时，不必继续把 QQ 写死在公共配置结构里。

## 文案策略

对外文案统一表达为：

- `codecli-channels` 是一个 channels 产品
- 当前已实现 QQ channel
- 后续可扩展到其他聊天平台

避免再把整个产品定义成“QQ bridge”，但在需要明确当前能力范围的地方，保留“QQ channel”描述。

## 影响范围

### 代码

- `go.mod`
- `cmd/codecli-channels/main.go`
- 所有 Go import path
- 运行时目录命名
- 启动日志

### 配置

- `config/qqbot.example.json` 的项目示例与说明
- 视情况新增 `config/codecli-channels.example.json`
- 默认配置文件路径改为 `config/codecli-channels.json`

### 文档

- `README.md`
- `README.zh-CN.md`
- `CONTRIBUTING.md`
- `docs/architecture*.md`
- `docs/configuration*.md`
- `docs/troubleshooting*.md`
- 本轮新建的设计与计划文档

## 验收标准

- 仓库内项目身份统一为 `codecli-channels`
- `go test ./...` 通过
- 默认启动命令与 README 示例使用新名字
- 配置层支持 `maxReplyChars`，并继续兼容旧的 `qqMaxReplyChars`
- 文档明确表达“当前支持 QQ，后续支持多平台”
