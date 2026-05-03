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

// ─── Main ─────────────────────────────────────────────────────────────────

func main() {
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
		fmt.Printf("⚠ Warning: Could not open PipeWire remote: %v\n", err)
	} else {
		fmt.Printf("✓ PipeWire remote FD: %d\n", pwFd)
	}

	// ── Start recording ───────────────────────────────────────────────────
	recCmd, err := startRecording(pwFd, nodeID, "output.mkv")
	if err != nil {
		log.Fatal(err)
	}

	fmt.Println("🎥 Recording started → output.mkv")
	fmt.Println("\n📺 Screen sharing is active!")
	fmt.Println("Press Ctrl+C to stop and exit...")

	// ── Wait for interrupt ────────────────────────────────────────────────
	<-ctx.Done()
	fmt.Println("\nScreen sharing stopped.")

	// ── Graceful shutdown ─────────────────────────────────────────────────
	if recCmd != nil && recCmd.Process != nil {
		fmt.Println("Sending SIGTERM to GStreamer...")
		recCmd.Process.Signal(os.Interrupt)

		done := make(chan error, 1)
		go func() { done <- recCmd.Wait() }()

		select {
		case err := <-done:
			if err != nil {
				fmt.Printf("GStreamer exited with error: %v\n", err)
			} else {
				fmt.Println("✅ GStreamer exited cleanly")
			}
		case <-time.After(5 * time.Second):
			fmt.Println("⚠ Timeout, forcing kill...")
			recCmd.Process.Kill()
			recCmd.Wait()
		}
	}

	fmt.Println("✅ Recording saved to output.mkv")
	fmt.Println("\nPlay it with: ffplay output.mkv")
}