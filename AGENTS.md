# AirIn (隔空输入) — Agent Guide

## Build & Run
- Build: `go build ./cmd/ime/`
- Run: `go run ./cmd/ime/`
- No Makefile, no Docker, no CI, no lint config, no tests.

## Architecture
- **Single Go module** (`airin`), no monorepo.
- Entrypoint: `cmd/ime/main.go`
- `internal/` layout: one package per concern — `config`, `state`, `hotkey`, `tray`, `asr`, `audio`, `inject`.

## Platform build tags
- Hotkey: `//go:build darwin` (`platform_darwin.go`) and `//go:build linux` (`platform_linux.go`). Linux reads raw `/dev/input/event*`; macOS uses `robotn/gohook`.
- Audio: `capture_darwin.go` / `capture_linux.go`. Currently shells out to system tools (arecord/parec/ffmpeg/sox) — portaudio replacement planned.
- Inject: `inject_darwin.go` (CGO, CoreGraphics) / `inject_linux.go` (xdotool/wtype/ydotool).
- D-Bus tray works on both Linux and macOS (macOS has D-Bus if brew-installed).

## Config (`~/.config/ime/config.json`)
- Required: `api_key` (火山引擎 ASR token)
- Optional: `secret_key`, `app_id`, `cluster` (defaults to `volcengine_input_edu`)
- Default hotkey: `Ctrl+Shift+V`

## Key dependencies
- `github.com/godbus/dbus/v5` — raw D-Bus SNI protocol for system tray
- `github.com/robotn/gohook` — global keyboard hook (macOS only)
- `github.com/gorilla/websocket` — WebSocket client for ASR

## State machine
- 4 states: `Unconfigured` → `Idle` → `Listening` → `Transcribing` → back to `Idle`
- `recEngine` in main.go manages the audio→ASR→inject pipeline lifecycle.

## ASR protocol (`internal/asr/`)
- Volcengine/Doubao ASR v2 bidirectional streaming over WebSocket (`wss://openspeech.bytedance.com/api/v2/asr`).
- Binary frame headers detailed in `protocol.go` (8-byte header per DESIGN.md).
- Auth: token-based (via Full Client Request body) or HMAC-SHA256 WebSocket upgrade headers if `secret_key` is configured.
- Reading/writing runs in separate goroutines; `Results()` channel yields `RecognitionResult`.

## Audio capture (`internal/audio/`)
- Platform-specific shell-out: `arecord`/`parec` on Linux, `ffmpeg`/`sox` on macOS.
- Output: `<-chan []int16`, 16kHz mono PCM chunks (~100ms each).
- `audio.Config` validates rate/channels/bit depth.

## Text injection (`internal/inject/`)
- macOS: CGO CoreGraphics `CGEventPost` with unicode strings.
- Linux: `xdotool type` (X11), `wtype`/`ydotool` (Wayland), or clipboard-paste fallback.
- Incremental diff: intermediate results (`definite=false`) use `computeDelta` to only send changed characters; final results (`definite=true`) are committed.

## Conventions
- Chinese comments and docs throughout.
- Hotkey key names normalized via `hotkey.NormalizeKey()` — aliases like `ctrl`/`control`, `cmd`/`super`/`win`/`meta`.
