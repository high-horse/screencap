package pipeline

import "os/exec"

type Recorder struct {
	cmd *exec.Cmd
}
