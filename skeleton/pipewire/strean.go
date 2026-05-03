package pipewire

import (
	"fmt"
	"os/exec"
	"syscall"
)

// InheritFD configures cmd so that the PipeWire socket fd is kept open
// across the fork/exec into gst-launch-1.0.
// Without this the fd is closed-on-exec and GStreamer can't connect.
func (r *Remote) InheritFD(cmd *exec.Cmd) error {
	if r.FD == nil {
		return fmt.Errorf("no PipeWire fd to inherit")
	}

	if cmd.SysProcAttr == nil {
		cmd.SysProcAttr = &syscall.SysProcAttr{}
	}

	// Pass the fd as an extra file so it survives exec.
	// It appears in the child as fd 3, 4, … (after stdin/stdout/stderr).
	// pipewiresrc uses the fd= property which refers to the *original* fd
	// number — we must tell the OS not to close it.
	cmd.ExtraFiles = append(cmd.ExtraFiles, r.FD)
	return nil
}