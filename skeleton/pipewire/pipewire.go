package pipewire

import (
	"fmt"
	"os"
	"os/exec"
	"strconv"
)

type Remote struct {
	FD      *os.File
	NodeIDs []uint32
}

func (r *Remote) NewGStreamerCmd() (*exec.Cmd, error) {
	if len(r.NodeIDs) == 0 {
		return nil, fmt.Errorf("no PipeWire node IDs")
	}

	nodeID := strconv.Itoa(int(r.NodeIDs[0]))

	args := []string{
		"pipewiresrc",
		"fd=3",
		// "target-object=" + nodeID,
		"path=" + nodeID,
		"do-timestamp=true",

		"!",
		"videoconvert",

		"!",
		"video/x-raw,format=I420",

		"!",
		"queue",

		"!",
		"x264enc",
		"tune=zerolatency",
		"bitrate=2000",
		"speed-preset=ultrafast",

		"!",
		"h264parse",

		"!",
		"fdsink", "fd=1",
	}

	cmd := exec.Command("/usr/bin/gst-launch-1.0", args...)

	cmd.Stderr = os.Stderr
	cmd.Env = append(os.Environ(), "GST_DEBUG=2")

	// 👇 CRITICAL: pass PipeWire fd as fd=3
	cmd.ExtraFiles = []*os.File{r.FD}

	return cmd, nil
}