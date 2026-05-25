# Hermes-Go 复现计划：作者视角的学习路线与 Coding Agent 约束协议

> 目标：通过用 Go 从零复现 Hermes Agent，**重演原作者的设计演化过程**，把每个抽象的"为什么"内化掉。配合 coding agent 协作，但严格限制其行为，确保学习主导而非生产力主导。

---

## 0. 核心原则：演化式复现，不是自顶向下重构

| 错误姿势 | 正确姿势 |
|---------|---------|
| PR #2 一口气定义所有顶层 interface（Engine、Downloader、Parser...） | 第一个 interface 只在你**真的有两个实现**之后才提取 |
| 先搭框架，再填业务 | 先用最丑的硬编码完成端到端，再以"为什么要抽象"为驱动逐次重构 |
| 把 Hermes 当作设计完美的目标态 | 把 Hermes 当作 N 个增量决策叠加的结果，每个 PR 模拟一个决策点 |

**rule of three 松散版**：在出现第 2 个实现之前，**不要**提取接口；在出现第 3 个实现之前，**不要**做插件化。这条规则比任何 PR 模板都重要——它强迫你写一遍"丑代码"，再写一遍"重构 PR"，这两个 diff 之间的差就是你真正学到的东西。

---

## 1. 作者心法工作流：Issue → Design → PR

每一个新功能走三步，**强制分离思考阶段**。这是约束 coding agent 的核心：思考阶段不让它写代码。

### 1.1 需求 Issue（你自己写，禁止 agent 介入）

```markdown
# [需求] <一句话需求>

## 用户场景
- 谁会用？（CLI 用户 / 平台用户 / 子 agent）
- 没有这个功能时会怎样？（具体描述一个 broken flow）

## 验收标准（必须可执行）
- [ ] 给定 X 输入，运行 `go run main.go ...`，能看到 Y 输出
- [ ] 测试用例 `TestZZZ` 通过

## 不在本次范围内
- 明确列出 3 条**主动放弃**的功能，避免 scope creep
```

### 1.2 设计 Issue（你主导，可以请 agent 提建议但不写代码）

```markdown
# [设计] <对应需求 issue #N>

## 我看过的 Hermes 源码位置
- `agent/xxx.py:L123-L200`（说明你**真的读过原始实现**）

## 关键决策
- 决策 1: <做了什么选择，为什么>
- 决策 2: <做了什么选择，trade-off 是什么>

## Open question（必填，至少 1 条）
- 我还不确定的地方：______
  （这一条强迫你正视没想清楚的部分，而不是让 agent 替你做选择）

## Go 化映射
- Python 的 async/await → Go 的 ______
- Python 的 dict 自由传递 → Go 的 struct ______
- Python 的鸭子类型 → Go 的 interface（仅在 N≥2 时引入）

## 不引入的抽象
- 暂不做 interface，因为现在只有 1 个实现：______
```

### 1.3 实现 PR（agent 主导写代码，但受协议约束）

见第 2 节的约束协议。

---

## 2. Coding Agent 约束协议（直接复制粘贴给 agent）

```
你是协助我学习 Hermes 架构的 coding 助手。我从 Python 项目用 Go 复现，每次只做一件极小的事。请严格遵守以下协议：

【硬性禁止】
1. 不允许大范围重构。本次 PR 最多修改 3 个文件，新增 LOC 不超过 200。
2. 不允许引入新的 Go 模块/第三方库，除非我在 issue 里明确写了。
3. 不允许在本 PR 范围外做"顺手清理"——任何与本 PR 无关的改动都必须拒绝。
4. 不允许提取 interface，除非现在已经有 ≥2 个实现，或者我在 design issue 里明确要求。
5. 不允许使用 panic 处理可恢复错误；不允许吞掉 error；不允许返回裸 nil。

【强制流程】
Step 1: 先读 docs/issues/CURRENT.md 里的需求和设计 issue。如果没读到，停下来问我路径。
Step 2: 输出 diff 计划：列出要修改/新建的文件，每个文件要做什么改动（不要写代码），等我确认。
Step 3: 我说"go"之后，才写代码。代码必须满足：
   - 每个新函数都有对应的 _test.go 测试
   - 不依赖 main.go 也能 go test 通过
   - 注释只解释"为什么"，不解释"做什么"（除非是非显然的算法）
Step 4: 提供验证步骤：
   - go test ./... 期望输出
   - go run main.go ... 期望终端输出（粘贴出来）

【遇到歧义时】
- 优先 ASK，不要 GUESS。一次只问一个最关键的问题。
- 如果设计 issue 的 Open question 没解决，停下来提醒我先回答。

【输出格式】
每次回复结构必须是：
1. "我读到的需求是: ___"（一句话复述，确保对齐）
2. Diff 计划 / 代码 / 验证步骤（取决于当前 Step）
3. "下一步建议"（一句话）
不要超出。不要总结。不要展望未来 PR。
```

