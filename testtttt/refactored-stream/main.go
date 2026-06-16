package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/exec"
	"os/signal"
	"strings"
	"time"
	"io"

	 "net/http"
	"github.com/godbus/dbus/v5"
)

// ─── Portal Constants ─────────────────────────────────────────────────────

const (
	portalBusName    = "org.freedesktop.portal.Desktop"
	portalObjectPath = "/org/freedesktop/portal/desktop"
	portalInterface  = "org.freedesktop.portal.ScreenCast"
)

// ─── ScreenCastPortal ─────────────────────────────────────────────────────

// ScreenCastPortal handles all D-Bus communication with the desktop portal.
type ScreenCastPortal struct {
	conn   *dbus.Conn
	sender string
}

// NewScreenCastPortal connects to the session D-Bus and returns a portal client.
func NewScreenCastPortal() (*ScreenCastPortal, error) {
	conn, err := dbus.SessionBus()
	if err != nil {
		return nil, fmt.Errorf("failed to connect to session bus: %w", err)
	}

	// sender = unique name without leading ':', dots replaced with underscores
	sender := strings.TrimPrefix(conn.Names()[0], ":")
	sender = strings.ReplaceAll(sender, ".", "_")

	return &ScreenCastPortal{
		conn:   conn,
		sender: sender,
	}, nil
}

// Close cleans up the D-Bus connection.
func (p *ScreenCastPortal) Close() {
	p.conn.Close()
}

// GetAvailableSourceTypes returns what the portal can capture (monitor/window/virtual).
func (p *ScreenCastPortal) GetAvailableSourceTypes() (uint32, error) {
	obj := p.conn.Object(portalBusName, portalObjectPath)
	var sources uint32
	err := obj.Call("org.freedesktop.DBus.Properties.Get", 0, portalInterface, "AvailableSourceTypes").Store(&sources)
	return sources, err
}

// CreateSession creates a new screen cast session and returns its path.
func (p *ScreenCastPortal) CreateSession() (dbus.ObjectPath, error) {
	reqToken := fmt.Sprintf("u%d", time.Now().UnixNano())
	sessToken := fmt.Sprintf("u%d", time.Now().UnixNano())

	reqPath := p.requestPath(reqToken)

	// CRITICAL: AddMatch must include sender='org.freedesktop.portal.Desktop'
	matchRule := fmt.Sprintf(
		"type='signal',sender='%s',interface='org.freedesktop.portal.Request',member='Response',path='%s'",
		portalBusName, reqPath,
	)
	if err := p.conn.BusObject().Call("org.freedesktop.DBus.AddMatch", 0, matchRule).Err; err != nil {
		return "", fmt.Errorf("failed to add match: %w", err)
	}
	defer p.conn.BusObject().Call("org.freedesktop.DBus.RemoveMatch", 0, matchRule)

	// Setup signal channel BEFORE making the call
	signalCh := make(chan *dbus.Signal, 10)
	p.conn.Signal(signalCh)
	defer p.conn.RemoveSignal(signalCh)

	// Make the call
	obj := p.conn.Object(portalBusName, portalObjectPath)
	call := obj.Call(portalInterface+".CreateSession", 0, map[string]interface{}{
		"session_handle_token": sessToken,
		"handle_token":         reqToken,
	})
	if call.Err != nil {
		return "", fmt.Errorf("CreateSession call failed: %w", call.Err)
	}

	// Debug: print the returned handle
	fmt.Printf("DEBUG: CreateSession returned handle: %v\n", call.Body)

	timeout := time.After(30 * time.Second)
	for {
		select {
		case sig := <-signalCh:
			fmt.Printf("DEBUG: received signal: name=%s path=%s\n", sig.Name, sig.Path)
			if sig.Name != "org.freedesktop.portal.Request.Response" {
				continue
			}
			if string(sig.Path) != string(reqPath) {
				continue
			}

			response := sig.Body[0].(uint32)
			if response != 0 {
				return "", fmt.Errorf("session creation failed with code %d", response)
			}

			results := sig.Body[1].(map[string]dbus.Variant)
			sessionHandleStr, ok := results["session_handle"].Value().(string)
			if !ok {
				return "", fmt.Errorf("session_handle is not a string")
			}

			return dbus.ObjectPath(sessionHandleStr), nil

		case <-timeout:
			return "", fmt.Errorf("timeout waiting for session creation")
		}
	}
}

