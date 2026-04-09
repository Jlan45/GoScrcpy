# GoScrcpy

[scrcpy](https://github.com/Genymobile/scrcpy) 的 Go 语言客户端实现。GoScrcpy 让你无需在设备上安装任何 App，即可通过 Go 程序或浏览器镜像并控制 Android 设备。

[English Documentation](README.md)

---

## 功能特性

- **视频流** — 支持 H.264、H.265、AV1
- **音频流** — 支持 Opus、AAC、FLAC、PCM Raw
- **设备控制** — 触摸、滚动、按键事件、返回/主页/最近任务/电源键
- **多设备** — 可同时管理多台 ADB 设备
- **Web 界面** — 内置浏览器 UI，使用 WebCodecs 加速解码
- **REST + WebSocket API** — 可与任意前端或服务集成
- **自动连接** — 通过环境变量在启动时自动连接设备

---

## 环境要求

- Go 1.24+
- [ADB](https://developer.android.com/studio/command-line/adb) 已安装并在 `PATH` 中
- [scrcpy-server jar](https://github.com/Genymobile/scrcpy/releases)（推荐版本 **3.3.4**）
- 已开启 USB 调试的 Android 设备

---

## 快速开始

### 1. 克隆并编译示例服务器

```bash
git clone https://github.com/Jlan45/GoScrcpy.git
cd GoScrcpy/example
go build -o goscrcpy .
```

### 2. 下载 scrcpy 服务端 jar

从 [scrcpy 发布页](https://github.com/Genymobile/scrcpy/releases/tag/v3.3.4) 下载 `scrcpy-server-v3.3.4`，放到 `example/` 目录或任意位置。

### 3. 启动服务器

```bash
SCRCPY_SERVER_PATH=/path/to/scrcpy-server ./goscrcpy
```

在浏览器中打开 **http://localhost:8080**。

可选环境变量：

| 变量 | 默认值 | 说明 |
|---|---|---|
| `PORT` | `8080` | HTTP 监听端口 |
| `SCRCPY_SERVER_PATH` | *(空)* | scrcpy-server jar 的本地路径。若为空，jar 必须已存在于设备的 `/data/local/tmp/scrcpy-server.jar`。 |
| `DEVICE_SERIAL` | *(空)* | 启动时自动连接的 ADB 设备序列号 |

---

## Web 界面

内置 Web 界面（访问 `/`）支持：

- **刷新** ADB 检测到的设备列表
- **连接/断开** 单个设备
- 通过 WebCodecs（H.264/H.265/AV1）在浏览器中实时查看设备屏幕
- 发送**触摸**、**滚动**、**返回**、**主页**、**最近任务**、**电源**等操作

---

## REST API

所有接口返回 JSON。

| 方法 | 路径 | 说明 |
|---|---|---|
| `GET` | `/api/devices` | 列出 ADB 设备及其连接状态 |
| `POST` | `/api/connect?serial=<序列号>` | 为该设备启动 scrcpy 会话 |
| `POST` | `/api/disconnect?serial=<序列号>` | 停止该设备的 scrcpy 会话 |
| `GET` | `/api/sessions` | 列出所有活跃的 scrcpy 会话 |

### 示例

```bash
# 列出设备
curl http://localhost:8080/api/devices

# 连接设备
curl -X POST "http://localhost:8080/api/connect?serial=emulator-5554"

# 断开设备
curl -X POST "http://localhost:8080/api/disconnect?serial=emulator-5554"
```

---

## WebSocket API

设备连接后，建立 WebSocket 连接：`ws://localhost:8080/ws?serial=<序列号>`

### 服务端 → 客户端消息

| 字节 0 | 含义 | 负载 |
|---|---|---|
| `0x00` | 视频数据包 | `byte[1]` 标志位（`0x01`=关键帧，`0x02`=配置帧），`uint64 大端序` PTS，原始 NAL 数据 |
| `0x02` | 设备信息 | JSON：`{"serial","device","width","height","codec"}` |

### 客户端 → 服务端消息（控制）

| 字节 0 | 操作 | 负载 |
|---|---|---|
| `0` | 触摸 | `action(1) x(4) y(4) sw(2) sh(2)`，均为大端序 |
| `1` | 按键 | `action(1) keycode(4)` |
| `2` | 滚动 | `x(4) y(4) sw(2) sh(2) hscroll(2) vscroll(2)` |
| `3` | 返回键 | — |
| `4` | 主页键 | — |
| `5` | 最近任务键 | — |
| `6` | 电源键 | — |

---

## 作为 Go 库使用

可以在自己的 Go 程序中直接嵌入 `client` 包。

```go
import (
    "context"
    "github.com/Jlan45/GoScrcpy/client"
)

opts := client.DefaultOptions()
opts.Serial        = "emulator-5554"    // ADB 设备序列号
opts.ServerPath    = "/path/to/scrcpy-server"
opts.ServerVersion = "3.3.4"
opts.VideoCodec    = "h264"             // "h264", "h265", "av1"
opts.MaxSize       = 1080               // 限制最大画面分辨率

c := client.NewClient(opts)
if err := c.Start(context.Background()); err != nil {
    log.Fatal(err)
}
defer c.Stop()

// 消费视频数据包
for pkt := range c.VideoStream() {
    // pkt.Data      — 原始 NAL 数据
    // pkt.IsConfig  — SPS/PPS 配置帧
    // pkt.IsKeyFrame
    // pkt.PTS
}
```

### `client.Options` 字段说明

| 字段 | 默认值 | 说明 |
|---|---|---|
| `Serial` | `""` | ADB 设备序列号，为空时使用第一个已连接设备 |
| `ServerPath` | `""` | scrcpy-server jar 的本地路径 |
| `ServerVersion` | `"3.1"` | 必须与 jar 的版本号一致 |
| `Video` | `true` | 启用视频流 |
| `Audio` | `true` | 启用音频流 |
| `Control` | `true` | 启用设备控制 |
| `VideoCodec` | `"h264"` | `"h264"`、`"h265"`、`"av1"` |
| `AudioCodec` | `"opus"` | `"opus"`、`"aac"`、`"flac"`、`"raw"` |
| `VideoBitRate` | `8000000` | 视频码率（bps） |
| `AudioBitRate` | `128000` | 音频码率（bps） |
| `MaxSize` | `0` | 视频最大边长，0 表示不限制 |
| `MaxFPS` | `""` | 最大帧率，如 `"60"` |
| `ForceADBForward` | `false` | 强制使用 adb forward 而非 reverse 隧道 |
| `PortRange` | `[27183,27199]` | 隧道 TCP 端口范围 |
| `ShowTouches` | `false` | 在设备上显示触摸点 |
| `StayAwake` | `false` | 连接期间保持设备亮屏 |
| `PowerOffOnClose` | `false` | 断开连接时关闭屏幕 |
| `ClipboardAutoSync` | `true` | 自动同步剪贴板 |
| `PowerOn` | `true` | 连接时自动点亮屏幕 |

---

## 项目结构

```
GoScrcpy/
├── adb/          # ADB 工具（列设备、推送文件、端口映射、启动服务端）
├── client/       # 高层 scrcpy 客户端（Options、Client、Demuxer、Controller）
├── protocol/     # 传输协议：数据包、编解码器、控制消息、二进制工具函数
└── example/      # 独立 HTTP 服务器，内嵌 Web 界面
    ├── main.go
    └── index.html
```

---

## 许可证

本项目为开源项目，详见 [LICENSE](LICENSE)（如存在）。
