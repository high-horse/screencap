// Package encoder provides output sinks for captured video data.
package encoder

import "io"

// RawWriter copies raw frames from r directly to w with no encoding.
// Useful for debugging: pipe w to `ffplay -f rawvideo -pix_fmt yuv420p ...`
type RawWriter struct {
	W io.Writer
}

func (rw *RawWriter) Write(frame []byte) (int, error) {
	return rw.W.Write(frame)
}