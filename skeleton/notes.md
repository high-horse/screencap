screen for x11 but black screen in wayland
```bash
gst-launch-1.0 ximagesrc use-damage=0 ! videoconvert ! x264enc speed-preset=ultrafast tune=zerolatency ! mp4mux faststart=true fragment-duration=1000 ! filesink location=fast.mp4
```

captures cam in wayland
```bash
gst-launch-1.0 -e pipewiresrc ! videoconvert ! \
x264enc speed-preset=ultrafast tune=zerolatency ! \
mp4mux faststart=true fragment-duration=1000 ! \
filesink location=fast.mp4
```

captures cam in wayland
```bash
gst-launch-1.0 pipewiresrc ! videoconvert ! x264enc speed-preset=ultrafast tune=zerolatency ! mp4mux faststart=true fragment-duration=1000 ! filesink location=fast.mp4
```

captures cam in wayland but media unplayable add `-e` at last to make it playable
```bash
gst-launch-1.0 pipewiresrc ! videoconvert ! x264enc ! mp4mux ! filesink location=screen.mp4
```

captures cam in wayland
```bash
gst-launch-1.0 pipewiresrc ! videoconvert ! x264enc tune=zerolatency speed-preset=ultrafast ! mp4mux ! filesink location=fast.mp4 -e
```

```sh
screencast/
│
├── cmd/
│   └── demo/
│       └── main.go        # example CLI usage (your current main)
│
├── portal/
│   ├── client.go         # ScreenCastPortal struct + constructor
│   ├── session.go        # CreateSession, SelectSources, Start
│   ├── pipewire.go       # OpenPipeWireRemote
│   ├── signals.go        # D-Bus signal handling helpers
│   └── types.go          # shared structs / constants
│
├── dbusutil/
│   ├── conn.go           # session bus wrapper helpers
│   ├── match.go          # AddMatch / RemoveMatch helpers
│
├── pipeline/
│   ├── gstreamer.go      # startRecording functions
│   ├── encoders.go       # x264 / future nvenc/vaapi configs
│   └── hls.go            # optional HLS pipeline builder
│
├── config/
│   └── config.go         # bitrate, encoder settings, defaults
│
├── internal/
│   ├── util/
│   │   ├── token.go      # request/session token generation
│   │   └── timeout.go    # shared timeout helpers
│   │
│   └── errors/
│       └── errors.go     # custom error types
│
├── go.mod
└── README.md
```

### What goes where (mapped from your code)
1. `portal/` (core of your logic)

Move this whole struct here:

- ScreenCastPortal
- CreateSession
- SelectSources
- Start
- GetAvailableSourceTypes
- OpenPipeWireRemote
- parseStreams
- requestPath

This becomes your public API layer

2. `dbusutil/` (cleanup D-Bus boilerplate)

Extract repeated logic like:

- AddMatch
- RemoveMatch
- Signal channel setup
- session bus connection

So your portal code becomes clean and readable.

3. `pipeline/` (media logic)

Move:

Current:
```go
startRecording()
startRecording_()
```
Into:
```go
GStreamerPipeWireRecorder
BuildHLSPipeline()
BuildRTCPipeline()
```

This is where you later plug:

- x264enc
- nvh264enc
- vaapih264enc
- HLS
- WebRTC
4. `cmd/demo/` (your current main)

Your current `main()` becomes:

- just orchestration
- example usage
- CLI demo

No D-Bus logic here anymore.

5. `internal/` (helpers)

Put:

- token generation (u%d)
- timeout helpers
- minor parsing utilities
- error wrapping
### What the final API should feel like

After refactor, users should be able to do:
```go
p, _ := portal.New()

session, _ := p.CreateSession()
p.SelectSources(session)

node, props, _ := p.Start(session)

fd, _ := p.OpenPipeWireRemote(session)

rec := pipeline.NewRecorder(fd, node)
rec.StartH264("output.mkv")
```

### Why this structure works
- portal/ → control plane (Wayland portal)
- pipeline/ → media plane (GStreamer / encoding / HLS / RTC)
- dbusutil/ → system glue
- cmd/ → examples only

This separation makes it easy to later add:

- WebRTC (webrtcbin)
- HLS (hlssink2)
- hardware encoders (NVENC/VAAPI)
- remote streaming APIs