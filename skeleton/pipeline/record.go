package pipeline

import (
	"fmt"
	"os"
	"os/exec"
	"screencap/portal"
	"strings"
)

// ─── Recording ────────────────────────────────────────────────────────────
// // pwFd int, nodeID uint32
func StartRecording(session *portal.CaptureSession, output string) (*exec.Cmd, error) {
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

	return cmd, nil
}