---

## 3. Hermes-Go 演化式 PR 路线

每个阶段对应 Hermes 演化史上一个关键决策点。**前 4 个阶段是必经之路**，后面可选。

### Phase 0: Bootstrap（让一个壳跑起来）

| PR | 内容 | 演示效果 | 学到什么 |
|---|---|---|---|
| 0.1 | 项目骨架（main.go、config.yaml 加载、logger） | `go run main.go` 打印 "hermes-go v0" + 当前配置 | Go 项目结构、viper/slog 习惯 |
| 0.2 | 定义 `Provider` 接口（**仅一个方法 `Complete(ctx, msgs) (string, error)`**） | 无可见效果 | 接口的"最小可用面"如何确定 |
| 0.3 | `MockProvider` 实现，返回硬编码字符串 | `go run main.go --msg hi` 打印 mock 响应 | 端到端先打通，不碰真 API |

**关键纪律**：Phase 0 结束时项目能跑、有 1 个真接口、1 个假实现。不要有第二个 provider，不要有 tool，不要有 system prompt。

### Phase 1: 第一次真正调用 LLM（无工具）

| PR | 内容 | 演示效果 |
|---|---|---|
| 1.1 | `AnthropicProvider` 实现 Provider 接口（直接调 messages API） | `go run main.go --msg "你好"` 收到真实模型回复 |
| 1.2 | `AIAgent.RunOnce(userMsg) string` —— 单轮对话，没有 loop | 同上，但走 Agent 入口 |
| 1.3 | 硬编码 system prompt（一个字符串常量），传给 provider | 同上，但模型行为受 system prompt 影响 |

**陷阱预警**：很多人这里就忍不住把 system prompt 抽象成 builder。**忍住**。等 Phase 4。

### Phase 2: 单工具调用循环（最小可用 agent）

| PR | 内容 | 演示效果 |
|---|---|---|
| 2.1 | 定义 `Tool` 接口（`Name() string; Schema() ToolSchema; Execute(args) Result`） | 无 |
| 2.2 | 硬编码一个 `EchoTool`（参数 text，返回 text） | `go run main.go --msg "echo hello"`，模型调用 EchoTool 后返回 "hello" |
| 2.3 | Agent 主循环：while 有 tool_call → 执行 → 把 result 追加进 messages → 再调 LLM | 多轮对话能跑通 |
| 2.4 | 消息格式规范化（user/assistant/tool 三种 role 的 struct） | 内部清晰 |

**对应 Hermes 文件**：`run_agent.py` 里的 `while api_call_count < max_iterations` 循环（参见 wiki agent-loop 章节）。

### Phase 3: 工具注册表（首次重构）

到这里你应该已经有强烈冲动想加第二个工具。**先做注册表 PR，再加工具**——这样能真切感受"为什么需要 registry"。

| PR | 内容 | 学习焦点 |
|---|---|---|
| 3.1 | `Registry.Register(Tool)` / `Registry.Dispatch(name, args)` —— 但**只注册 EchoTool 一个** | Registry 在只有 1 个工具时**完全是过度设计**。这种"无意义感"本身就是学习——你会理解为什么作者要在加第二个工具之前先做这步 |
| 3.2 | 加 `ShellTool`（执行 shell 命令） | 此时 Registry 第一次"有用"，对比 3.1 之前的硬编码 |
| 3.3 | 加 `Toolset` 概念（一组 tool 的逻辑分组，可按名启/禁用） | Hermes 的 toolsets-system，对应 wiki toolsets-system 章节 |

### Phase 4: Prompt Builder 模块化

到这里 system prompt 已经累积了：identity、tool-use 强制指导、平台提示。**这是第二次重构的最佳时机**。

| PR | 内容 |
|---|---|
| 4.1 | `PromptBuilder` 把 system prompt 拆为有序的 parts（参考 wiki 列出的 13 层结构） |
| 4.2 | SOUL.md 文件加载（identity 槽位） |
| 4.3 | 项目上下文文件加载（`.hermes.md → AGENTS.md → CLAUDE.md` first-match-wins）|
| 4.4 | `PLATFORM_HINTS` 注册表（cli、telegram 等） |
| 4.5 | **缓存机制**：会话内 system prompt 只构建一次，存 `_cached_system_prompt` |

**关键学习点**：缓存不是性能优化，是为了让 Anthropic 的 prefix cache 命中率最大化。这种"软件设计被外部系统的特性反推"的现象，是高级 agent 工程的标志。

