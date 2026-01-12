# kiro2api Extended Thinking 功能分析报告

## 问题描述

在 Claude Code 中使用 kiro2api 时，无法看到 Extended Thinking（深度思考）功能的输出，而使用其他 2API 项目（如 ai-cli-proxy-api 连接 Anthropic API，或 AIClient-2-API 连接 CodeWhisperer）时可以正常显示 thinking 内容。

## 核心结论

**kiro2api 没有在流式处理中解析 `<thinking>` 标签！**

CodeWhisperer 返回的 thinking 内容是以 `<thinking>...</thinking>` 标签嵌入在 content 字段中的，而不是独立的 `thinkingEvent` 事件。kiro2api 虽然实现了 thinking 相关的代码，但在流式处理的关键路径上没有调用这些代码。

---

## 三个项目对比分析

### 1. kiro.rs (Rust) - 参考实现

**文件**: `src/anthropic/stream.rs`

kiro.rs 是最完整的参考实现，它在 `process_content_with_thinking` 方法中：

```rust
fn process_content_with_thinking(&mut self, content: &str) -> Vec<SseEvent> {
    // 将内容添加到缓冲区
    self.thinking_buffer.push_str(content);

    loop {
        if !self.in_thinking_block && !self.thinking_extracted {
            // 查找 <thinking> 开始标签（跳过被引号包裹的假标签）
            if let Some(start_pos) = find_real_thinking_start_tag(&self.thinking_buffer) {
                // 发送 <thinking> 之前的内容作为 text_delta
                // 进入 thinking 块，创建 content_block_start 事件
            }
        } else if self.in_thinking_block {
            // 查找 </thinking> 结束标签
            if let Some(end_pos) = find_real_thinking_end_tag(&self.thinking_buffer) {
                // 提取 thinking 内容，发送 thinking_delta 事件
                // 发送 content_block_stop 事件
            } else {
                // 发送当前缓冲区内容作为 thinking_delta（流式输出）
            }
        }
    }
}
```

**关键特性**：
- 使用状态机管理 thinking 块的开始/结束
- 假标签检测：跳过被引号、反引号等包裹的 `<thinking>` 标签
- 结束标签验证：真正的 `</thinking>` 后面必须有 `\n\n`
- 流式输出：在 thinking 块内持续输出 `thinking_delta` 事件

### 2. AIClient-2-API (JavaScript)

**文件**: `src/providers/claude/claude-kiro.js`

```javascript
// 流式处理中主动检测 <thinking> 标签
while (streamState.buffer.length > 0) {
    if (!streamState.inThinking && !streamState.thinkingExtracted) {
        const startPos = findRealTag(streamState.buffer, KIRO_THINKING.START_TAG);
        if (startPos !== -1) {
            // 找到 <thinking> 标签
            streamState.inThinking = true;
            // 输出 thinking_delta 事件
        }
    }
    if (streamState.inThinking) {
        const endPos = findRealTag(streamState.buffer, KIRO_THINKING.END_TAG);
        if (endPos !== -1) {
            // 提取 thinking 内容并输出 thinking_delta
            events.push(...createThinkingDeltaEvents(thinkingPart));
        }
    }
}
```

**实现方式**：与 kiro.rs 类似，在流式处理中主动检测和提取 `<thinking>` 标签。

### 3. kiro2api (Go) - 当前实现

**文件**: `parser/message_event_handlers.go`

```go
// handleStreamingEvent 直接把 content 作为 text_delta 输出
func (h *StandardAssistantResponseEventHandler) handleStreamingEvent(event *FullAssistantResponseEvent) ([]SSEEvent, error) {
    if event.Content != "" {
        events = append(events, SSEEvent{
            Event: "content_block_delta",
            Data: map[string]any{
                "type":  "content_block_delta",
                "index": 0,
                "delta": map[string]any{
                    "type": "text_delta",  // ← 直接作为 text_delta！没有解析 thinking 标签
                    "text": event.Content,
                },
            },
        })
    }
    return events, nil
}
```

**问题**：
- `handleStreamingEvent` 直接将 content 作为 `text_delta` 输出
- 没有调用 `ThinkingStreamContext` 状态机
- 没有检测 `<thinking>` 标签
- thinking 相关代码只在以下情况被使用：
  1. 上游返回独立的 `thinkingEvent` 事件类型
  2. 上游返回 `{"type": "thinking", "content": "..."}` 格式

---

## 数据流对比

### CodeWhisperer 返回的数据格式

```json
{
  "assistantResponseEvent": {
    "content": "<thinking>\n这是思考内容...\n</thinking>\n\n这是正式回复内容..."
  }
}
```

### 期望的输出格式（Anthropic Extended Thinking 规范）

```
event: content_block_start
data: {"type":"content_block_start","index":0,"content_block":{"type":"thinking","thinking":""}}

event: content_block_delta
data: {"type":"content_block_delta","index":0,"delta":{"type":"thinking_delta","thinking":"这是思考内容..."}}

event: content_block_stop
data: {"type":"content_block_stop","index":0}

event: content_block_start
data: {"type":"content_block_start","index":1,"content_block":{"type":"text","text":""}}

event: content_block_delta
data: {"type":"content_block_delta","index":1,"delta":{"type":"text_delta","text":"这是正式回复内容..."}}
```

### kiro2api 当前的输出