// SelectSources configures what to capture (monitors, windows, cursor).
func (p *ScreenCastPortal) SelectSources(sessionPath dbus.ObjectPath) error {
	reqToken := fmt.Sprintf("u%d", time.Now().UnixNano())
	reqPath := p.requestPath(reqToken)

	matchRule := fmt.Sprintf(
		"type='signal',sender='%s',interface='org.freedesktop.portal.Request',member='Response',path='%s'",
		portalBusName, reqPath,
	)
	if err := p.conn.BusObject().Call("org.freedesktop.DBus.AddMatch", 0, matchRule).Err; err != nil {
		return fmt.Errorf("failed to add match: %w", err)
	}
	defer p.conn.BusObject().Call("org.freedesktop.DBus.RemoveMatch", 0, matchRule)

	signalCh := make(chan *dbus.Signal, 10)
	p.conn.Signal(signalCh)
	defer p.conn.RemoveSignal(signalCh)

	obj := p.conn.Object(portalBusName, portalObjectPath)
	call := obj.Call(portalInterface+".SelectSources", 0, sessionPath, map[string]interface{}{
		"handle_token": reqToken,
		"types":        uint32(1 | 2), // Monitor + Window
		"multiple":     false,
		"cursor_mode":  uint32(2), // Embedded
	})
	if call.Err != nil {
		return fmt.Errorf("SelectSources call failed: %w", call.Err)
	}

	timeout := time.After(30 * time.Second)
	for {
		select {
		case sig := <-signalCh:
			if sig.Name != "org.freedesktop.portal.Request.Response" {
				continue
			}
			if string(sig.Path) != string(reqPath) {
				continue
			}

			response := sig.Body[0].(uint32)
			if response != 0 {
				return fmt.Errorf("source selection failed with code %d", response)
			}
			return nil

		case <-timeout:
			return fmt.Errorf("timeout waiting for source selection")
		}
	}
}

// Start triggers the GUI dialog for user to pick a screen/window.
// Returns the PipeWire node ID and stream properties.
func (p *ScreenCastPortal) Start(sessionPath dbus.ObjectPath) (uint32, map[string]dbus.Variant, error) {
	reqToken := fmt.Sprintf("u%d", time.Now().UnixNano())
	reqPath := p.requestPath(reqToken)

	matchRule := fmt.Sprintf(
		"type='signal',sender='%s',interface='org.freedesktop.portal.Request',member='Response',path='%s'",
		portalBusName, reqPath,
	)
	if err := p.conn.BusObject().Call("org.freedesktop.DBus.AddMatch", 0, matchRule).Err; err != nil {
		return 0, nil, fmt.Errorf("failed to add match: %w", err)
	}
	defer p.conn.BusObject().Call("org.freedesktop.DBus.RemoveMatch", 0, matchRule)

	signalCh := make(chan *dbus.Signal, 10)
	p.conn.Signal(signalCh)
	defer p.conn.RemoveSignal(signalCh)

	fmt.Println("  Opening screen sharing dialog...")
	fmt.Println("  Please select a monitor or window to share.")

	obj := p.conn.Object(portalBusName, portalObjectPath)
	call := obj.Call(portalInterface+".Start", 0, sessionPath, "", map[string]interface{}{
		"handle_token": reqToken,
	})
	if call.Err != nil {
		return 0, nil, fmt.Errorf("Start call failed: %w", call.Err)
	}

	timeout := time.After(60 * time.Second)
	for {
		select {
		case sig := <-signalCh:
			if sig.Name != "org.freedesktop.portal.Request.Response" {
				continue
			}
			if string(sig.Path) != string(reqPath) {
				continue
			}

			response := sig.Body[0].(uint32)
			if response != 0 {
				return 0, nil, fmt.Errorf("start failed with code %d (user cancelled or error)", response)
			}

			if len(sig.Body) < 2 {
				return 0, nil, fmt.Errorf("no results in response")
			}

			results, ok := sig.Body[1].(map[string]dbus.Variant)
			if !ok {
				return 0, nil, fmt.Errorf("results is not a map, got %T", sig.Body[1])
			}

			return p.parseStreams(results)

		case <-timeout:
			return 0, nil, fmt.Errorf("timeout waiting for user to select sources")
		}
	}
}

