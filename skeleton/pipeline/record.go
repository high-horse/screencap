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


// working fine implementation
func StartStream(session *portal.CaptureSession) (*Recorder, error) {
	file := os.NewFile(uintptr(session.PipeWireFD), "pipewire")

	cmd := exec.Command(
		"gst-launch-1.0",

		// Source
		"pipewiresrc",
		"fd=3",
		fmt.Sprintf("path=%d", session.NodeID),

		"do-timestamp=true",

		// Prevent queue buildup
		"!",
		"queue",
		"leaky=downstream",
		"max-size-buffers=2",

		// Convert
		"!",
		"videoconvert",

		// NVENC expects NV12
		"!",
		"video/x-raw,format=NV12",

		// Encoder
		"!",
		"nvh264enc",

		"bitrate=6000",
		"preset=1",
		"bframes=0",
		"gop-size=30",

		// Parse
		"!",
		"h264parse",

		"config-interval=1",

		// MPEGTS mux
		"!",
		"mpegtsmux",

		"alignment=7",

		// UDP stream
		"!",
		"udpsink",

		"host=127.0.0.1",
		"port=5000",

		"sync=false",
		"async=false",
	)

	cmd.ExtraFiles = []*os.File{file}

	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Start(); err != nil {
		return nil, err
	}

	fmt.Println("UDP MPEG-TS stream ready")
	fmt.Println("")
	fmt.Println("Open with VLC:")
	fmt.Println("vlc udp://@:5000")
	fmt.Println("")
	fmt.Println("Open with ffplay:")
	fmt.Println("ffplay -fflags nobuffer -flags low_delay udp://127.0.0.1:5000")

	return &Recorder{
		cmd: cmd,
	}, nil
}


// does nnot works properly.
func StartStream_bkp(session *portal.CaptureSession) (*Recorder, error) {
	file := os.NewFile(uintptr(session.PipeWireFD), "pipewire")

	cmd := exec.Command(
		"gst-launch-1.0",

		// Source
		"pipewiresrc",
		"fd=3",
		fmt.Sprintf("path=%d", session.NodeID),
		"do-timestamp=true",

		// Prevent latency buildup
		"!",
		"queue",
		"leaky=downstream",
		"max-size-buffers=2",

		// Convert
		"!",
		"videoconvert",

		// x264 wants I420
		"!",
		"video/x-raw,format=I420",

		// Encoder
		"!",
		"x264enc",

		// ultra low latency
		"tune=zerolatency",

		// fastest encoding
		"speed-preset=ultrafast",

		// bitrate kbps
		"bitrate=4000",

		// keyframe every second
		"key-int-max=30",

		// no bframes
		"bframes=0",

		// H264 parser
		"!",
		"h264parse",

		// resend codec config
		"config-interval=1",

		// MPEGTS mux
		"!",
		"mpegtsmux",

		"alignment=7",

		// UDP output
		"!",
		"udpsink",

		"host=127.0.0.1",
		"port=5000",

		"sync=false",
		"async=false",
	)

	cmd.ExtraFiles = []*os.File{file}

	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Start(); err != nil {
		return nil, err
	}

	fmt.Println("Stream ready")
	fmt.Println("")
	fmt.Println("VLC:")
	fmt.Println("vlc udp://@:5000")
	fmt.Println("")
	fmt.Println("ffplay:")
	fmt.Println("ffplay -fflags nobuffer -flags low_delay udp://127.0.0.1:5000")

	return &Recorder{
		cmd: cmd,
	}, nil
}