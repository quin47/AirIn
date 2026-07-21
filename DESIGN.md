# 技术选型与设计方案: 跨平台语音输入法 (macOS / Linux)

## 一、项目概述

一款基于 Golang + 豆包(火山引擎)实时语音识别 API 的跨平台语音输入法。用户通过快捷键触发录音,语音实时转写为文字并注入当前光标位置,支持 macOS 和 Linux。

---

## 二、技术栈选型及理由

| 层 | 技术选型 | 选型理由 |
|---|---|---|
| **开发语言** | Go 1.22+ | 静态编译、单一二进制分发、优秀并发模型(goroutine)、cgo 可调用系统原生 API |
| **音频采集 - macOS** | CoreAudio (cgo) / portaudio | portaudio 封装良好、跨平台一致;CoreAudio 延迟更低但绑定工作量更大 |
| **音频采集 - Linux** | PulseAudio Simple API / portaudio | PulseAudio 主流发行版标配;portaudio 可统一跨平台代码 |
| **键盘模拟 - macOS** | `CGEventPost` (cgo) | 系统级键盘事件注入,需辅助功能权限(辅助功能授权一次) |
| **键盘模拟 - Linux** | `uinput` (/dev/uinput) | 内核级输入设备模拟,兼容 X11/Wayland,无需依赖 X 扩展 |
| **语音识别引擎** | 豆包语音识别大模型 WebSocket API (双向流式) | README 指定;流式低延迟转写,支持中英文;全双工双向并发 |
| **系统托盘** | `getlantern/systray` 或 `fyne-io/systray` | Go 原生、跨平台、轻量 |
| **快捷键注册** | macOS: `CGEvent` / Carbon; Linux: `keybinder` / X11 grab | 全局热键监听,用于触发/停止录音 |
| **打包分发** | GoReleaser + Homebrew (macOS) / deb/rpm/AppImage (Linux) | CI/CD 自动化构建与发布 |
| **音频编码** | PCM 16kHz 16bit 单声道 (raw) | 豆包 API 默认格式,低延迟无编码损耗 |

---

## 三、整体架构图

```
┌─────────────────────────────────────────────────────┐
│                    UI Layer                          │
│   ┌──────────┐  ┌──────────┐  ┌──────────────────┐  │
│   │ 系统托盘  │  │ 浮窗提示  │  │ 快捷键监听         │  │
│   └────┬─────┘  └────┬─────┘  └────────┬─────────┘  │
│        │              │                 │            │
├────────┼──────────────┼─────────────────┼────────────┤
│        ▼              ▼                 ▼            │
│   ┌─────────────────────────────────────────────┐    │
│   │              State Machine                   │    │
│   │   IDLE → LISTENING → TRANSCRIBING → DONE    │    │
│   └─────────────────────┬───────────────────────┘    │
│                         │                             │
│   ┌─────────────────────┼───────────────────────┐    │
│   │         Core Service Layer                   │    │
│   │                                              │    │
│   │  ┌───────────┐  ┌───────────┐  ┌─────────┐  │    │
│   │  │ Audio     │  │ ASR       │  │ Text     │  │    │
│   │  │ Capture   │─▶│ Client    │─▶│ Injector │  │    │
│   │  │ Module    │  │ (WebSocket│  │ Module   │  │    │
│   │  └───────────┘  │ 双向流式)  │  └─────────┘  │    │
│   │                  └─────┬─────┘               │    │
│   │                        │                      │    │
│   │                  ┌─────▼─────┐               │    │
│   │                  │ Token     │               │    │
│   │                  │ Auth      │               │    │
│   │                  └───────────┘               │    │
│   └──────────────────────────────────────────────┘    │
└─────────────────────────────────────────────────────┘
```

---

## 四、核心模块设计

### 4.1 Audio Capture Module (音频采集)

```go
// 抽象接口,平台各自实现
type AudioCapture interface {
    Start() (<-chan []int16, error)  // 返回 PCM 16bit 音频流
    Stop() error
    Devices() ([]Device, error)
}
```

- **macOS 实现**: 基于 `AudioQueue` / `AudioUnit`,获取默认输入设备,采样率 16kHz,单声道,PCM S16LE
- **Linux 实现**: 基于 PulseAudio Simple API,参数同上
- **可选统一方案**: 引入 `portaudio` C 库绕过分平台绑定

### 4.2 ASR Client Module — 双向流式(优化版)

基于火山引擎 豆包语音识别大模型的 WebSocket 双向流式协议构建。核心特点:

#### 4.2.1 协议选型优势

