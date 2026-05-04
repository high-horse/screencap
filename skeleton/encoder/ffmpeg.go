package encoder

import (
	"fmt"
	"io"
	"os"
	"os/exec"
)

type FFmpeg struct {
	OutputPath string
	cmd        *exec.Cmd
}

func NewFFmpeg(outputPath string) *FFmpeg {
	return &FFmpeg{
		OutputPath: outputPath,
	}
}

func (f *FFmpeg) Start(r io.Reader) error {
	f.cmd = exec.Command(
		"ffmpeg",
		"-y",

		// input is encoded H264 stream
		"-f", "h264",
		"-probesize", "32",
		"-analyzeduration", "0",
		"-i", "pipe:0",

		// just copy (no re-encode)
		"-c:v", "copy",

		"-movflags", "+faststart",

		f.OutputPath,
	)

	f.cmd.Stdin = r
	f.cmd.Stdout = os.Stdout
	f.cmd.Stderr = os.Stderr

	return f.cmd.Start()
}

func (f *FFmpeg) Wait() error {
	if f.cmd == nil {
		return fmt.Errorf("ffmpeg not started")
	}
	return f.cmd.Wait()
}