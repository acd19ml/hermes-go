# Hermes-Go 演化式复现计划

> 目标：用 Go 复现 Hermes Agent 的关键架构演化，而不是照着最终形态一次性重构。每个 PR 都必须回答：这个抽象为什么现在才出现？它解决了上一个 PR 中已经真实暴露的问题吗？

本路线基于 `Hermes-Wiki` 重新规划，重点参考：

- [Agent Loop and Prompt Assembly](../Hermes-Wiki/concepts/agent-loop-and-prompt-assembly.md)
- [Tool Registry](../Hermes-Wiki/concepts/tool-registry-architecture.md)、[Model Tools](../Hermes-Wiki/concepts/model-tools-dispatch.md)、[Toolsets](../Hermes-Wiki/concepts/toolsets-system.md)
- [Prompt Builder](../Hermes-Wiki/concepts/prompt-builder-architecture.md)、[Prompt Caching](../Hermes-Wiki/concepts/prompt-caching-optimization.md)
- [Provider Transport](../Hermes-Wiki/concepts/provider-transport-architecture.md)、[ProviderProfile](../Hermes-Wiki/concepts/provider-plugin-system.md)
- [Context Compressor](../Hermes-Wiki/concepts/context-compressor-architecture.md)、[Large Tool Result Handling](../Hermes-Wiki/concepts/large-tool-result-handling.md)
- [Memory](../Hermes-Wiki/concepts/memory-system-architecture.md)、[Skills](../Hermes-Wiki/concepts/skills-system-architecture.md)、[Session Search](../Hermes-Wiki/concepts/session-search-and-sessiondb.md)
- [Security Defense](../Hermes-Wiki/concepts/security-defense-system.md)、[Tool Loop Guardrails](../Hermes-Wiki/concepts/tool-loop-guardrails.md)、[Checkpoints](../Hermes-Wiki/concepts/checkpoints-architecture.md)
- [Parallel Tools](../Hermes-Wiki/concepts/parallel-tool-execution.md)、[MCP and Plugins](../Hermes-Wiki/concepts/mcp-and-plugins.md)、[Hooks](../Hermes-Wiki/concepts/hook-system-architecture.md)
- [CLI](../Hermes-Wiki/concepts/cli-architecture.md)、[Profiles](../Hermes-Wiki/concepts/configuration-and-profiles.md)、[Worktree](../Hermes-Wiki/concepts/worktree-isolation.md)
- [Web Tools](../Hermes-Wiki/concepts/web-tools-architecture.md)、[Browser Tool](../Hermes-Wiki/concepts/browser-tool-architecture.md)、[Terminal Backends](../Hermes-Wiki/concepts/terminal-backends.md)、[execute_code](../Hermes-Wiki/concepts/code-execution-sandbox.md)
- [Multi Agent](../Hermes-Wiki/concepts/multi-agent-architecture.md)、[Goal / Ralph Loop](../Hermes-Wiki/concepts/goal-and-ralph-loop.md)、[Kanban](../Hermes-Wiki/concepts/kanban-multi-agent-board.md)

---

## 0. 不变量

这些规则优先级高于任何阶段表。

| 规则 | 含义 |
|---|---|
| 端到端先于抽象 | 先写硬编码、switch、具体类型；等重复和痛点出现后再重构 |
| 两个生产实现之后才提 interface | 测试替身不算架构理由；Provider、Tool、ContextEngine、MemoryProvider 都遵守 |
| 第三个方向之后才插件化 | Registry 可以在第二个工具后出现；插件发现必须等本地 registry 已经稳定 |
| System prompt 会话内 byte-static | 首轮构建并缓存；当前会话内 memory/skills/context 变化不反映到 system prompt |
| 工具结果必须可审计 | 每个 tool call 都有 role/tool_call_id/result；失败也回写结构化结果 |
| 安全默认保守 | 未知工具串行、未知 URL 拒绝、未知 provider 不静默切换、危险内容先拦截或提示 |
| PR 可回滚 | 写文件/patch/危险 terminal 之前要有 checkpoint；早期没有 checkpoint 时禁止危险工具 |

---

## 1. Issue -> PR 工作流

每个 PR 前只写一个 issue 文件。Issue 同时承担需求、边界和轻量设计约束；PR 只负责实现该 issue。不要把 requirement/design/CURRENT 拆成多个文件，拆开后维护成本高，也容易让 agent 选择性阅读。