// OpenPipeWireRemote gets a file descriptor to connect to PipeWire.
func (p *ScreenCastPortal) OpenPipeWireRemote(sessionPath dbus.ObjectPath) (int, error) {
	obj := p.conn.Object(portalBusName, portalObjectPath)
	var fd dbus.UnixFD
	err := obj.Call(portalInterface+".OpenPipeWireRemote", 0, sessionPath, map[string]interface{}{}).Store(&fd)
	if err != nil {
		return -1, fmt.Errorf("OpenPipeWireRemote failed: %w", err)
	}
	return int(fd), nil
}

// ─── Internal helpers ─────────────────────────────────────────────────────

func (p *ScreenCastPortal) requestPath(token string) dbus.ObjectPath {
	return dbus.ObjectPath(fmt.Sprintf("/org/freedesktop/portal/desktop/request/%s/%s", p.sender, token))
}

// parseStreams extracts nodeID and properties from the streams data.
func (p *ScreenCastPortal) parseStreams(results map[string]dbus.Variant) (uint32, map[string]dbus.Variant, error) {
	streamsValue, hasStreams := results["streams"]
	if !hasStreams {
		return 0, nil, fmt.Errorf("no 'streams' key in results")
	}

	streamsData := streamsValue.Value()

	// Helper to extract from a tuple [nodeID, props]
	extract := func(tuple []interface{}) (uint32, map[string]dbus.Variant, error) {
		if len(tuple) < 2 {
			return 0, nil, fmt.Errorf("stream tuple has %d elements, expected at least 2", len(tuple))
		}

		var nodeID uint32
		switch id := tuple[0].(type) {
		case uint32:
			nodeID = id
		case int:
			nodeID = uint32(id)
		case int32:
			nodeID = uint32(id)
		default:
			return 0, nil, fmt.Errorf("node ID is not uint32, got %T", tuple[0])
		}

		props, ok := tuple[1].(map[string]dbus.Variant)
		if !ok {
			return 0, nil, fmt.Errorf("stream properties is not map, got %T", tuple[1])
		}

		return nodeID, props, nil
	}

	switch v := streamsData.(type) {
	case []interface{}:
		if len(v) == 0 {
			return 0, nil, fmt.Errorf("no streams returned - user probably cancelled")
		}
		if firstStream, ok := v[0].([]interface{}); ok {
			return extract(firstStream)
		}
		return extract(v)

	case [][]interface{}:
		if len(v) == 0 {
			return 0, nil, fmt.Errorf("no streams returned - user probably cancelled")
		}
		tuple := make([]interface{}, len(v[0]))
		for i, x := range v[0] {
			tuple[i] = x
		}
		return extract(tuple)

	default:
		return 0, nil, fmt.Errorf("unexpected streams data type: %T", streamsData)
	}
}



type ScreenStream struct {
    cmd     *exec.Cmd
    stdout  io.Reader
    cancel  context.CancelFunc
    done    chan struct{} // To signal when streaming is done
}

// StartScreenStream starts a screen capture and returns a stream
func StartScreenStream(pwFd int, nodeID uint32) (*ScreenStream, error) {
    ctx, cancel := context.WithCancel(context.Background())
    file := os.NewFile(uintptr(pwFd), "pipewire")

    // Use MPEG-TS with H.264 (better for HTTP streaming)
    cmd := exec.CommandContext(ctx,
        "gst-launch-1.0",
        "pipewiresrc", "fd=3", fmt.Sprintf("path=%d", nodeID),
        "!", "videoconvert",
        "!", "x264enc", "bitrate=2000", "tune=zerolatency", "speed-preset=ultrafast",
        "!", "mpegtsmux", "alignment=7", // Byte-aligned for HTTP
        "!", "filesink", "location=/dev/stdout",
    )

    cmd.ExtraFiles = []*os.File{file}
    stdout, err := cmd.StdoutPipe()
    if err != nil {
        cancel()
        return nil, fmt.Errorf("failed to get stdout pipe: %w", err)
    }

    if err := cmd.Start(); err != nil {
        cancel()
        return nil, fmt.Errorf("failed to start gstreamer: %w", err)
    }

    return &ScreenStream{
        cmd:    cmd,
        stdout: stdout,
        cancel: cancel,
        done:   make(chan struct{}),
    }, nil
}

// Close stops the stream
func (s *ScreenStream) Close() error {
    s.cancel()
    close(s.done) // Signal that streaming is done
    return s.cmd.Wait()
}

// GetReader returns the io.Reader for the stream
func (s *ScreenStream) GetReader() io.Reader {
    return s.stdout
}