| 维度 | 基础流式 | 双向流式(优化版/大模型) |
|---|---|---|
| **协议** | WebSocket 二进制帧协议 | WebSocket 二进制流 + JSON |
| **识别模型** | 传统 ASR 模型 | 豆包语音识别大模型 |
| **首字延迟** | ~500ms | ~200ms |
| **并发** | 单向上行,下行按序 | 全双工:音频上行与结果下行独立并发 |
| **中间结果** | 有限支持 | `definite: false` 中间结果实时刷新 + `definite: true` 最终确认 |
| **分句/VAD** | 服务端固定分段 | `show_utterances` + `result_type=single` 实时分句 |
| **热词/替换词** | 不支持 | 支持 `boosting_table_name` / `correct_table_name` |
| **字级时间戳** | 无 | `words[].start_time/end_time` 逐字时间戳 |

#### 4.2.2 双向流式交互流程

```
Client                                Server
  │                                     │
  │──── HTTP Upgrade + 鉴权签名头 ─────▶│  (WebSocket 握手)
  │                                     │
  │──── Full Client Request ───────────▶│  (握手帧: JSON 参数 + Sequence=1)
  │     {                                │
  │       "app": {appid, token, cluster} │
  │       "audio": {format,rate,...}     │
  │       "request": {                   │
  │         "reqid": "uuid",            │
  │         "sequence": 1,              │
  │         "show_utterances": true,    │   启用分句+分词
  │         "result_type": "single"     │   逐句返回(非全量)
  │       }                              │
  │     }                                │
  │                                     │
  │  ═══════ 双向流式阶段 ═══════        │
  │                                     │
  │──── Audio Only (seq=2) ────────────▶│
  │──── Audio Only (seq=3) ────────────▶│  Client 持续推送音频帧
  │◀─── Full Server Response ───────────│  Server 异步返回识别结果
  │     {"sequence":2, "code":1000,      │
  │      "result":[{"text":"今天",        │
  │        "definite":false}]}          │     (中间结果,流式刷新)
  │                                     │
  │──── Audio Only (seq=4) ────────────▶│
  │◀─── Full Server Response ───────────│
  │     {"sequence":3, "code":1000,      │
  │      "result":[{"text":"今天天气",    │
  │        "definite":false}]}          │     (持续修正)
  │                                     │
  │◀─── Full Server Response ───────────│
  │     {"sequence":3, "code":1000,      │
  │      "result":[{"text":"今天天气不错",│
  │        "definite":true,             │     最终结果,确定不再变
  │        "words":[{"text":"今",...}]}]}│
  │                                     │
  │──── Audio Only (seq=-5, END) ──────▶│  最后一帧 sequence 取负值
  │◀─── Full Server Response ───────────│
  │     {"sequence":-5, "code":1000,     │
  │      "result":[...]}               │
  │                                     │
  │──── Close ──────────────────────────│
  │                                     │
```

#### 4.2.3 二进制帧格式 (大端)

```
Byte 0:    [Protocol Version:4][Header Size:4]
Byte 1:    [Message Type:4][Type Flags:4]
Byte 2:    [Serialization:4][Compression:4]
Byte 3:    [Reserved:8]
Byte 4-7:  Payload Size (uint32, big-endian)
Byte 8+:   Payload (JSON or raw audio)
```

Message Type 常量:
- `0x10` — Full Client Request (含 JSON 参数的握手帧)
- `0x20` — Audio Only Request (纯音频数据帧)
- `0x22` — Audio Only Request + Last (最后一帧,sequence 取负值)
- `0x90` — Full Server Response (识别结果)
- `0xF0` — Server Error

#### 4.2.4 核心接口定义

```go
type ASRClient interface {
    Connect(ctx context.Context) error
    SendAudio(pcm []int16) error
    SendEnd() error
    Results() <-chan RecognitionResult
    Errors() <-chan error
    Close() error
}

type RecognitionResult struct {
    Text      string     // 当前累积/分句文本
    Definite  bool       // true=最终结果不再变, false=中间结果
    Utterance *Utterance // 分句详情(show_utterances=true时)
    SeqNum    int        // 对应的音频帧序号
}

type Utterance struct {
    Text      string
    StartTime int
    EndTime   int
    Words     []Word
}

type Word struct {
    Text      string
    StartTime int
    EndTime   int
}
```

#### 4.2.5 双向并发架构

