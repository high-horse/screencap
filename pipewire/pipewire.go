// Package pipewire bridges the PipeWire fd and node ID from the XDG portal
// into a runnable GStreamer pipeline via gst-launch-1.0.
// For a pure-Go PipeWire binding you would use CGo against libpipewire-0.3;
// GStreamer is used here to keep the PoC dependency-free.
package pipewire

import (
	"fmt"
	"os"
	"os/exec"
	"strconv"
)

// Remote holds the PipeWire socket fd and the selected stream node IDs.
type Remote struct {
	FD      *os.File
	NodeIDs []uint32
}

// GStreamerArgs builds the gst-launch-1.0 argument list that reads from
// the PipeWire node and writes raw video to stdout.
// The caller (encoder/ffmpeg.go) connects stdout to ffmpeg's stdin.
//
//	pipewiresrc fd=<N> path=<nodeID> do-timestamp=true
//	  ! video/x-raw ! fdsink fd=1
func (r *Remote) GStreamerArgs() ([]string, error) {
	if len(r.NodeIDs) == 0 {
		return nil, fmt.Errorf("no PipeWire node IDs")
	}
	nodeID := strconv.Itoa(int(r.NodeIDs[0]))
	return []string{nodeID}, nil // just return nodeID, fd computed in NewGStreamerCmd
}

// NewGStreamerCmd returns a ready-but-not-started *exec.Cmd for gst-launch-1.0.
// stdout is left open so the caller can pipe it to ffmpeg.

func (r *Remote) NewGStreamerCmd() (*exec.Cmd, error) {
	if len(r.NodeIDs) == 0 {
		return nil, fmt.Errorf("no PipeWire node IDs")
	}

	nodeID := strconv.Itoa(int(r.NodeIDs[0]))

	// ExtraFiles[0] lands at fd=3 in the child (after stdin/stdout/stderr)
	// but only AFTER we append to ExtraFiles — do it here so we know the index
	childFD := 3 // will be ExtraFiles[0]

	args := []string{
		"pipewiresrc",
		fmt.Sprintf("fd=%d", childFD),
		"target-object=" + nodeID,
		"do-timestamp=true",
		"!",
		"videoconvert",
		"!",
		"video/x-raw,format=I420",
		"!",
		"fdsink", "fd=1",
	}

	cmd := exec.Command("gst-launch-1.0", args...)
	cmd.Stderr = os.Stderr
	cmd.Env = append(os.Environ(), "GST_DEBUG=pipewiresrc:5")

	// Append the PipeWire fd as ExtraFiles[0] → child fd 3
	cmd.ExtraFiles = []*os.File{r.FD}

	return cmd, nil
}