// ─── Recording ────────────────────────────────────────────────────────────
func startRecording(pwFd int, nodeID uint32, output string) (*exec.Cmd, error) {
	file := os.NewFile(uintptr(pwFd), "pipewire")

	if !strings.HasSuffix(output, ".mkv") {
		output = output + ".mkv"
	}

	cmd := exec.Command(
		"gst-launch-1.0",
		"pipewiresrc", "fd=3", fmt.Sprintf("path=%d", nodeID),
		"!", "videoconvert",
		"!", "videoscale", "!", "video/x-raw, format=I420", // ensure proper colorspace
		"!", "x264enc",
		"bitrate=20000",       // 20 Mbps — adjust based on resolution
		"speed-preset=medium", // balance of speed/quality
		"tune=zerolatency",    // better for screen content
		"key-int-max=60",      // less frequent keyframes, better compression
		"vbv-buf-capacity=0",  // let encoder manage buffering
		"ref=4",               // more reference frames for quality
		"!", "matroskamux",
		"!", "filesink", fmt.Sprintf("location=%s", output),
	)

	cmd.ExtraFiles = []*os.File{file}
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Start(); err != nil {
		return nil, err
	}

	return cmd, nil
}

func startRecording_(pwFd int, nodeID uint32, output string) (*exec.Cmd, error) {
	file := os.NewFile(uintptr(pwFd), "pipewire")

	if !strings.HasSuffix(output, ".mkv") {
		output = output + ".mkv"
	}

	cmd := exec.Command(
		"gst-launch-1.0",
		"pipewiresrc", "fd=3", fmt.Sprintf("path=%d", nodeID),
		"!", "videoconvert",
		"!", "x264enc", "bitrate=5000", "speed-preset=ultrafast", "key-int-max=30",
		"!", "matroskamux",
		"!", "filesink", fmt.Sprintf("location=%s", output),
	)

	cmd.ExtraFiles = []*os.File{file}
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Start(); err != nil {
		return nil, err
	}

	return cmd, nil
}


func StreamHandler(stream *ScreenStream) http.HandlerFunc {
    return func(w http.ResponseWriter, r *http.Request) {
        // Set headers for MPEG-TS
        w.Header().Set("Content-Type", "video/mp2t")
        w.Header().Set("Connection", "keep-alive")
        w.Header().Set("Cache-Control", "no-cache")
        w.Header().Set("Access-Control-Allow-Origin", "*") // For browser testing

        // Get the reader from the stream
        reader := stream.GetReader()

        // Create a buffer
        buf := make([]byte, 32768) // 32KB buffer

        // Stream in a goroutine so we can handle client disconnects
        done := make(chan struct{})
        go func() {
            defer close(done)
            for {
                n, err := reader.Read(buf)
                if err != nil {
                    if err != io.EOF {
                        log.Printf("Stream read error: %v", err)
                    }
                    return
                }
                if n > 0 {
                    if _, err := w.Write(buf[:n]); err != nil {
                        log.Printf("Stream write error: %v", err)
                        return
                    }
                    if f, ok := w.(http.Flusher); ok {
                        f.Flush()
                    }
                }
            }
        }()

        // Wait for either the stream to end or client to disconnect
        select {
        case <-done:
            return
        case <-r.Context().Done():
            return
        }
    }
}


