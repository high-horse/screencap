### The code uses three layers working together:
1 D-Bus Portal API — Requests permission to capture screen from the desktop environment
2 PipeWire — Receives the actual video frames from the compositor
3 GStreamer — Encodes the raw video into MKV file


### Step-by-Step Flow
```plain
┌─────────────────────────────────────────────────────────────────────────┐
│  1. CONNECT TO D-BUS                                                     │
│     • Connects to the session bus (user's desktop session)               │
│     • This is the communication channel to talk to GNOME/KDE/Portal     │
└─────────────────────────────────────────────────────────────────────────┘
                                    ↓
┌─────────────────────────────────────────────────────────────────────────┐
│  2. CREATE SESSION                                                       │
│     • Calls `CreateSession` on org.freedesktop.portal.ScreenCast         │
│     • Portal returns a session handle (like a "reservation ID")          │
│     • Waits for async D-Bus signal response                              │
└─────────────────────────────────────────────────────────────────────────┘
                                    ↓
┌─────────────────────────────────────────────────────────────────────────┐
│  3. SELECT SOURCES                                                       │
│     • Calls `SelectSources` with:                                        │
│       - types: Monitor (1) + Window (2)                                │
│       - cursor_mode: Embedded (2) — captures cursor too                  │
│     • This tells portal WHAT we want to capture (not WHICH yet)          │
└─────────────────────────────────────────────────────────────────────────┘
                                    ↓
┌─────────────────────────────────────────────────────────────────────────┐
│  4. START SESSION → USER DIALOG APPEARS                                  │
│     • Calls `Start` — THIS TRIGGERS THE GUI DIALOG                      │
│     • GNOME/KDE shows: "Share your screen" with monitor/window picker    │
│     • User clicks "Share" → portal returns stream info                   │
│                                                                        │
│     Response contains:                                                  │
│     • nodeID — PipeWire node ID (the actual video stream)               │
│     • streamProps — size, position, source_type                         │
└─────────────────────────────────────────────────────────────────────────┘
                                    ↓
┌─────────────────────────────────────────────────────────────────────────┐
│  5. OPEN PIPEWIRE REMOTE                                                 │
│     • Calls `OpenPipeWireRemote` — portal gives us a file descriptor (FD)│
│     • This FD is a direct connection to PipeWire daemon                  │
│     • We'll pass this FD to GStreamer so it can read frames              │
└─────────────────────────────────────────────────────────────────────────┘
                                    ↓
┌─────────────────────────────────────────────────────────────────────────┐
│  6. START GSTREAMER RECORDING                                            │
│                                                                        │
│     GStreamer pipeline:                                                  │
│                                                                        │
│     pipewiresrc fd=3 path=<nodeID>                                     │
│          ↓                                                              │
│     videoconvert    ← converts raw PipeWire format to something encodable│
│          ↓                                                              │
│     x264enc         ← H.264 video encoder (compresses video)            │
│          ↓                                                              │
│     matroskamux     ← wraps in MKV container                            │
│          ↓                                                              │
│     filesink        ← writes to output.mkv                              │
│                                                                        │
│     Key trick: `fd=3` + `ExtraFiles = []*os.File{file}`                  │
│     • FD from PipeWire is passed as file descriptor #3 to GStreamer     │
│     • GStreamer reads video frames directly from it                     │
└─────────────────────────────────────────────────────────────────────────┘
                                    ↓
┌─────────────────────────────────────────────────────────────────────────┐
│  7. WAIT FOR CTRL+C                                                      │
│     • Blocks until user presses Ctrl+C                                   │
│     • Context is cancelled → triggers shutdown                          │
└─────────────────────────────────────────────────────────────────────────┘
                                    ↓
┌─────────────────────────────────────────────────────────────────────────┐
│  8. GRACEFUL SHUTDOWN                                                    │
│     • Sends SIGTERM (Interrupt) to GStreamer                             │
│     • Waits up to 5 seconds for GStreamer to:                           │
│       - Flush remaining frames                                          │
│       - Finalize MKV file                                               │
│       - Close file properly                                             │
│     • If it doesn't exit in time → force kill                           │
│                                                                        │
│     MKV advantage: Even if force-killed, file is usually playable         │
│     (unlike MP4 which needs final moov atom written at end)              │
└─────────────────────────────────────────────────────────────────────────┘
```

### Visual Data Flow
```plain
┌─────────────┐     D-Bus API      ┌─────────────┐
│    Go       │ ─────────────────→ │   Portal    │
│    Code     │                    │  (GNOME/KDE)│
│             │ ←─session_handle── │             │
└─────────────┘                    └──────┬──────┘
                                        │
                              User picks screen/window
                                        │
                              ┌─────────▼─────────┐
                              │   PipeWire Daemon  │
                              │  (video transport)  │
                              └─────────┬─────────┘
                                        │
                              ┌─────────▼─────────┐
                              │   GStreamer        │
                              │  (encode to MKV)   │
                              └─────────┬─────────┘
                                        ↓
                                   output.mkv
```

##### Why D-Bus Signals?
The portal API is asynchronous. When you call CreateSession, it doesn't immediately return the result. Instead:
- Register a signal listener for Response on a specific request path
- Make the call
- Portal processes it and emits a signal back

Code catches the signal and extracts the result
This is why the code has all that signalCh, matchRule, AddMatch boilerplate — it's waiting for the portal's "callback".

##### The PipeWire FD Trick
```go
file := os.NewFile(uintptr(pwFd), "pipewire")  // wrap FD as *os.File
cmd.ExtraFiles = []*os.File{file}              // becomes fd=3 in child process
```

In Unix, child processes inherit file descriptors. By default a process has:
- fd 0 = stdin
- fd 1 = stdout
- fd 2 = stderr

ExtraFiles adds more starting at fd 3. So GStreamer's pipewiresrc fd=3 reads from that exact PipeWire connection.


##### Summary Table

| Step | Function               | What It Does                     |
| ---- | ---------------------- | -------------------------------- |
| 1    | `main()`               | Setup, connect D-Bus             |
| 2    | `createSession()`      | Get session handle from portal   |
| 3    | `selectSources()`      | Configure what to capture        |
| 4    | `startSession()`       | Show GUI dialog, get stream info |
| 5    | `openPipeWireRemote()` | Get FD to actual video stream    |
| 6    | `startRecording()`     | Launch GStreamer to encode       |
| 7    | `<-ctx.Done()`         | Wait for Ctrl+C                  |
| 8    | Shutdown               | Gracefully stop GStreamer        |