```text
docs/issues/NNNN-short-name.md
```

### 1.1 Issue 模板

```markdown
# Issue NNNN: <一句话>

## Goal
- <本 PR 完成什么>

## User Story
- 谁会用？
- 没有这个功能时哪个 flow 会断？

## Scope
Allowed:
- <允许改什么>

Not allowed:
- <主动放弃 1>
- <主动放弃 2>
- <主动放弃 3>

## GitHub Labels
- `phase-N`
- `<capability>`
- `learning-pr`

## Design Notes
- 我读过的 Hermes-Wiki 页面：`Hermes-Wiki/concepts/...md`
- 当前代码里的真实痛点：<不是猜测，是上一版已经出现的问题>
- 决策：<做什么，为什么现在做，代价是什么>
- Go 化映射：<Python 机制如何映射到 Go>
- 暂不引入：<明确不做的抽象及原因>

## Acceptance Criteria
- [ ] `go test ./...` 通过
- [ ] `go run ./cmd/hermes-go ...` 输出：<粘贴预期>
- [ ] 至少 1 个失败路径测试

## Open Question
- <至少 1 条。若未解决，agent 不许写代码>

## PR Instruction
- 先读本 issue，只输出 diff 计划，不写代码。
```

### 1.2 Coding Agent 协议

```text
你是 Hermes-Go 学习复现助手。每次只做一个极小 PR。

硬性禁止：
1. 未读本 PR 对应的 docs/issues/NNNN-*.md 不写代码。
2. 单 PR 最多改 3 个生产文件；新增生产 LOC 默认 <= 200。
3. 不引入第三方库，除非 issue 明确批准。
4. 不做 unrelated cleanup。
5. 不提前提 interface；只有 >=2 个生产实现或 issue 明确说明例外才可以。
6. 不吞 error，不用 panic 处理可恢复错误，不返回裸 nil。
7. 不改 system prompt byte-static 不变量，除非 PR 主题就是 prompt/cache。

流程：
1. 复述需求，一句话。
2. 输出 diff 计划，列文件和行为，不写代码。
3. 用户说 go 后才实现。
4. 每个新行为有测试，至少包含失败路径。
5. 输出验证命令和实际结果。

遇到歧义：
- 优先问一个最关键问题。
- Issue 里的 Open Question 未解决时停下。
```

---

## 2. PR 路线总览

| 阶段 | 主题 | 主要 Wiki 锚点 | 抽象纪律 |
|---|---|---|---|
| Phase 0 | 最小可运行 CLI + 消息结构 | CLI、Agent Loop | 无 Provider interface |
| Phase 1 | 单轮 LLM 调用 | Provider Transport | 先具体 Anthropic，再提最小 client seam |
| Phase 2 | 单工具循环 | Agent Loop、Model Tools | 先硬编码一个工具 |
| Phase 3 | 第二工具后重构 Registry / Toolsets | Tool Registry、Toolsets | 第二工具出现后再 registry |
| Phase 4 | Prompt Builder + byte-static cache | Prompt Builder、Prompt Caching | 先字符串，后 parts |
| Phase 5 | 文件/终端能力 + 安全底座 | Security、Checkpoint、Large Results | mutating tool 必须有回滚/校验 |
| Phase 6 | 第二 API 路径后 Transport | Provider Transport、ProviderProfile | 两条 API mode 后再 transport |
| Phase 7 | 上下文压缩 | Context Compressor | 先截断，第二策略前再 ContextEngine |
| Phase 8 | Memory / Session Search / Skills | Memory、Skills | 先内置，再 provider/plugin |
| Phase 9 | Web / Browser / execute_code | Web、Browser、Sandbox | 先本地/单后端，再 provider registry |
| Phase 10 | Profiles / Worktree / Gateway | Profiles、Worktree、Gateway | 路径隔离先于多平台 |
| Phase 11 | Multi-agent / Goal / Kanban | Multi Agent、Goal、Kanban | delegate 先于持久 Kanban |
| Phase 12 | MCP / Plugins / Hooks | MCP、Hooks | 核心稳定后再外部扩展 |

---

## 3. Mandatory Path

### Phase 0: Bootstrap，不做 Provider 抽象

目标：让 Hermes-Go 有可运行入口、稳定消息类型和可测试的单轮假响应。