// ─── Main ─────────────────────────────────────────────────────────────────
func main() {
    if err := CheckDependencies(); err != nil {
        log.Fatal(err)
    }
    ctx, cancel := context.WithCancel(context.Background())
    defer cancel()

    // Handle Ctrl+C
    sigCh := make(chan os.Signal, 1)
    signal.Notify(sigCh, os.Interrupt)
    go func() {
        <-sigCh
        fmt.Println("\nShutting down...")
        cancel()
    }()

    // ── Connect to portal ─────────────────────────────────────────────────
    portal, err := NewScreenCastPortal()
    if err != nil {
        log.Fatal(err)
    }
    defer portal.Close()

    // ── Check capabilities ────────────────────────────────────────────────
    if sources, err := portal.GetAvailableSourceTypes(); err != nil {
        fmt.Printf("Warning: Could not get available source types: %v\n", err)
    } else {
        fmt.Printf("Available source types: %d (1=Monitor, 2=Window, 4=Virtual)\n", sources)
    }

    // ── Create session ────────────────────────────────────────────────────
    sessionPath, err := portal.CreateSession()
    if err != nil {
        log.Fatal("Failed to create session:", err)
    }
    fmt.Println("✓ Session created:", sessionPath)

    // ── Select sources ────────────────────────────────────────────────────
    if err := portal.SelectSources(sessionPath); err != nil {
        log.Fatal("Failed to select sources:", err)
    }
    fmt.Println("✓ Sources selected")

    // ── Start (shows GUI dialog) ──────────────────────────────────────────
    nodeID, streamProps, err := portal.Start(sessionPath)
    if err != nil {
        log.Fatal("Failed to start session:", err)
    }
    fmt.Printf("✓ Screen sharing started! PipeWire Node ID: %d\n", nodeID)

    if size, ok := streamProps["size"]; ok {
        fmt.Printf("  Stream size: %+v\n", size.Value())
    }
    if position, ok := streamProps["position"]; ok {
        fmt.Printf("  Position: %+v\n", position.Value())
    }
    if sourceType, ok := streamProps["source_type"]; ok {
        fmt.Printf("  Source type: %d (1=Monitor, 2=Window, 4=Virtual)\n", sourceType.Value())
    }

    // ── Open PipeWire remote ──────────────────────────────────────────────
    pwFd, err := portal.OpenPipeWireRemote(sessionPath)
    if err != nil {
        log.Fatal("⚠ Failed to open PipeWire remote: %v\n", err)
    }
    fmt.Printf("✓ PipeWire remote FD: %d\n", pwFd)

    // ── Start the screen stream ───────────────────────────────────────────
    stream, err := StartScreenStream(pwFd, nodeID)
    if err != nil {
        log.Fatal("Failed to start stream:", err)
    }
    defer stream.Close()

    // ── Set up HTTP server ───────────────────────────────────────────────
    http.HandleFunc("/stream", StreamHandler(stream))
    http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintf(w, `
		<html>
		<head>
			<title>Screen Stream</title>
			<script src="https://cdn.jsdelivr.net/npm/hls.js@latest"></script>
			<script src="https://cdn.jsdelivr.net/npm/mpegts.js@1.3.0/dist/mpegts.js"></script>
		</head>
		<body>
			<h1>Screen Stream</h1>
			<video id="video" width="800" controls autoplay playsinline></video>
			<script>
				const video = document.getElementById('video');

				// Try native MPEG-TS first
				video.src = '/stream';
				video.load();

				// Fallback to HLS.js if native doesn't work
				video.addEventListener('error', function() {
					console.log("Trying HLS.js fallback...");

					if (Hls.isSupported()) {
						const hls = new Hls();
						hls.loadSource('/stream');
						hls.attachMedia(video);
						hls.on(Hls.Events.MANIFEST_PARSED, function() {
							video.play().catch(e => console.log("Play error:", e));
						});
					} else if (video.canPlayType('application/vnd.apple.mpegurl')) {
						// Safari native HLS support
						video.src = '/stream';
						video.addEventListener('loadedmetadata', function() {
							video.play().catch(e => console.log("Play error:", e));
						});
					} else {
						console.error("No compatible streaming method found");
					}
				});

				// Try to play immediately
				video.play().catch(e => console.log("Initial play error:", e));
			</script>
		</body>
		</html>`)
	})

    server := &http.Server{
        Addr: ":8080",
        Handler: nil,
    }

    fmt.Println("Server running on :8080")
    fmt.Println("Stream available at http://localhost:8080/stream")
    fmt.Println("HTML page with player available at http://localhost:8080/")

    // Start server in a goroutine
    go func() {
        if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
            log.Printf("HTTP server error: %v\n", err)
        }
    }()

    // Wait for interrupt or context cancellation
    select {
    case <-sigCh:
        fmt.Println("\nShutting down server...")
        if err := server.Shutdown(context.Background()); err != nil {
            log.Printf("Server shutdown error: %v\n", err)
        }
    case <-ctx.Done():
        fmt.Println("\nContext cancelled, shutting down server...")
        if err := server.Shutdown(context.Background()); err != nil {
            log.Printf("Server shutdown error: %v\n", err)
        }
    }

    fmt.Println("✅ Server stopped")
}