```
                    ┌──────────────────────┐
                    │     ASR Client        │
                    │                      │
  音频采集 ────────▶│  audioChan           │
  goroutine         │    │                 │
                    │    ▼                 │
                    │  sendLoop(写协程)     │──── WebSocket ────▶ 火山引擎 ASR
                    │                      │         │
                    │  readLoop(读协程)     │◀────────┘
                    │    │                 │
                    │    ▼                 │
  TextInjector ◀────│  resultCh            │
                    │                      │
                    └──────────────────────┘

  写协程(sendLoop):
    - 从 audioChan 消费 PCM 数据
    - 编码为二进制帧
    - 写入 WebSocket
    - sequence 号原子递增
    - 不阻塞: 使用 buffered channel

  读协程(readLoop):
    - 持续从 WebSocket 读取帧
    - 解码 binary header
    - 解析 JSON → RecognitionResult
    - definite=false: 标记为中间结果(UI 灰色显示)
    - definite=true: 标记为最终结果(UI 黑色确认)
    - 写入 resultCh 供 TextInjector 消费
```

#### 4.2.6 关键优化参数配置

```go
type ASRConfig struct {
    AppID     string `json:"appid"`
    Token     string `json:"token"`
    Cluster   string `json:"cluster"`

    // 音频参数
    Format   string `json:"format"`  // "raw" / "wav" / "mp3"
    Codec    string `json:"codec"`   // "raw" (PCM) / "opus"
    Rate     int    `json:"rate"`    // 16000
    Bits     int    `json:"bits"`    // 16
    Channel  int    `json:"channel"` // 1

    // 请求优化参数
    ShowUtterances bool   `json:"show_utterances"` // true: 启用分句+分词
    ResultType     string `json:"result_type"`     // "single": 逐句返回
    NBest          int    `json:"nbest"`           // 1
    Workflow       string `json:"workflow"`        // "audio_in,resample,partition,vad,fe,decode"

    // 热词/替换词(可选)
    BoostingTableName string `json:"boosting_table_name,omitempty"`
    CorrectTableName  string `json:"correct_table_name,omitempty"`
}
```

### 4.3 Text Injection Module (文本注入)

```go
type TextInjector interface {
    // Inject 注入文本,支持中间结果增量更新
    Inject(text string, definite bool) error
    // Commit 确认当前文本,移动光标到最终位置
    Commit() error
    // Cancel 取消当前输入,清除中间结果
    Cancel() error
}
```

#### 增量输出策略 (利用 definite 标志)

```go
func (inj *TextInjector) HandleStream(resultCh <-chan RecognitionResult) {
    var lastText string

    for result := range resultCh {
        if result.Definite {
            // 最终结果: 提交确认,移动光标至末尾
            inj.commit(result.Text)
            lastText = ""
        } else {
            // 中间结果: 计算增量差异,仅注入变更部分
            delta := diffText(lastText, result.Text)
            inj.applyDelta(delta)
            lastText = result.Text
        }
    }
}
```

- **macOS**: `CGEventCreateKeyboardEvent` + `CGEventPost` 逐字符模拟键盘输入
- **Linux**: `uinput` + 剪贴板粘贴方案(兼容性最佳,尤其中文);备选 `ibus/fcitx` forward API

### 4.4 State Machine (状态机)

```
                    hotkey press
     ┌──────┐ ──────────────────▶ ┌───────────┐
     │ IDLE │                      │ LISTENING │
     └──┬───│◀──── hotkey/error ───│  (录音中)  │
        │   │                      └─────┬─────┘
        │   │                    语音检测到活动
        │   │                            │
        │   │                      ┌─────▼─────────┐
        │   │                      │ TRANSCRIBING  │
        │   │   识别完成/超时      │ (转写中,持续输出)│
        │   │◀─────────────────────└───────────────┘
        │   │
  hotkey press     ┌──────────┐
        └─────────▶│ SHUTDOWN │
                    └──────────┘
```

- **IDLE**: 待机,等待热键触发
- **LISTENING**: 启动麦克风采集,持续发送音频,等待首字返回
- **TRANSCRIBING**: 流式接收识别文本并实时注入光标;中间结果用 `definite: false` 区分,最终结果用 `definite: true` 确认
- **SHUTDOWN**: 收起录音,完成最终文本注入,回到 IDLE

### 4.5 UI 层

| 组件 | 方案 |
|---|---|
| **系统托盘图标** | `systray` 库,显示麦克风图标;右键菜单: 启动/停止、设置、退出 |
| **录音状态浮窗** | 透明无边框窗口,macOS: `NSFloatingWindowLevel`,Linux: `_NET_WM_STATE_ABOVE` |
| **音频电平指示** | 实时 PPM 柱状图(读取 PCM 幅值),反应用户说话音量 |
| **识别状态提示** | 区分中间结果(灰色显示)与最终结果(正常文字) |
| **热键设置** | 可配置组合键(默认 `Ctrl+Shift+V`),持久化到 `~/.config/ime/config.json` |

