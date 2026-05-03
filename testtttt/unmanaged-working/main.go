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

const portalBusName = "org.freedesktop.portal.Desktop"
const portalObjectPath = "/org/freedesktop/portal/desktop"
const portalInterface = "org.freedesktop.portal.ScreenCast"

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

	// Connect to D-Bus
	conn, err := dbus.SessionBus()
	if err != nil {
		log.Fatal("Failed to connect to session bus:", err)
	}

	// Check available source types
	sources, err := getAvailableSourceTypes(conn)
	if err != nil {
		fmt.Printf("Warning: Could not get available source types: %v\n", err)
	} else {
		fmt.Printf("Available source types: %d (1=Monitor, 2=Window, 4=Virtual)\n", sources)
	}

	// Create session
	sessionPath, err := createSession(conn)
	if err != nil {
		log.Fatal("Failed to create session:", err)
	}
	fmt.Println("✓ Session created:", sessionPath)

	// Select sources
	if err := selectSources(conn, sessionPath); err != nil {
		log.Fatal("Failed to select sources:", err)
	}
	fmt.Println("✓ Sources selected")

	// Start session (this shows the dialog)
	nodeID, streamProps, err := startSession(conn, sessionPath)
	if err != nil {
		log.Fatal("Failed to start session:", err)
	}
	fmt.Printf("✓ Screen sharing started!\n")
	fmt.Printf("  PipeWire Node ID: %d\n", nodeID)

	// Print stream properties
	if size, ok := streamProps["size"]; ok {
		fmt.Printf("  Stream size: %+v\n", size.Value())
	}
	if position, ok := streamProps["position"]; ok {
		fmt.Printf("  Position: %+v\n", position.Value())
	}
	if sourceType, ok := streamProps["source_type"]; ok {
		fmt.Printf("  Source type: %d (1=Monitor, 2=Window, 4=Virtual)\n", sourceType.Value())
	}

	// Open PipeWire remote
	pwFd, err := openPipeWireRemote(conn, sessionPath)
	if err != nil {
		fmt.Printf("⚠ Warning: Could not open PipeWire remote: %v\n", err)
		fmt.Printf("  You may need to connect to PipeWire manually\n")
	} else {
		fmt.Printf("✓ PipeWire remote FD: %d\n", pwFd)
		fmt.Printf("  Use this file descriptor to connect to PipeWire\n")
	}

	recCmd, err := startRecordingMKV(pwFd, nodeID, "output.mp4")
	if err != nil {
		log.Fatal(err)
	}

	fmt.Println("🎥 Recording started → output.mp4")
	fmt.Println("\n📺 Screen sharing is active!")
	fmt.Println("Press Ctrl+C to stop and exit...")

	// Wait for interrupt
	<-ctx.Done()
	fmt.Println("\nScreen sharing stopped.")

	// Graceful shutdown: Send SIGTERM first, wait for process to exit
	if recCmd != nil && recCmd.Process != nil {
		fmt.Println("Sending SIGTERM to GStreamer...")
		if err := recCmd.Process.Signal(os.Interrupt); err != nil {
			fmt.Printf("Warning: failed to send SIGTERM: %v\n", err)
		}

		// Wait up to 5 seconds for GStreamer to finalize the MP4 file
		done := make(chan error, 1)
		go func() {
			done <- recCmd.Wait()
		}()

		select {
		case err := <-done:
			if err != nil {
				fmt.Printf("GStreamer exited with error: %v\n", err)
			} else {
				fmt.Println("✅ GStreamer exited cleanly, MP4 finalized")
			}
		case <-time.After(5 * time.Second):
			fmt.Println("⚠ GStreamer did not exit in time, forcing kill...")
			recCmd.Process.Kill()
			recCmd.Wait()
		}
	}

	fmt.Println("✅ Recording saved to output.mp4")
}