### Phase 5: 第二个 Provider（首次让 interface 名副其实）

| PR | 内容 |
|---|---|
| 5.1 | `OpenAIProvider`（chat completions API） |
| 5.2 | **此时回头重构 Provider 接口**——你会发现 Phase 0.2 定义的接口大概率不够用（reasoning content、role 切换 developer/system、tool call 格式都不同） |
| 5.3 | Provider transport 抽象层（对应 wiki provider-transport-architecture） |

### Phase 6: Context 管理（pluggable engine）

对应 Hermes 2026-04-10 update：从硬编码 ContextCompressor 重构为 ContextEngine ABC。

| PR | 内容 |
|---|---|
| 6.1 | Token counter（按 provider 不同，先简单按字符数估算） |
| 6.2 | 阈值触发的简单截断 engine |
| 6.3 | 定义 `ContextEngine` 接口（**只在你即将写第二个 engine 时**） |
| 6.4 | Summarization-based compressor engine |

### Phase 7+（可选，按兴趣推进）

- **Phase 7**: Auxiliary Client（辅助 LLM 路由器）—— 对应 wiki auxiliary-client-architecture
- **Phase 8**: Memory 系统（MEMORY.md 冻结快照、memory tool、provider plugin）
- **Phase 9**: Skills + Curator 状态机
- **Phase 10**: Multi-agent（delegate_task / mixture_of_agents）
- **Phase 11**: 安全防御（prompt injection patterns、redaction）
- **Phase 12**: Checkpoint v2、Fallback chain、Credential pool

---

## 4. Python → Go 映射陷阱（Hermes-specific）

| Python 习惯 | Go 对应 | 陷阱 |
|---|---|---|
| `dict` 自由传递（`message: dict`） | 强类型 struct，`Message{Role, Content, ToolCalls, Reasoning}` | 不要用 `map[string]any`，那是放弃 Go 类型系统 |
| Anthropic SDK 的 async stream | Goroutine + channel，返回 `<-chan Chunk` | 别忘了 ctx cancel 关闭 channel |
| Python 的鸭子类型（toolset 拿到任何 dict 都能 `.get('description')`） | interface + 编译期约束 | 在没有第二个实现前**不要**提取 interface |
| `**kwargs` 透传 | functional options 模式（`WithXxx(...)` 函数） | 不要用 `map[string]any` 模拟 kwargs |
| Python 的 OrderedDict LRU 缓存 | `container/list` + map，或直接用 `hashicorp/golang-lru` | Phase 4.5 会用到 |
| Python 的运行时 monkey patch | 编译期组合 + 显式注册 | Tool registry 就是替代品 |

---

## 5. 反模式清单（你或 agent 一旦做了就要立刻停）

1. **PR 太大**：单个 PR 超过 200 行 diff 或 3 个文件 → 拆分
2. **没有验证步骤**：PR 描述里没有"运行 X 看到 Y" → 不合格
3. **提前抽象**：在只有 1 个实现时定义 interface → 删掉
4. **跨 PR 改动**：本 PR 顺手改了 unrelated 文件 → revert
5. **agent 帮你做设计决策**：你没在 design issue 里决定的东西，agent 自动选了一个方案 → 撤回，先让你决定
6. **测试只测 happy path**：没有 error case、edge case → 补
7. **跳过"丑代码"阶段**：Phase 3.1 你嫌 Registry 蠢就直接做 3.2 → 损失学习信号

---

## 6. 使用建议

1. **在 repo 里建 `docs/issues/` 目录**，每个 PR 之前先写两个 markdown（需求 + 设计），commit 后再开始 PR。
2. **每个 PR 合并后写一段 reflection**（150 字以内）：我学到了什么？我原本以为是 X 但其实是 Y？这是把隐性学习显性化的关键。
3. **每 3-5 个 PR 回头读一次 Hermes 对应源码**，对比你的实现和原作者的差异。差异的地方就是你下一轮要思考的设计 trade-off。
4. **每次给 coding agent 任务时，把第 2 节的协议粘进去**，再附上当前 issue。不要省略。

---

## 附录 A: 第一个 PR 的具体动作（直接执行）

```bash
mkdir hermes-go && cd hermes-go
git init
go mod init github.com/<you>/hermes-go
mkdir -p docs/issues internal/agent internal/provider internal/config cmd/hermes
```

然后写 `docs/issues/0001-bootstrap.md`（按第 1.1 节模板）。**不要让 agent 写这个 issue**。这是你的思考产物。

写完后把这份文档 + issue 一起喂给 coding agent，让它做 PR 0.1。