| PR | 内容 | 验收 | 不做 |
|---|---|---|---|
| 0.1 | `go mod init`、`cmd/hermes-go`、最小 config/env 读取、版本输出 | `go run ./cmd/hermes-go --version` | 不用 viper，不建 agent 框架 |
| 0.2 | 定义内部 `Message` / `ToolCall` / `ToolResult` struct，覆盖 JSON round-trip 测试 | `go test ./internal/agent` | 不接 API，不定义 Provider |
| 0.3 | concrete `StaticResponder`，CLI `--msg` 打通 user -> assistant | `go run ./cmd/hermes-go --msg hi` | 不支持 system prompt，不支持工具 |

完成门槛：只有具体类型和函数，没有 exported interface。

### Phase 1: 第一次真实 LLM，无工具

目标：先把真实 Anthropic Messages API 跑通，再抽最小 seam 给 Agent。

| PR | 内容 | 验收 | 抽象纪律 |
|---|---|---|---|
| 1.1 | `AnthropicClient` 直接构造 request/response，只支持 user/assistant text | 真实 env 下 `--msg` 返回模型文本；无 key 时测试错误 | 不提 Transport |
| 1.2 | `AIAgent.RunOnce(ctx, userMsg)`，内部仍只做单轮 | Agent 单元测试覆盖 success/error | 可以引入未导出的最小 `chatClient` interface，因为已有 Static + Anthropic 两个生产路径 |
| 1.3 | 一个硬编码 system prompt 字符串，首轮插入 messages | system prompt 改变模型行为 | 不拆 PromptBuilder |
| 1.4 | iteration budget 数据结构，但 loop 仍只跑一轮 | budget exhausted 有测试 | 不做自动继续 |

Wiki 对齐点：Hermes 的 `run_conversation` 是同步循环，内部消息遵循 OpenAI role 形状；但 Go 版此阶段只复现一轮。

### Phase 2: 单工具调用循环

目标：复现 Hermes 最小 agent loop：LLM -> tool_call -> tool result -> LLM。

| PR | 内容 | 验收 | 不做 |
|---|---|---|---|
| 2.1 | Anthropic tool-call wire parsing，转内部 `ToolCall` | fixture 测试 tool_use 解析 | 不执行工具 |
| 2.2 | 硬编码 `echo` 工具，dispatch 用 switch | 模型或 fixture 调 echo 后返回 tool result | 不建 Tool interface |
| 2.3 | `RunConversation` while loop：直到无 tool_call 或 budget 耗尽 | echo 两轮 fixture 测试 | 不并行、不 registry |
| 2.4 | tool result 完整性：tool_call_id 配对、孤儿 tool result 清理 | orphan pair 测试 | 不压缩 |
| 2.5 | warning-first tool loop guardrail：exact failure + idempotent no-progress | 重复失败追加 warning | 默认不 hard stop |

Wiki 对齐点：`Tool Loop Guardrails` 默认 warning-first；hard stop 会损害探索能力，后续只给 cron/batch 打开。

### Phase 3: 第二个工具出现后才 Registry

目标：让重复 dispatch 痛点先真实出现，再提 registry/toolsets。

| PR | 内容 | 验收 | 学习点 |
|---|---|---|---|
| 3.1 | 加 `read_file`，仍用 switch dispatch；路径限制在 cwd 下 | 读取 fixture 文件；路径逃逸失败 | 感受 switch 扩张 |
| 3.2 | 重构 `ToolEntry` + `Registry.Register/Dispatch/GetSchemas` | echo/read_file 行为不变 | registry 是对重复的修复，不是预设框架 |
| 3.3 | 统一工具错误 JSON：unknown、bad args、handler error | LLM 能收到结构化 error | 错误也要进入历史 |
| 3.4 | `Toolset`：`core`、`file`、`debugging`，支持 enable/disable | disabled tool 不出现在 schema | toolset 是分组，不是权限系统 |
| 3.5 | 动态 schema 调整：只暴露实际可用工具 | disabled/read_file schema 不泄漏 | 减少模型 hallucination |

Wiki 对齐点：Hermes registry 零外部依赖，工具文件依赖 registry，上层只查询 registry；Go 版先做手动注册，自动发现等插件期再做。

### Phase 4: Prompt Builder 与缓存不变量

目标：把 system prompt 从字符串演化为分层构建，但保持会话内 byte-static。

