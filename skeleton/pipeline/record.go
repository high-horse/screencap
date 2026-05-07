package pipeline

import (
	"fmt"
	"os"
	"os/exec"
	"screencap/portal"
	"strings"
	"time"
)

func (r *Recorder) Stop() error {
	if r.cmd == nil || r.cmd.Process == nil {
		return nil
	}

	r.cmd.Process.Signal(os.Interrupt)

	done := make(chan error, 1)

	go func() {
		done <- r.cmd.Wait()
	}()

	select {
	case err := <-done:
		if err != nil {
			fmt.Printf("GStreamer exited with error: %v\n", err)
		} else {
			fmt.Println("✅ GStreamer exited cleanly")
		}
		return err

	case <-time.After(5 * time.Second):
		r.cmd.Process.Kill()
		return r.cmd.Wait()
	}
}

// ─── Recording ────────────────────────────────────────────────────────────
func StartRecording__(session *portal.CaptureSession, output string) (*Recorder, error) {
	file := os.NewFile(uintptr(session.PipeWireFD), "pipewire")

	if !strings.HasSuffix(output, ".mkv") {
		output = output + ".mkv"
	}

	cmd := exec.Command(
		"gst-launch-1.0",
		"pipewiresrc", "fd=3", fmt.Sprintf("path=%d", session.NodeID),
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

	return &Recorder{cmd: cmd}, nil
}

func StartRecording(session *portal.CaptureSession, output string) (*Recorder, error) {
	file := os.NewFile(uintptr(session.PipeWireFD), "pipewire")
	if !strings.HasSuffix(output, ".mkv") {
		output = output + ".mkv"
	}

	// cpu heavy
	// cmd := exec.Command(
	// 	"gst-launch-1.0",

	// 	// ── Source ──────────────────────────────────────────────────────────
	// 	"pipewiresrc",
	// 	"fd=3",
	// 	fmt.Sprintf("path=%d", session.NodeID),
	// 	"do-timestamp=true",
	// 	"resend-last=true",

	// 	// Let PipeWire negotiate whatever format it wants
	// 	"!", "queue", "max-size-buffers=8", "leaky=downstream",

	// 	// ── Stabilize framerate AFTER negotiation ────────────────────────────
	// 	// videorate converts variable/unknown fps to a stable 30fps
	// 	"!", "videorate",
	// 	"!", "video/x-raw,framerate=30/1",

	// 	// ── Color conversion ─────────────────────────────────────────────────
	// 	"!", "videoconvert",
	// 	"!", "video/x-raw,format=I420",

	// 	// ── Encoder ──────────────────────────────────────────────────────────
	// 	"!", "x264enc",
	// 	"bitrate=8000",
	// 	"speed-preset=faster",
	// 	"tune=stillimage",
	// 	"key-int-max=250",
	// 	"vbv-buf-capacity=600",
	// 	"b-adapt=1",
	// 	"bframes=2",
	// 	"ref=3",
	// 	"rc-lookahead=20",

	// 	"!", "queue", "max-size-buffers=16",

	// 	// ── Mux + Sink ────────────────────────────────────────────────────────
	// 	"!", "matroskamux",
	// 	"!", "filesink",
	// 	fmt.Sprintf("location=%s", output),
	// )

	cmd := exec.Command(
		"gst-launch-1.0",

		"pipewiresrc",
		"fd=3",
		fmt.Sprintf("path=%d", session.NodeID),
		"do-timestamp=true",
		"resend-last=true",

		"!", "queue", "max-size-buffers=8", "leaky=downstream",
		"!", "videorate",
		"!", "videoscale",
		"!", "video/x-raw,framerate=24/1,width=1920,height=1080",

		"!", "videoconvert",
		"!", "video/x-raw,format=NV12",

		"!", "nvh264enc",
		"bitrate=8000",
		"rc-mode=cbr",
		"gop-size=250",
		"bframes=2",
		"b-adapt=true",
		"preset=5",
		"vbv-buffer-size=6000",
		"temporal-aq=true",

		"!", "queue", "max-size-buffers=16",
		"!", "h264parse",
		"!", "matroskamux",
		"!", "filesink",
		fmt.Sprintf("location=%s", output),
	)

	cmd.ExtraFiles = []*os.File{file}
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Start(); err != nil {
		return nil, err
	}
	return &Recorder{cmd: cmd}, nil
}
