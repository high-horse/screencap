package main

import (
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"screencap/encoder"
	"screencap/pipewire"
	"screencap/portal"
)

func main() {
	outputPath := "output.mp4"
	if len(os.Args) > 1 {
		outputPath = os.Args[1]
	}

	log.Println("→ Creating portal session...")
	session, err := portal.NewSession()
	if err != nil {
		log.Fatalf("session: %v", err)
	}
	defer session.Close()

	log.Println("→ Selecting sources (monitor)...")
	if err := session.SelectSources(portal.SourceMonitor, false); err != nil {
		log.Fatalf("SelectSources: %v", err)
	}

	log.Println("→ Starting — pick your screen in the dialog...")
	nodeIDs, err := session.Start()
	if err != nil {
		log.Fatalf("Start: %v", err)
	}
	fmt.Printf("✓ Got %d PipeWire node(s): %v\n", len(nodeIDs), nodeIDs)

	log.Println("→ Opening PipeWire remote fd...")
	pwFile, err := session.OpenPipeWireRemote()
	if err != nil {
		log.Fatalf("OpenPipeWireRemote: %v", err)
	}
	defer pwFile.Close()

	remote := &pipewire.Remote{
		FD:      pwFile,
		NodeIDs: nodeIDs,
	}

	// Build GStreamer command
	log.Println("→ Building GStreamer pipeline...")
	gstCmd, err := remote.NewGStreamerCmd()
	if err != nil {
		log.Fatalf("gst cmd: %v", err)
	}

	// InheritFD must happen before StdoutPipe
	// if err := remote.InheritFD(gstCmd); err != nil {
	// 	log.Fatalf("inherit fd: %v", err)
	// }
	// DEBUG: print what fd number the child will see
	log.Printf("DEBUG: parent fd=%d, child will see fd=%d",
		int(remote.FD.Fd()),
		3+len(gstCmd.ExtraFiles)-1, // 3=first extra, index from end
	)
	log.Printf("DEBUG: gst args: %v", gstCmd.Args)

	gstOut, err := gstCmd.StdoutPipe()
	if err != nil {
		log.Fatalf("gst stdout pipe: %v", err)
	}

	// ⚠️  Set to your monitor resolution.
	// Next step after PoC: read this from PipeWire SPA format negotiation.
	enc := encoder.NewFFmpeg(outputPath, 30)
	enc.Width = 1920
	enc.Height = 1080

	// Start GStreamer first — must negotiate PipeWire stream before ffmpeg reads
	log.Println("→ Starting GStreamer...")
	if err := gstCmd.Start(); err != nil {
		log.Fatalf("gst start: %v", err)
	}

	// Give GStreamer time to negotiate format with PipeWire
	log.Println("→ Waiting for PipeWire negotiation...")
	time.Sleep(2 * time.Second)

	// Now start ffmpeg — GStreamer is already producing frames
	log.Println("→ Starting ffmpeg...")
	if err := enc.Start(gstOut); err != nil {
		gstCmd.Process.Kill()
		log.Fatalf("ffmpeg start: %v", err)
	}

	fmt.Printf("🎬 Recording → %s   (Ctrl+C to stop)\n", outputPath)

	// Block until SIGINT/SIGTERM
	sig := make(chan os.Signal, 1)
	signal.Notify(sig, syscall.SIGINT, syscall.SIGTERM)
	<-sig

	fmt.Println("\n→ Stopping...")

	// SIGINT lets GStreamer flush its buffers cleanly before EOF
	gstCmd.Process.Signal(syscall.SIGINT)
	gstCmd.Wait()

	// ffmpeg sees EOF on stdin and finalizes the mp4 container
	enc.Wait()

	fmt.Printf("✓ Saved to %s\n", outputPath)
}