| PR | 内容 | 验收 | 不变量 |
|---|---|---|---|
| 4.1 | `PromptBuilder` parts：stable/context/volatile，但输出仍是一条 system message | parts 顺序测试 | 不做多 system messages |
| 4.2 | SOUL.md identity 槽位；不存在用默认 identity | fixture home 测试 | SOUL 独立于项目上下文 |
| 4.3 | 项目上下文 first-match：`.hermes.md/HERMES.md -> AGENTS.md -> CLAUDE.md -> .cursorrules` | 同目录多文件只加载一个 | 不把它们全拼进去 |
| 4.4 | 上下文文件注入扫描：ignore instructions、secret exfil、不可见 Unicode | malicious fixture 被 BLOCKED | fail closed |
| 4.5 | `AIAgent` 缓存 system prompt；memory/skill 变化不重建 | 同 session 两轮 hash 一致 | 压缩前不 invalidate |
| 4.6 | 平台提示 registry：cli/cron/sms 先做 3 个 | 平台不同 prompt 不同 | 不接 gateway |

Wiki 对齐点：Hermes 把稳定内容放前、易变内容放后，并且会话内缓存 `_cached_system_prompt`；这是为 prefix cache 和可复现行为服务，不只是性能优化。

### Phase 5: 文件、终端与安全底座

目标：在扩大工具能力前，先具备结果预算、写后校验、checkpoint 和基础安全防线。

| PR | 内容 | 验收 | 安全门槛 |
|---|---|---|---|
| 5.1 | large result persistence：单结果超阈值写 `.hermes-go/results/`，历史只留 preview | 100KB fixture 被持久化 | `read_file` 阈值 pin 住，避免循环 |
| 5.2 | `write_file` 只允许 cwd 内，原子写入 temp + rename | 写入成功/路径逃逸失败 | 无 checkpoint 前只允许新建/覆盖 fixture |
| 5.3 | 写后 delta lint：JSON/YAML/Go 基础语法先做本地检查 | 坏 JSON 返回诊断 | 不接 LSP |
| 5.4 | file-mutation verifier footer：本轮失败写入在最终响应后追加提醒 | patch 失败不会声称成功 | 只读工具不计入 |
| 5.5 | checkpoint v0：mutating tool 前复制受影响文件到 `.hermes-go/checkpoints/` | rollback 测试恢复文件 | 先 per-project，v2 shared store 后做 |
| 5.6 | local terminal tool，只支持非交互命令、timeout、cwd | `go test` 可调用 fixture shell | 危险命令 deny，暂不 background |
| 5.7 | secret redaction 默认 ON：tool result 和 debug log 脱敏 | fake key 被遮蔽 | redactor 自测可禁用 |

Wiki 对齐点：Hermes 的 checkpoint 对 LLM 透明，不是 tool；写文件后有 delta lint/LSP/footer 三层校验。Go 版先做最小本地版，后续再演进到 shared shadow git。

### Phase 6: 第二条 API 路径后重构 Transport

目标：不要一开始就做 provider 架构。等 Anthropic + OpenAI 风格两个协议都存在，再抽 transport。

| PR | 内容 | 验收 | 抽象纪律 |
|---|---|---|---|
| 6.1 | `OpenAIChatClient` 具体实现，直接构造 chat completions request | fixture + 可选真实调用 | 暂时重复转换代码 |
| 6.2 | 对比 Anthropic/OpenAI 差异，重构 `Transport`：ConvertMessages/ConvertTools/BuildRequest/Normalize | 两个 client 行为不变 | 现在才有两条 API mode |
| 6.3 | `NormalizedResponse`：text/tool_calls/usage/finish_reason | 两 provider fixture 归一 | Agent loop 只看 normalized |
| 6.4 | provider-specific role：OpenAI developer/system 切换 | gpt/codex fixture | 不散落到 agent loop |
| 6.5 | `ProviderProfile` 数据类：auth/env/base_url/model defaults | OpenAI-compatible 第二个 provider 出现后再落地 | profile 纯数据，不持 client |

Wiki 对齐点：Transport 管数据路径，ProviderProfile 管 provider 身份/auth/quirks，两者正交。不要把 token refresh、retry、stream 生命周期塞进 profile。

### Phase 7: 上下文压缩

目标：先做最朴素策略，再演化到结构化摘要和 engine 插件点。

