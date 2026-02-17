# Realtime AI Agent 架构设计方案

## 核心思想转变

### 从 LLM-Centric 到 Agent-Centric

```
旧架构：Pipeline = Audio → STT → LLM → TTS → Audio
                    (单一 LLM 处理所有逻辑)

新架构：Pipeline = Audio → STT → Agent System → TTS → Audio
                    (多个 Agent 协作，LLM 只是 Agent 的能力之一)
```

## Agent 架构设计

### 1. Agent 定义

```go
// Agent 是一个能够处理特定任务的智能实体
type Agent interface {
    // 身份标识
    ID() string
    Name() string
    Description() string
    
    // 能力声明
    Capabilities() []Capability
    
    // 处理消息
    Process(ctx context.Context, msg *Message) (*Message, error)
    
    // 生命周期
    Start(ctx context.Context) error
    Stop() error
}

// Capability 表示 Agent 的能力
type Capability struct {
    Name        string
    Description string
    InputTypes  []DataType
    OutputTypes []DataType
    Tools       []Tool  // Agent 可以使用的工具
}
```

### 2. Agent 类型

```
┌─────────────────────────────────────────────────────────────────┐
│                        Agent 生态系统                             │
├─────────────────────────────────────────────────────────────────┤
│                                                                 │
│  ┌─────────────┐  ┌─────────────┐  ┌─────────────┐             │
│  │  Router     │  │  Task       │  │  Memory     │             │
│  │  Agent      │  │  Agent      │  │  Agent      │             │
│  │  (入口)      │  │  (执行)      │  │  (存储)      │             │
│  └──────┬──────┘  └──────┬──────┘  └──────┬──────┘             │
│         │                │                │                    │
│         └────────────────┼────────────────┘                    │
│                          │                                     │
│              ┌───────────▼───────────┐                        │
│              │     Coordinator       │                        │
│              │     (协调器)           │                        │
│              └───────────┬───────────┘                        │
│                          │                                     │
│         ┌────────────────┼────────────────┐                   │
│         │                │                │                   │
│  ┌──────▼──────┐  ┌──────▼──────┐  ┌──────▼──────┐          │
│  │  Weather    │  │  Calendar   │  │  Search     │          │
│  │  Agent      │  │  Agent      │  │  Agent      │          │
│  │  (天气查询)   │  │  (日程管理)   │  │  (信息检索)   │          │
│  └─────────────┘  └─────────────┘  └─────────────┘          │
│                                                                 │
│  ┌─────────────┐  ┌─────────────┐  ┌─────────────┐             │
│  │  Code       │  │  Image      │  │  Knowledge  │             │
│  │  Agent      │  │  Agent      │  │  Agent      │             │
│  │  (代码生成)   │  │  (图像处理)   │  │  (知识库)    │             │
│  └─────────────┘  └─────────────┘  └─────────────┘             │
│                                                                 │
└─────────────────────────────────────────────────────────────────┘
```

### 3. 核心组件

#### 3.1 Coordinator (协调器)

```go
// Coordinator 管理所有 Agent 的协作
type Coordinator struct {
    agents      map[string]Agent
    router      Router
    memory      Memory
    
    // 对话状态
    sessions    map[string]*AgentSession
}

// Process 处理用户输入，协调 Agent 执行
func (c *Coordinator) Process(ctx context.Context, sessionID string, input *Message) (*Message, error) {
    session := c.sessions[sessionID]
    
    // 1. 构建上下文
    context := c.buildContext(session, input)
    
    // 2. 路由决策：决定由哪个 Agent 处理
    route := c.router.Decide(ctx, context)
    
    // 3. 执行 Agent
    agent := c.agents[route.AgentID]
    result, err := agent.Process(ctx, input)
    
    // 4. 如果需要多 Agent 协作
    if route.RequiresCollaboration {
        result = c.collaborate(ctx, route.Collaborators, input, result)
    }
    
    // 5. 更新记忆
    c.memory.Store(sessionID, input, result)
    
    return result, nil
}
```

#### 3.2 Router (路由器)

```go
// Router 决定请求应该由哪个 Agent 处理
type Router interface {
    Decide(ctx context.Context, context *Context) *RouteDecision
}

type RouteDecision struct {
    AgentID               string
    Confidence            float64
    Reasoning             string
    RequiresCollaboration bool
    Collaborators         []string
}

// LLMRouter 使用 LLM 进行路由决策
type LLMRouter struct {
    llm        LLMClient
    agents     []AgentDescriptor
}

func (r *LLMRouter) Decide(ctx context.Context, context *Context) *RouteDecision {
    prompt := r.buildRoutingPrompt(context)
    
    // 调用 LLM 进行决策
    response := r.llm.Complete(ctx, prompt)
    
    // 解析决策
    return parseRoutingDecision(response)
}
```

#### 3.3 Agent 实现示例

```go
// WeatherAgent 天气查询 Agent
type WeatherAgent struct {
    BaseAgent
    weatherAPI WeatherService
}

func (a *WeatherAgent) Capabilities() []Capability {
    return []Capability{
        {
            Name:        "weather_query",
            Description: "查询指定城市的天气信息",
            InputTypes:  []DataType{DataTypeText, DataTypeLocation},
            OutputTypes: []DataType{DataTypeText, DataTypeAudio},
            Tools: []Tool{
                {
                    Name:        "get_current_weather",
                    Description: "获取当前天气",
                    Parameters:  []Parameter{
                        {Name: "city", Type: "string", Required: true},
                        {Name: "unit", Type: "string", Enum: []string{"celsius", "fahrenheit"}},
                    },
                },
            },
        },
    }
}

func (a *WeatherAgent) Process(ctx context.Context, msg *Message) (*Message, error) {
    // 1. 理解意图
    intent := a.extractIntent(msg)
    
    // 2. 提取参数
    params := a.extractParameters(msg)
    
    // 3. 调用工具
    weather := a.weatherAPI.GetWeather(params.City, params.Unit)
    
    // 4. 生成回复
    response := a.generateResponse(weather, msg.Language)
    
    return &Message{
        Type:    MessageTypeText,
        Content: response,
        Audio:   a.generateAudio(response), // 可选
    }, nil
}
```