```
event: content_block_delta
data: {"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"<thinking>\n这是思考内容...\n</thinking>\n\n这是正式回复内容..."}}
```

**问题**：Claude Code 收到的是带 `<thinking>` 标签的纯文本，而不是 `thinking_delta` 事件，因此无法识别为 thinking 模式。

---

## kiro2api 已有但未使用的代码

kiro2api 已经实现了以下 thinking 相关代码，但在流式处理中没有调用：

### 1. ThinkingStreamContext 状态机

**文件**: `parser/thinking_state_machine.go`

```go
type ThinkingState int

const (
    StateNotInThinking ThinkingState = iota  // 未进入 thinking 块
    StateInThinking                          // 在 thinking 块内
    StateThinkingExtracted                   // thinking 已提取完成
)

type ThinkingStreamContext struct {
    enabled           bool
    state             ThinkingState
    buffer            strings.Builder
    thinkingBlockIdx  int
    textBlockIdx      int
}
```

### 2. Thinking 标签检测器

**文件**: `parser/thinking_detector.go`

```go
func FindRealThinkingStartTag(buffer string) int { ... }
func FindRealThinkingEndTag(buffer string) int { ... }
func ExtractThinkingContent(buffer string) (thinking, remaining string, found bool) { ... }
```

### 3. ThinkingEventHandler

**文件**: `parser/message_event_handlers.go`

```go
type ThinkingEventHandler struct{}

func (h *ThinkingEventHandler) Handle(message *EventStreamMessage) ([]SSEEvent, error) {
    // 输出 thinking_delta 事件
}
```

---

## 修复方案

### 方案 1：在 handleStreamingEvent 中集成 thinking 解析

修改 `parser/message_event_handlers.go` 中的 `handleStreamingEvent` 方法：

```go
func (h *StandardAssistantResponseEventHandler) handleStreamingEvent(event *FullAssistantResponseEvent) ([]SSEEvent, error) {
    if event.Content == "" {
        return []SSEEvent{}, nil
    }

    // 如果启用了 thinking 模式，使用 ThinkingStreamContext 处理
    if h.processor.thinkingContext != nil && h.processor.thinkingContext.IsEnabled() {
        return h.processor.thinkingContext.ProcessContent(event.Content)
    }

    // 非 thinking 模式，直接输出 text_delta
    return []SSEEvent{{
        Event: "content_block_delta",
        Data: map[string]any{
            "type":  "content_block_delta",
            "index": 0,
            "delta": map[string]any{
                "type": "text_delta",
                "text": event.Content,
            },
        },
    }}, nil
}
```

### 方案 2：在 StreamProcessor 层面处理

修改 `server/stream_processor.go`，在事件处理后、发送前进行 thinking 标签解析。

### 方案 3：参考 kiro.rs 重构

完全参考 kiro.rs 的 `process_content_with_thinking` 实现，重构 thinking 处理逻辑。

---

## 关键实现要点

参考 kiro.rs 和 AIClient-2-API，thinking 解析需要：

1. **状态机管理**
   - `in_thinking_block`: 是否在 thinking 块内
   - `thinking_extracted`: thinking 是否已提取完成
   - `thinking_buffer`: 缓冲区，用于处理跨事件的标签

2. **假标签检测**
   - 跳过被引号、反引号等包裹的 `<thinking>` 标签
   - 引用字符集：`` ` " ' \ # ! @ $ % ^ & * ( ) - _ = + [ ] { } ; : < > , . ? / ``

3. **结束标签验证**
   - 真正的 `</thinking>` 后面必须有 `\n\n`
   - 边界情况：流结束时 `</thinking>` 后面可能只有空白字符

4. **流式输出**
   - 在 thinking 块内持续输出 `thinking_delta` 事件
   - 保留可能是部分标签的内容（避免截断标签）
   - UTF-8 安全的字符边界处理

5. **块索引管理**
   - thinking 块和 text 块使用不同的索引
   - 正确发送 `content_block_start` 和 `content_block_stop` 事件

---

## 附录：ai-cli-proxy-api 的不同之处

ai-cli-proxy-api 连接的是 Anthropic API（`api.anthropic.com`），而不是 CodeWhisperer。它通过设置 HTTP Header 启用 Extended Thinking：

```go
r.Header.Set("Anthropic-Beta", "claude-code-20250219,oauth-2025-04-20,interleaved-thinking-2025-05-14,fine-grained-tool-streaming-2025-05-14")
```

Anthropic API 原生支持 Extended Thinking，返回的是独立的 `thinking_delta` 事件，不需要解析 `<thinking>` 标签。

---

## 总结

| 项目 | 后端 | Thinking 数据格式 | 是否需要解析标签 | 当前状态 |
|------|------|------------------|-----------------|---------|
| kiro.rs | CodeWhisperer | `<thinking>` 标签嵌入 content | 是 | ✅ 已实现 |
| AIClient-2-API | CodeWhisperer | `<thinking>` 标签嵌入 content | 是 | ✅ 已实现 |
| ai-cli-proxy-api | Anthropic API | 独立 `thinking_delta` 事件 | 否 | ✅ 原生支持 |
| **kiro2api** | CodeWhisperer | `<thinking>` 标签嵌入 content | 是 | ❌ **未实现** |

kiro2api 需要在流式处理中集成 `<thinking>` 标签解析逻辑，才能正确输出 `thinking_delta` 事件，让 Claude Code 识别 thinking 模式。