| PR | 内容 | 验收 | 不做 |
|---|---|---|---|
| 7.1 | 粗略 token estimator，含 tools/schema 成本估算 | 超阈值能预警 | 不调 LLM |
| 7.2 | 简单 sliding truncate，保护 system + 前 N + 后 N | 消息数下降且 tool pair 完整 | 不摘要 |
| 7.3 | Smart collapse：旧 tool result 按工具类型变一行摘要 | terminal/read_file/search_files fixture | 不丢命令和路径 |
| 7.4 | LLM summary compressor，输出 Goal/Completed Actions/Active State 等结构 | 压缩摘要含文件/命令/错误 | 不改 system prompt |
| 7.5 | 压缩失败 cooldown + 低收益防抖 | 连续低收益停止压缩 | 不无限重试 |
| 7.6 | `ContextEngine` interface，仅在 sliding + compressor 两策略都存在后提取 | engine 切换测试 | 同时只允许一个 active engine |

Wiki 对齐点：Hermes v3 压缩先本地去重/折叠，再 LLM 摘要；压缩不能破坏 tool_call/tool_result 配对，也不能让 system prompt 每轮漂移。

---

## 4. Capability Path

以下阶段按兴趣推进，但顺序不要乱。每个阶段仍按 Issue -> PR。

### Phase 8: Memory、Session Search、Skills

| PR | 内容 | Hermes 不变量 |
|---|---|---|
| 8.1 | `MemoryStore` 双文件 `MEMORY.md` / `USER.md`，`§` 分隔、字符上限 | 原子写入 + 文件锁 |
| 8.2 | memory 内容扫描：prompt injection、secret exfil、不可见 Unicode | 写入前拦截 |
| 8.3 | 冻结快照注入 system prompt | 当前会话写入下次才生效 |
| 8.4 | `session_search`：SQLite FTS，先只索引 role/content/tool_name | 过去对话不进 memory |
| 8.5 | `skills_list` / `skill_view` 渐进式披露 | system prompt 只放索引，不放全文 |
| 8.6 | `skill_manage` create/update/archive，agent-created skills 才能自动改 | pinned 写保护后续做 |
| 8.7 | Background review fork：只给 memory + skills toolset | 不污染主 session prompt cache |
| 8.8 | `MemoryProvider` interface：内置 + 一个外部 provider 后再提 | 至多一个外部 provider |

### Phase 9: Web、Browser、execute_code

| PR | 内容 | Hermes 约束 |
|---|---|---|
| 9.1 | `web_search` 单后端，统一 result shape | SSRF / URL secret scan |
| 9.2 | `web_extract` + LLM 内容压缩，小于阈值跳过 | >2M 拒绝 |
| 9.3 | Web provider registry：search/extract/crawl capability 分开 | 至少两个 backend 后再做 |
| 9.4 | Browser local CDP：navigate/snapshot/click/type | accessibility tree 优先 |
| 9.5 | BrowserProvider registry：cloud provider 显式配置才启用 | 不因 Firecrawl key 自动用云浏览器 |
| 9.6 | `execute_code` 沙箱：只允许 7 个工具，限制 calls/stdout/timeout | 中间结果不进 context |

### Phase 10: Profiles、Worktree、Gateway

| PR | 内容 | 关键点 |
|---|---|---|
| 10.1 | `HERMES_HOME` 单一来源，所有状态路径经 resolver | 禁止 import-time 缓存路径 |
| 10.2 | profile create/use/list，隔离 config/memory/sessions/skills | profile 切换要在启动最早发生 |
| 10.3 | worktree mode：`.worktrees/` 创建、`.worktreeinclude` 复制/symlink | 路径遍历防护 |
| 10.4 | slash command registry：CLI/Gateway 共享命令定义 | command 是数据，不散落 if/else |
| 10.5 | Gateway session key 与 platform hints | 平台隔离，PII 脱敏 |
| 10.6 | cron no-agent watchdog | 定时任务可跳过 LLM |

### Phase 11: Multi-agent、Goal、Kanban

| PR | 内容 | 约束 |
|---|---|---|
| 11.1 | `delegate_task` 单子代理，skip_context_files + skip_memory | 子代理工具不超过父代理 |
| 11.2 | 批量 delegate，最多 3 个，结果只回 summary/tool_trace | 中断传播 |
| 11.3 | MoA：多模型回答 + 聚合器 | API 调用数明确展示 |
| 11.4 | Background review 与 delegate 分离 | 自动触发不由 LLM 决定 |
| 11.5 | `/goal` post-turn judge，fail-open，续推是 user message | 不改 system prompt |
| 11.6 | `/subgoal`，要求具体证据 | 空泛完成声明不算 done |
| 11.7 | Kanban SQLite board，dispatcher tick，worker 心跳 | 持久化跨 session/restart |