### 4. 与 Realtime API 的集成

```go
// RealtimeAgentSession 将 Agent 系统与 Realtime API 连接
type RealtimeAgentSession struct {
    sessionID    string
    coordinator  *Coordinator
    realtimeAPI  *realtimeapi.Session
    
    // 状态
    currentAgent Agent
    agentStack   []Agent  // Agent 调用栈（用于子任务）
}

// HandleInput 处理 Realtime API 的输入
func (s *RealtimeAgentSession) HandleInput(audioData []byte) error {
    // 1. 构建消息
    msg := &Message{
        Type:      MessageTypeAudio,
        AudioData: audioData,
        SessionID: s.sessionID,
    }
    
    // 2. 发送给 Coordinator 处理
    response, err := s.coordinator.Process(s.ctx, s.sessionID, msg)
    if err != nil {
        return err
    }
    
    // 3. 转换回 Realtime API 事件
    s.sendResponseEvents(response)
    
    return nil
}

// sendResponseEvents 将 Agent 响应转换为 Realtime API 事件
func (s *RealtimeAgentSession) sendResponseEvents(response *Message) {
    // 创建 Response
    s.realtimeAPI.CreateResponse(events.ResponseConfig{
        Modalities: []events.Modality{events.ModalityAudio, events.ModalityText},
    })
    
    // 发送内容
    switch response.Type {
    case MessageTypeAudio:
        s.realtimeAPI.SendAudioDelta(response.AudioData)
    case MessageTypeText:
        s.realtimeAPI.SendTextDelta(response.Content)
    }
    
    // 完成
    s.realtimeAPI.CompleteResponse()
}
```

### 5. 多 Agent 协作流程

```
用户: "帮我查一下明天北京的天气，如果下雨就提醒我带上雨伞，
      并把提醒添加到我的日历"

Coordinator 处理流程:

1. Router 决策
   ├─ 意图分析: 复合任务（天气查询 + 条件判断 + 日历操作）
   ├─ 主 Agent: WeatherAgent
   └─ 协作 Agent: CalendarAgent

2. WeatherAgent 执行
   ├─ 调用 get_current_weather(city="北京", date="明天")
   ├─ 返回: {"condition": "rain", "temperature": "15-20°C"}
   └─ 生成回复: "明天北京有雨，气温15-20度"

3. Coordinator 判断
   ├─ 检测到条件: "如果下雨"
   ├─ 条件为真，触发后续动作
   └─ 调用 CalendarAgent

4. CalendarAgent 执行
   ├─ 调用 create_reminder(
   │     title="带雨伞",
   │     time="明天早上8点",
   │     condition="weather.rain"
   │   )
   └─ 返回: "已添加提醒"

5. 整合回复
   "明天北京有雨，气温15-20度。已为您添加明天早上8点的带雨伞提醒。"
```

### 6. 记忆系统

```go
// Memory 为 Agent 提供长期记忆能力
type Memory interface {
    // 存储对话历史
    Store(sessionID string, msg *Message) error
    
    // 检索相关记忆
    Retrieve(ctx context.Context, sessionID string, query string, limit int) ([]*MemoryItem, error)
    
    // 存储用户偏好
    StorePreference(userID string, key string, value interface{}) error
    
    // 获取用户偏好
    GetPreference(userID string, key string) (interface{}, error)
}

type MemoryItem struct {
    Timestamp time.Time
    Content   string
    Type      MemoryType
    Relevance float64
}
```

### 7. 工具系统

```go
// Tool 是 Agent 可以调用的外部能力
type Tool interface {
    Name() string
    Description() string
    Parameters() []Parameter
    Execute(ctx context.Context, args map[string]interface{}) (interface{}, error)
}

// ToolRegistry 工具注册表
type ToolRegistry struct {
    tools map[string]Tool
}

func (r *ToolRegistry) Register(tool Tool) {
    r.tools[tool.Name()] = tool
}

func (r *ToolRegistry) Execute(ctx context.Context, name string, args map[string]interface{}) (interface{}, error) {
    tool, ok := r.tools[name]
    if !ok {
        return nil, fmt.Errorf("tool not found: %s", name)
    }
    return tool.Execute(ctx, args)
}
```

## 架构优势

1. **模块化** - 每个 Agent 独立开发、测试、部署
2. **可扩展** - 新功能只需添加新 Agent
3. **可复用** - Agent 可以在不同场景复用
4. **可观测** - 清晰的调用链和决策路径
5. **灵活性** - 支持单 Agent、多 Agent 协作、Agent 调用链

## 实现路线图

### Phase 1: 基础架构
- [ ] Agent 接口定义
- [ ] Coordinator 实现
- [ ] Router 实现
- [ ] 基础 Agent（Echo、Weather）

### Phase 2: 核心能力
- [ ] Memory 系统
- [ ] Tool 系统
- [ ] 多 Agent 协作
- [ ] Realtime API 集成

### Phase 3: 高级功能
- [ ] Agent 学习/优化
- [ ] 动态 Agent 发现
- [ ] Agent 市场/生态