---

## 五、数据流

```
麦克风 ──PCM 16k──▶ Audio Capture ──chan []int16──▶ ASR Client
                                                       │
                                              WebSocket (wss://)
                                              openspeech.bytedance.com
                                              /api/v2/asr
                                                       │
                                       ┌───────────────┴───────────────┐
                                       │       全双工双向并发           │
                                       │  sendLoop ──▶ 音频帧流        │
                                       │         ◀── readLoop 识别结果 │
                                       └───────────────────────────────┘
                                                       │
                                              JSON 识别结果流
                                              (definite + utterances)
                                                       │
                                                       ▼
                                                 Text Injector
                                                 增量 diff 输出
                                                       │
                                            模拟键盘/粘贴注入
                                                       │
                                                       ▼
                                                 应用程序光标
```

---

## 六、目录结构规划

```
ime/
├── cmd/
│   └── ime/                # 入口 main.go
├── internal/
│   ├── audio/              # 音频采集抽象 + 平台实现
│   │   ├── audio.go        # AudioCapture 接口
│   │   ├── capture_darwin.go
│   │   └── capture_linux.go
│   ├── asr/                # 豆包 ASR WebSocket 客户端(双向流式)
│   │   ├── client.go       # 客户端实现(读写双协程)
│   │   ├── protocol.go     # 二进制帧编解码
│   │   ├── auth.go         # Token 鉴权签名
│   │   └── types.go        # 协议类型定义
│   ├── inject/             # 文本注入
│   │   ├── inject.go       # TextInjector 接口 + 增量 diff 逻辑
│   │   ├── inject_darwin.go
│   │   └── inject_linux.go
│   ├── hotkey/             # 全局热键
│   ├── tray/               # 系统托盘
│   ├── state/              # 状态机
│   └── config/             # 配置管理
├── assets/                 # 图标资源
├── scripts/                # 构建脚本
├── go.mod
├── go.sum
├── Makefile
├── README.md
└── DESIGN.md
```

---

## 七、API 接入端点与鉴权

| 项目 | 说明 |
|---|---|
| **接入地址** | `wss://openspeech.bytedance.com/api/v2/asr` |
| **鉴权方式** | Token-based HMAC-SHA256 签名,通过 HTTP Header 传递
| **鉴权流程** | ① 火山引擎 IAM 获取 AccessKey + SecretKey ② 每次连接生成签名头(`X-Date` + `Authorization`) ③ 携带签名头完成 WebSocket 升级 |
| **音频格式** | PCM 16kHz 16bit 单声道,raw audio 二进制帧 |
| **协议格式** | 首帧 JSON 握手 → 双向流式阶段(音频上行 + 识别结果下行) |
| **流控** | 客户端限速发送,每秒约 10 帧(每帧 100ms 音频 = 3200 bytes) |

---

## 八、关键技术风险与应对

| 风险 | 影响 | 应对措施 |
|---|---|---|
| Linux 中文文本注入兼容性差 | 部分应用无法输入中文 | 优先使用剪贴板+粘贴方案,备选 ibus forward API |
| Wayland 下全局热键受限 | Linux 下热键无法注册 | 检测 `$XDG_SESSION_TYPE`,Wayland 下提示用户手动设置 |
| 网络波动导致 ASR 断连 | 识别中断 | WebSocket 自动重连 + 本地短时(3s)环形缓冲缓存音频 |
| 豆包 API 费用控制 | 成本过高 | 本地 VAD 前置过滤静音,仅发送有效语音段 |
| macOS 辅助功能权限 | 键盘注入拒绝 | 首次启动引导用户授权 `系统偏好 → 隐私 → 辅助功能` |
| 中间结果闪烁 | 用户输入体验差 | 增量 diff 输出,仅注入变更字符,避免全量删除重写 |

---

## 九、开发阶段规划

| 阶段 | 内容 | 产出 |
|---|---|---|
| **Phase 1** | 项目骨架 + 音频采集(macOS) + ASR Client 双向流式 | 可录音并打印识别结果 |
| **Phase 2** | 键盘注入(macOS) + 状态机 + 系统托盘 | macOS 可用版本 |
| **Phase 3** | Linux 音频采集 + 键盘注入适配 | Linux 可用版本 |
| **Phase 4** | UI 优化(浮窗、电平指示、配置面板) + 热词功能 | 体验完善 |
| **Phase 5** | 打包分发(GoReleaser + Homebrew/apt) + CI/CD | 正式发布 |
