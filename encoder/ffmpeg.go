package encoder

import (
	"fmt"
	"io"
	"os"
	"os/exec"
)

// FFmpeg wraps an ffmpeg process that reads raw I420 video from stdin
// and muxes it into an output file (mp4, mkv, etc.).
type FFmpeg struct {
	OutputPath string
	Framerate  int
	Width      int
	Height     int

	cmd *exec.Cmd
}

// NewFFmpeg creates an FFmpeg encoder writing to outputPath.
// Width/Height/Framerate must match what GStreamer negotiates.
// For the PoC we use 0x0 and let ffmpeg probe from the stream.
func NewFFmpeg(outputPath string, framerate int) *FFmpeg {
	return &FFmpeg{
		OutputPath: outputPath,
		Framerate:  framerate,
	}
}

// Start launches ffmpeg reading from r (GStreamer's stdout pipe).
func (f *FFmpeg) Start(r io.Reader) error {
	fps := fmt.Sprintf("%d", f.Framerate)
	if f.Framerate == 0 {
		fps = "30"
	}

	f.cmd = exec.Command(
		"ffmpeg",
		"-y",                  // overwrite output
		"-f", "rawvideo",
		"-pix_fmt", "yuv420p",
		"-framerate", fps,
		// width/height: if 0 we rely on gst to negotiate — ffmpeg needs them
		// for rawvideo; in a real impl you'd read them from the SPA format event.
		// For PoC: pass via env or hardcode your monitor res.
		"-video_size", fmt.Sprintf("%dx%d", f.Width, f.Height),
		"-i", "pipe:0",        // read from stdin
		"-c:v", "libx264",
		"-preset", "ultrafast",
		"-crf", "23",
		f.OutputPath,
	)

	f.cmd.Stdin = r
	f.cmd.Stdout = os.Stdout
	f.cmd.Stderr = os.Stderr

	return f.cmd.Start()
}

// Wait blocks until ffmpeg exits.
func (f *FFmpeg) Wait() error {
	if f.cmd == nil {
		return fmt.Errorf("ffmpeg not started")
	}
	return f.cmd.Wait()
}

// Kill forcibly terminates ffmpeg.
func (f *FFmpeg) Kill() {
	if f.cmd != nil && f.cmd.Process != nil {
		f.cmd.Process.Kill()
	}
}