func CheckDependencies() error {
	var errs []string
	var missingPackages []string

	// ─── Helper functions ───────────────────────────────────────────────

	checkBinary := func(name string, pkg string) {
		if _, err := exec.LookPath(name); err != nil {
			errs = append(errs, fmt.Sprintf("missing binary: %s", name))
			missingPackages = append(missingPackages, pkg)
		}
	}

	checkEnv := func(key string) {
		if os.Getenv(key) == "" {
			errs = append(errs, fmt.Sprintf("missing environment variable: %s", key))
		}
	}

	// checkEnvEquals := func(key, expected string) {
	// 	val := os.Getenv(key)
	// 	if val == "" {
	// 		errs = append(errs, fmt.Sprintf("missing environment variable: %s", key))
	// 		return
	// 	}
	// 	if val != expected {
	// 		errs = append(errs, fmt.Sprintf("%s must be '%s' (got '%s')", key, expected, val))
	// 	}
	// }

	checkGstPlugin := func(plugin, pkg string) {
		cmd := exec.Command("gst-inspect-1.0", plugin)
		if err := cmd.Run(); err != nil {
			errs = append(errs, fmt.Sprintf("missing GStreamer plugin: %s", plugin))
			missingPackages = append(missingPackages, pkg)
		}
	}

	checkDBusName := func(name string) {
		conn, err := dbus.SessionBus()
		if err != nil {
			errs = append(errs, fmt.Sprintf("failed to connect to D-Bus: %v", err))
			return
		}
		defer conn.Close()

		var names []string
		err = conn.BusObject().Call("org.freedesktop.DBus.ListNames", 0).Store(&names)
		if err != nil {
			errs = append(errs, fmt.Sprintf("failed to list D-Bus names: %v", err))
			return
		}

		found := false
		for _, n := range names {
			if n == name {
				found = true
				break
			}
		}
		if !found {
			errs = append(errs, fmt.Sprintf("D-Bus service not available: %s", name))
		}
	}

	// ─── Checks ─────────────────────────────────────────────────────────

	// 1. Required binaries
	checkBinary("gst-launch-1.0",  "gstreamer1.0-tools / gstreamer1")
	checkBinary("gst-inspect-1.0", "gstreamer1.0-tools / gstreamer1")

	// 2. GStreamer plugins
	checkGstPlugin("pipewiresrc", "gstreamer1.0-plugins-good / gst-plugins-good")
	checkGstPlugin("x264enc", "gstreamer1.0-plugins-ugly / gst-plugins-ugly")

	// 3. Environment (Wayland + D-Bus)
	checkEnv("DBUS_SESSION_BUS_ADDRESS")
	// checkEnv("WAYLAND_DISPLAY")
	// checkEnvEquals("XDG_SESSION_TYPE", "wayland")

	// 4. D-Bus portal availability
	checkDBusName("org.freedesktop.portal.Desktop")

	// ─── Result ─────────────────────────────────────────────────────────

	if len(errs) > 0 {
		distro := detectDistro()
		installCmd := generateInstallCommand(distro, missingPackages)
		
		return fmt.Errorf(
			"dependency check failed:\n - %s\n\nTo install on %s:\n  %s",
			strings.Join(errs, "\n - "),
			distro,
			installCmd,
		)
		// return fmt.Errorf("dependency check failed:\n - %s", strings.Join(errs, "\n - "))
	}

	return nil
}


func detectDistro() string {
	if _, err := os.Stat("/etc/debian_version"); err == nil {
		return "Debian/Ubuntu"
	}
	if _, err := os.Stat("/etc/fedora-release"); err == nil {
		return "Fedora"
	}
	if _, err := os.Stat("/etc/arch-release"); err == nil {
		return "Arch"
	}
	return "your distro"
}


func generateInstallCommand(distro string, pkgs []string) string {
	// Map generic package names to distro-specific ones
	debianMap := map[string]string{
		"gstreamer1.0-tools / gstreamer1": "gstreamer1.0-tools",
		"gstreamer1.0-plugins-good / gst-plugins-good": "gstreamer1.0-plugins-good",
		"gstreamer1.0-plugins-ugly / gst-plugins-ugly": "gstreamer1.0-plugins-ugly",
	}
	
	switch distro {
	case "Debian/Ubuntu":
		var resolved []string
		for _, p := range pkgs {
			if r, ok := debianMap[p]; ok {
				resolved = append(resolved, r)
			}
		}
		return fmt.Sprintf("sudo apt install %s", strings.Join(resolved, " "))
	case "Fedora":
		return fmt.Sprintf("sudo dnf install %s", strings.Join(pkgs, " "))
	case "Arch":
		return fmt.Sprintf("sudo pacman -S %s", strings.Join(pkgs, " "))
	default:
		return "please install: " + strings.Join(pkgs, ", ")
	}
}