func startRecordingMKV(pwFd int, nodeID uint32, output string) (*exec.Cmd, error) {
	file := os.NewFile(uintptr(pwFd), "pipewire")

	// Ensure output has .mkv extension
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

func startRecording(pwFd int, nodeID uint32, output string) (*exec.Cmd, error) {
	file := os.NewFile(uintptr(pwFd), "pipewire")

	cmd := exec.Command(
		"gst-launch-1.0",
		"pipewiresrc", "fd=3", fmt.Sprintf("path=%d", nodeID),
		"!", "videoconvert",
		"!", "x264enc", "bitrate=5000", "speed-preset=ultrafast", "key-int-max=30",
		"!", "mp4mux", "faststart=true",
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

func getAvailableSourceTypes(conn *dbus.Conn) (uint32, error) {
	obj := conn.Object(portalBusName, portalObjectPath)
	var sources uint32
	err := obj.Call("org.freedesktop.DBus.Properties.Get", 0, portalInterface, "AvailableSourceTypes").Store(&sources)
	if err != nil {
		return 0, err
	}
	return sources, nil
}

func createSession(conn *dbus.Conn) (dbus.ObjectPath, error) {
	requestToken := fmt.Sprintf("req%d", time.Now().UnixNano())
	sessionToken := fmt.Sprintf("sess%d", time.Now().UnixNano())

	sender := strings.ReplaceAll(conn.Names()[0], ":", "_")
	sender = strings.ReplaceAll(sender, ".", "_")
	requestPath := dbus.ObjectPath(fmt.Sprintf("/org/freedesktop/portal/desktop/request/%s/%s", sender, requestToken))

	matchRule := fmt.Sprintf("type='signal',interface='org.freedesktop.portal.Request',member='Response',path='%s'", requestPath)
	if err := conn.BusObject().Call("org.freedesktop.DBus.AddMatch", 0, matchRule).Err; err != nil {
		return "", fmt.Errorf("failed to add match: %v", err)
	}
	defer conn.BusObject().Call("org.freedesktop.DBus.RemoveMatch", 0, matchRule)

	signalCh := make(chan *dbus.Signal, 1)
	conn.Signal(signalCh)
	defer conn.Signal(nil)

	obj := conn.Object(portalBusName, portalObjectPath)
	call := obj.Call(portalInterface+".CreateSession", 0, map[string]interface{}{
		"session_handle_token": sessionToken,
		"handle_token":         requestToken,
	})
	if call.Err != nil {
		return "", fmt.Errorf("CreateSession call failed: %v", call.Err)
	}

	timeout := time.After(30 * time.Second)
	for {
		select {
		case sig := <-signalCh:
			if sig.Name == "org.freedesktop.portal.Request.Response" {
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
			}
		case <-timeout:
			return "", fmt.Errorf("timeout waiting for session creation")
		}
	}
}

func selectSources(conn *dbus.Conn, sessionPath dbus.ObjectPath) error {
	requestToken := fmt.Sprintf("req%d", time.Now().UnixNano())

	sender := strings.ReplaceAll(conn.Names()[0], ":", "_")
	sender = strings.ReplaceAll(sender, ".", "_")
	requestPath := dbus.ObjectPath(fmt.Sprintf("/org/freedesktop/portal/desktop/request/%s/%s", sender, requestToken))

	matchRule := fmt.Sprintf("type='signal',interface='org.freedesktop.portal.Request',member='Response',path='%s'", requestPath)
	if err := conn.BusObject().Call("org.freedesktop.DBus.AddMatch", 0, matchRule).Err; err != nil {
		return fmt.Errorf("failed to add match: %v", err)
	}
	defer conn.BusObject().Call("org.freedesktop.DBus.RemoveMatch", 0, matchRule)

	signalCh := make(chan *dbus.Signal, 1)
	conn.Signal(signalCh)
	defer conn.Signal(nil)

	obj := conn.Object(portalBusName, portalObjectPath)
	call := obj.Call(portalInterface+".SelectSources", 0, sessionPath, map[string]interface{}{
		"handle_token": requestToken,
		"types":        uint32(1 | 2),
		"multiple":     false,
		"cursor_mode":  uint32(2),
	})
	if call.Err != nil {
		return fmt.Errorf("SelectSources call failed: %v", call.Err)
	}

	timeout := time.After(30 * time.Second)
	for {
		select {
		case sig := <-signalCh:
			if sig.Name == "org.freedesktop.portal.Request.Response" {
				response := sig.Body[0].(uint32)
				if response != 0 {
					return fmt.Errorf("source selection failed with code %d", response)
				}
				return nil
			}
		case <-timeout:
			return fmt.Errorf("timeout waiting for source selection")
		}
	}
}

func startSession(conn *dbus.Conn, sessionPath dbus.ObjectPath) (uint32, map[string]dbus.Variant, error) {
	requestToken := fmt.Sprintf("req%d", time.Now().UnixNano())

	sender := strings.ReplaceAll(conn.Names()[0], ":", "_")
	sender = strings.ReplaceAll(sender, ".", "_")
	requestPath := dbus.ObjectPath(fmt.Sprintf("/org/freedesktop/portal/desktop/request/%s/%s", sender, requestToken))

	matchRule := fmt.Sprintf("type='signal',interface='org.freedesktop.portal.Request',member='Response',path='%s'", requestPath)
	if err := conn.BusObject().Call("org.freedesktop.DBus.AddMatch", 0, matchRule).Err; err != nil {
		return 0, nil, fmt.Errorf("failed to add match: %v", err)
	}
	defer conn.BusObject().Call("org.freedesktop.DBus.RemoveMatch", 0, matchRule)

	signalCh := make(chan *dbus.Signal, 1)
	conn.Signal(signalCh)
	defer conn.Signal(nil)

	obj := conn.Object(portalBusName, portalObjectPath)
	parentWindow := ""

	fmt.Println("  Opening GNOME screen sharing dialog...")
	fmt.Println("  Please select a monitor or window to share.")

	call := obj.Call(portalInterface+".Start", 0, sessionPath, parentWindow, map[string]interface{}{
		"handle_token": requestToken,
	})
	if call.Err != nil {
		return 0, nil, fmt.Errorf("Start call failed: %v", call.Err)
	}

	timeout := time.After(60 * time.Second)
	for {
		select {
		case sig := <-signalCh:
			if sig.Name == "org.freedesktop.portal.Request.Response" {
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

				streamsValue, hasStreams := results["streams"]
				if !hasStreams {
					return 0, nil, fmt.Errorf("no 'streams' key in results")
				}

				streamsData := streamsValue.Value()

				var nodeID uint32
				var streamProps map[string]dbus.Variant

				switch v := streamsData.(type) {
				case []interface{}:
					if len(v) == 0 {
						return 0, nil, fmt.Errorf("no streams returned - user probably cancelled")
					}
					if firstStream, ok := v[0].([]interface{}); ok {
						if len(firstStream) < 2 {
							return 0, nil, fmt.Errorf("stream tuple has %d elements, expected at least 2", len(firstStream))
						}
						switch id := firstStream[0].(type) {
						case uint32:
							nodeID = id
						case int:
							nodeID = uint32(id)
						case int32:
							nodeID = uint32(id)
						default:
							return 0, nil, fmt.Errorf("node ID is not uint32, got %T", firstStream[0])
						}
						if props, ok := firstStream[1].(map[string]dbus.Variant); ok {
							streamProps = props
						} else {
							return 0, nil, fmt.Errorf("stream properties is not map, got %T", firstStream[1])
						}
					} else {
						if len(v) < 2 {
							return 0, nil, fmt.Errorf("stream tuple has %d elements", len(v))
						}
						switch id := v[0].(type) {
						case uint32:
							nodeID = id
						case int:
							nodeID = uint32(id)
						case int32:
							nodeID = uint32(id)
						default:
							return 0, nil, fmt.Errorf("node ID is not uint32, got %T", v[0])
						}
						if props, ok := v[1].(map[string]dbus.Variant); ok {
							streamProps = props
						} else {
							return 0, nil, fmt.Errorf("stream properties is not map, got %T", v[1])
						}
					}
				case [][]interface{}:
					if len(v) == 0 {
						return 0, nil, fmt.Errorf("no streams returned - user probably cancelled")
					}
					firstStream := v[0]
					if len(firstStream) < 2 {
						return 0, nil, fmt.Errorf("stream tuple has %d elements", len(firstStream))
					}
					switch id := firstStream[0].(type) {
					case uint32:
						nodeID = id
					case int:
						nodeID = uint32(id)
					case int32:
						nodeID = uint32(id)
					default:
						return 0, nil, fmt.Errorf("node ID is not uint32, got %T", firstStream[0])
					}
					if props, ok := firstStream[1].(map[string]dbus.Variant); ok {
						streamProps = props
					} else {
						return 0, nil, fmt.Errorf("stream properties is not map, got %T", firstStream[1])
					}
				default:
					return 0, nil, fmt.Errorf("unexpected streams data type: %T", streamsData)
				}

				return nodeID, streamProps, nil
			}
		case <-timeout:
			return 0, nil, fmt.Errorf("timeout waiting for user to select sources")
		}
	}
}

func openPipeWireRemote(conn *dbus.Conn, sessionPath dbus.ObjectPath) (int, error) {
	obj := conn.Object(portalBusName, portalObjectPath)
	var fd dbus.UnixFD
	err := obj.Call(portalInterface+".OpenPipeWireRemote", 0, sessionPath, map[string]interface{}{}).Store(&fd)
	if err != nil {
		return -1, fmt.Errorf("OpenPipeWireRemote failed: %v", err)
	}
	return int(fd), nil
}