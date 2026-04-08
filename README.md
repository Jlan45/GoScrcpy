# GoScrcpy

A Go implementation of a [scrcpy](https://github.com/Genymobile/scrcpy) client. GoScrcpy lets you mirror and control Android devices from a Go program or a web browser, without installing any app on the device.

[中文文档](README_zh.md)

---

## Features

- **Video streaming** — H.264, H.265, AV1
- **Audio streaming** — Opus, AAC, FLAC, raw PCM
- **Device control** — touch, scroll, key events, back/home/recents/power buttons
- **Multi-device** — manage multiple ADB devices simultaneously
- **Web UI** — built-in browser interface with WebCodecs-accelerated decoding
- **REST + WebSocket API** — integrate with any frontend or service
- **Auto-connect** — connect a device at startup via environment variable

---

## Requirements

- Go 1.24+
- [ADB](https://developer.android.com/studio/command-line/adb) installed and in `PATH`
- [scrcpy-server jar](https://github.com/Genymobile/scrcpy/releases) (version **3.3.4** recommended)
- Android device with USB debugging enabled

---

## Quick Start

### 1. Clone and build the example server

```bash
git clone https://github.com/Jlan45/GoScrcpy.git
cd GoScrcpy/example
go build -o goscrcpy .
```

### 2. Download the scrcpy server jar

Download `scrcpy-server-v3.3.4` from the [scrcpy releases page](https://github.com/Genymobile/scrcpy/releases/tag/v3.3.4) and place it in the `example/` directory (or anywhere you like).

### 3. Run the server

```bash
SCRCPY_SERVER_PATH=/path/to/scrcpy-server ./goscrcpy
```

Open your browser at **http://localhost:8080**.

Optional environment variables:

| Variable | Default | Description |
|---|---|---|
| `PORT` | `8080` | HTTP listen port |
| `SCRCPY_SERVER_PATH` | *(empty)* | Path to the scrcpy-server jar. If empty, the jar must already be present on the device at `/data/local/tmp/scrcpy-server.jar`. |
| `DEVICE_SERIAL` | *(empty)* | ADB serial to auto-connect on startup |

---

## Web UI

The built-in web interface (served at `/`) lets you:

- **Refresh** the list of ADB-detected devices
- **Connect / disconnect** individual devices
- View the live device screen decoded in the browser via WebCodecs (H.264/H.265/AV1)
- Send **touch**, **scroll**, **Back**, **Home**, **Recents**, and **Power** inputs

---

## REST API

All endpoints return JSON.

| Method | Path | Description |
|---|---|---|
| `GET` | `/api/devices` | List ADB devices and their connection status |
| `POST` | `/api/connect?serial=<serial>` | Start a scrcpy session for the device |
| `POST` | `/api/disconnect?serial=<serial>` | Stop the scrcpy session for the device |
| `GET` | `/api/sessions` | List active scrcpy sessions |

### Example

```bash
# List devices
curl http://localhost:8080/api/devices

# Connect a device
curl -X POST "http://localhost:8080/api/connect?serial=emulator-5554"

# Disconnect
curl -X POST "http://localhost:8080/api/disconnect?serial=emulator-5554"
```

---

## WebSocket API

Connect to `ws://localhost:8080/ws?serial=<serial>` after the device is connected.

### Messages from server → client

| Byte 0 | Meaning | Payload |
|---|---|---|
| `0x00` | Video packet | `byte[1]` flags (`0x01`=keyframe, `0x02`=config), `uint64 BE` PTS, raw NAL data |
| `0x02` | Device info | JSON: `{"serial","device","width","height","codec"}` |

### Messages from client → server (control)

| Byte 0 | Action | Payload |
|---|---|---|
| `0` | Touch | `action(1) x(4) y(4) sw(2) sh(2)` — all big-endian |
| `1` | Key | `action(1) keycode(4)` |
| `2` | Scroll | `x(4) y(4) sw(2) sh(2) hscroll(2) vscroll(2)` |
| `3` | Back | — |
| `4` | Home | — |
| `5` | Recents | — |
| `6` | Power | — |

---

## Using the Go Library

You can embed the `client` package directly in your own Go program.

```go
import (
    "context"
    "github.com/Jlan45/GoScrcpy/client"
)

opts := client.DefaultOptions()
opts.Serial        = "emulator-5554"    // ADB device serial
opts.ServerPath    = "/path/to/scrcpy-server"
opts.ServerVersion = "3.3.4"
opts.VideoCodec    = "h264"             // "h264", "h265", "av1"
opts.MaxSize       = 1080               // limit max screen dimension

c := client.NewClient(opts)
if err := c.Start(context.Background()); err != nil {
    log.Fatal(err)
}
defer c.Stop()

// Consume video packets
for pkt := range c.VideoStream() {
    // pkt.Data      — raw NAL data
    // pkt.IsConfig  — SPS/PPS config packet
    // pkt.IsKeyFrame
    // pkt.PTS
}
```

### `client.Options` reference

| Field | Default | Description |
|---|---|---|
| `Serial` | `""` | ADB device serial. Empty = first connected device. |
| `ServerPath` | `""` | Local path to the scrcpy-server jar |
| `ServerVersion` | `"3.1"` | Must match the jar's version |
| `Video` | `true` | Enable video stream |
| `Audio` | `true` | Enable audio stream |
| `Control` | `true` | Enable device control |
| `VideoCodec` | `"h264"` | `"h264"`, `"h265"`, `"av1"` |
| `AudioCodec` | `"opus"` | `"opus"`, `"aac"`, `"flac"`, `"raw"` |
| `VideoBitRate` | `8000000` | Video bitrate in bps |
| `AudioBitRate` | `128000` | Audio bitrate in bps |
| `MaxSize` | `0` | Max video dimension (0 = no limit) |
| `MaxFPS` | `""` | Max frame rate, e.g. `"60"` |
| `ForceADBForward` | `false` | Force adb forward instead of reverse tunnel |
| `PortRange` | `[27183,27199]` | TCP port range for the tunnel |
| `ShowTouches` | `false` | Show touch indicators on device |
| `StayAwake` | `false` | Keep device awake while connected |
| `PowerOffOnClose` | `false` | Power off screen when disconnected |
| `ClipboardAutoSync` | `true` | Sync clipboard automatically |
| `PowerOn` | `true` | Power on screen when connecting |

---

## Project Structure

```
GoScrcpy/
├── adb/          # ADB helpers (list devices, push, forward/reverse, start server)
├── client/       # High-level scrcpy client (Options, Client, Demuxer, Controller)
├── protocol/     # Wire protocol: packets, codecs, control messages, binary helpers
└── example/      # Standalone HTTP server with embedded web UI
    ├── main.go
    └── index.html
```

---

## License

This project is open source. See [LICENSE](LICENSE) for details (if present).