### Phase 12: MCP、Plugins、Hooks

| PR | 内容 | 约束 |
|---|---|---|
| 12.1 | MCP stdio client，发现工具后注册到 registry | MCP 工具名 namespace |
| 12.2 | MCP SSE/http + stale-pipe retry | 初始 auth 失败不无限重试 |
| 12.3 | plugin manager：用户插件 + 项目插件 opt-in + pip entrypoint | 项目插件默认禁用 |
| 12.4 | `pre_tool_call` / `post_tool_call` hooks | pre 可 block，错误不传播 |
| 12.5 | `transform_tool_result` / `transform_llm_output` hooks | first non-None wins |
| 12.6 | plugin skill namespace `plugin:skill` | 不进 system prompt 索引 |
| 12.7 | provider/web/browser/context/memory 插件 facade | 每类遵守各自 interface 时机 |

---

## 5. Go 映射表

| Hermes/Python | Go 映射 | 禁忌 |
|---|---|---|
| dict message | `struct Message { Role, Content, ToolCalls, ToolCallID }` | 不用 `map[string]any` 贯穿业务层 |
| ABC/interface | 小 interface，只在两个生产实现后出现 | 不为“未来可能”建 interface |
| async tool/provider | 同步主循环 + goroutine/channel 只包并发工具 | 不把整个 agent loop 变 async |
| plugin self-register | init-time 注册可以用，但 registry 必须先稳定 | 不在 Phase 0 就做插件 |
| prompt parts | `PromptParts{Stable, Context, Volatile []string}` | 不把 parts 发成多条 system message |
| freeze snapshot | 首轮 clone 到 agent struct | 不从磁盘每轮重读 memory |
| large result storage | workspace-local `.hermes-go/results` 起步 | 不把 100KB+ 结果直接塞 history |
| checkpoint v2 | 先 per-project，再 shared bare git store | 不污染用户 `.git` |
| provider profile | dataclass 风格 struct + hooks | 不持 client/token/stream 状态 |
| terminal backend | interface 在 local + docker 两后端后提取 | 不一开始建 7 后端框架 |

---

## 6. PR Definition of Done

每个 PR 必须满足：

1. 有单个 issue 文件，并列出读过的 Hermes-Wiki 页面。
2. GitHub issue 已打标签：至少包含 `phase-N`、能力标签和 `learning-pr`。
3. PR 描述含“为什么现在引入/不引入这个抽象”。
4. `go test ./...` 通过。
5. 至少一个 CLI 或 fixture 演示。
6. 至少一个失败路径测试。
7. 没有 unrelated cleanup。
8. Reflection 不超过 150 字：我原本以为 X，但 Hermes-Wiki/实现让我看到 Y。

---

## 7. 常见反模式

| 反模式 | 立即处理 |
|---|---|
| Phase 0 定义 `Provider` / `Tool` 大接口 | 删除，退回具体类型 |
| PromptBuilder 每轮重建 system prompt | 修复为首轮缓存 |
| Memory 写入立刻改当前 system prompt | 改成冻结快照 |
| 工具失败只返回普通字符串 | 改结构化 JSON error |
| tool result 缺 tool_call_id | 不允许进入 history |
| web/browser 没有 SSRF 和 URL secret scan | 不合并 |
| write_file/patch 无 checkpoint 或 footer | 不合并 mutating tool |
| provider quirks 写进 agent loop | 移到 transport/profile |
| 插件技能自动进 system prompt | 改为显式 `plugin:skill` |
| hard stop guardrail 默认开启 | 改回 warning-first |

---

## 8. 第一个 PR 只做这些

```bash
mkdir -p hermes-go/cmd/hermes-go hermes-go/internal/agent hermes-go/docs/issues
cd hermes-go
go mod init github.com/<you>/hermes-go
```

然后手写：

```text
docs/issues/0001-bootstrap-cli.md
```

第一个 PR 的代码只允许做到：

```text
go run ./cmd/hermes-go --version
```

能打印版本即可。不要 Provider，不要 Tool，不要 PromptBuilder。
