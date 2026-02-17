# Agent 系统开发 TODO

## 已完成功能
- [x] Core Agent System (pkg/agent/core.go)
- [x] Basic Agents (Echo, Time, Weather)
- [x] Pipeline Integration (AgentElement)
- [x] Example (agent-pipeline)

## 待开发功能

### Phase 1: 核心功能增强
- [ ] LLM-based Router - 智能路由决策
- [ ] Tool System - 外部 API 调用
- [ ] Memory System - 对话历史存储
- [ ] Multi-Agent Collaboration - 多 Agent 协作

### Phase 2: Realtime API 集成
- [ ] AgentRealtimeServer - WebRTC + Agent
- [ ] Response lifecycle with Agent
- [ ] Interrupt handling

### Phase 3: 测试与 CI/CD
- [ ] 修复自动化测试
- [ ] Agent 系统单元测试
- [ ] 集成测试

---

## 测试修复计划

### 当前测试问题

1. **API Key 依赖测试**
   - `pkg/asr/*_test.go` - 需要 API Key
   - `pkg/tts/*_test.go` - 需要 API Key
   - `pkg/elements/*_test.go` - 可能需要外部服务

2. **外部服务依赖**
   - FFmpeg 相关测试
   - WebRTC 测试
   - VAD (ONNX Runtime) 测试

3. **Agent 系统测试缺失**
   - 需要添加 Agent 单元测试
   - Coordinator 测试
   - Router 测试

### 修复策略

#### 1. 分离单元测试和集成测试

```yaml
# .github/workflows/test.yml 修改建议
jobs:
  unit-test:
    name: Unit Tests
    runs-on: ubuntu-latest
    steps:
      - name: Run unit tests (no external deps)
        run: go test -short ./...
  
  integration-test:
    name: Integration Tests
    runs-on: ubuntu-latest
    needs: unit-test
    if: github.event_name == 'push' && github.ref == 'refs/heads/main'
    steps:
      - name: Run integration tests (with API keys)
        run: go test -run Integration ./...
        env:
          GOOGLE_API_KEY: ${{ secrets.GOOGLE_API_KEY }}
          OPENAI_API_KEY: ${{ secrets.OPENAI_API_KEY }}
```

#### 2. 使用 Mock 替代外部服务

```go
// 示例：Mock LLM Provider
type MockLLMProvider struct {
    Responses map[string]string
}

func (m *MockLLMProvider) Complete(ctx context.Context, prompt string) (string, error) {
    if response, ok := m.Responses[prompt]; ok {
        return response, nil
    }
    return "mock response", nil
}
```

#### 3. 添加 Agent 系统测试

```go
// pkg/agent/core_test.go
func TestCoordinator_Register(t *testing.T) {
    c := NewCoordinator(nil)
    agent := NewMockAgent("test", "Test Agent")
    
    c.Register(agent)
    
    if len(c.ListAgents()) != 1 {
        t.Errorf("Expected 1 agent, got %d", len(c.ListAgents()))
    }
}

func TestCoordinator_Process(t *testing.T) {
    c := NewCoordinator(nil)
    c.Register(NewEchoAgent())
    
    msg := &Message{
        SessionID:   "test-session",
        Type:        MessageTypeText,
        TextContent: "Hello",
    }
    
    response, err := c.Process(context.Background(), "test-session", msg)
    if err != nil {
        t.Fatalf("Process failed: %v", err)
    }
    
    if !strings.Contains(response.TextContent, "Hello") {
        t.Errorf("Expected response to contain 'Hello', got: %s", response.TextContent)
    }
}
```

---

## 需要的 API Keys

### 当前 CI/CD 已配置的 Keys
- `GOOGLE_API_KEY` - Gemini API
- `OPENAI_API_KEY` - OpenAI API (Whisper, TTS, GPT)
- `OPENAI_BASE_URL` - OpenAI Base URL (可选，用于代理)
- `DASHSCOPE_API_KEY` - 阿里云 DashScope (Qwen)
- `ELEVENLABS_API_KEY` - ElevenLabs TTS
- `CODECOV_TOKEN` - Codecov 覆盖率上传

### 可能需要新增的 Keys
- `AZURE_SPEECH_KEY` - Azure Speech Services (如果添加 Azure 支持)
- `ANTHROPIC_API_KEY` - Claude API (如果使用 Anthropic)

### 本地开发建议

创建 `.env.local` 文件（已加入 .gitignore）：

```bash
# .env.local
export GOOGLE_API_KEY="your-google-api-key"
export OPENAI_API_KEY="your-openai-api-key"
export DASHSCOPE_API_KEY="your-dashscope-api-key"
export ELEVENLABS_API_KEY="your-elevenlabs-api-key"
```

加载：
```bash
source .env.local
```

---

## 下一步行动

1. **测试分类**
   - 标记需要 API Key 的测试为 `//go:build integration`
   - 添加 Mock 实现用于单元测试

2. **Agent 测试**
   - 添加 `pkg/agent/core_test.go`
   - 添加 `pkg/agents/basic_test.go`
   - 添加 `pkg/elements/agent_element_test.go`

3. **CI 优化**
   - 分离 unit test 和 integration test
   - 添加 Agent 构建验证

需要我立即开始修复测试吗？还是需要先获取某些 API Key？
