package main

import (
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"

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
		log.Fatal(err)
	}
	defer session.Close()

	log.Println("→ Selecting sources...")
	if err := session.SelectSources(portal.SourceMonitor, false); err != nil {
		log.Fatal(err)
	}

	log.Println("→ Starting portal (pick screen)...")
	nodeIDs, err := session.Start()
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println("✓ Nodes:", nodeIDs)

	log.Println("→ Opening PipeWire fd...")
	pwFile, err := session.OpenPipeWireRemote()
	if err != nil {
		log.Fatal(err)
	}
	defer pwFile.Close()

	remote := &pipewire.Remote{
		FD:      pwFile,
		NodeIDs: nodeIDs,
	}

	log.Println("→ Building GStreamer...")
	gstCmd, err := remote.NewGStreamerCmd()
	if err != nil {
		log.Fatal(err)
	}

	// 👇 pipe stdout BEFORE start
	gstOut, err := gstCmd.StdoutPipe()
	if err != nil {
		log.Fatal(err)
	}

	// 👇 START GStreamer IMMEDIATELY (critical)
	log.Println("→ Starting GStreamer...")
	if err := gstCmd.Start(); err != nil {
		log.Fatal(err)
	}

	// 👇 NOW start ffmpeg
	log.Println("→ Starting ffmpeg...")
	enc := encoder.NewFFmpeg(outputPath)
	if err := enc.Start(gstOut); err != nil {
		gstCmd.Process.Kill()
		log.Fatal(err)
	}

	fmt.Println("🎬 Recording... Ctrl+C to stop")

	// wait for ctrl+c
	sig := make(chan os.Signal, 1)
	signal.Notify(sig, syscall.SIGINT, syscall.SIGTERM)
	<-sig

	fmt.Println("\n→ Stopping...")

	gstCmd.Process.Signal(syscall.SIGINT)
	gstCmd.Wait()

	enc.Wait()

	fmt.Println("✓ Saved:", outputPath)
}