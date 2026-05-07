package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/signal"
	"screencap/pipeline"
	"screencap/portal"
	"time"
)

func SetLogFlags() {
	log.SetFlags(log.LstdFlags | log.Lshortfile)
}

func main() {
	SetLogFlags()
	if err := CheckDependencies(); err != nil {
		log.Fatal(err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, os.Interrupt)
	go func() {
		<-sigCh
		fmt.Println("\n shutting down")
		cancel()
	}()

	portal, err := portal.NewScreenCastPortal()
	if err != nil {
		log.Fatal(err)
	}
	defer portal.Close()

	if sources, err := portal.GetAvailableSourceTypes(); err != nil {
		fmt.Printf("Warning: Could not get available source types: %v\n", err)
	} else {
		fmt.Printf("Available source types: %d (1=Monitor, 2=Window, 4=Virtual)\n", sources)
	}

	sessionPath, err := portal.CreateSession()
	if err != nil {
		log.Fatal("Failed to create session:", err)
	}

	if err := portal.SelectSources(sessionPath); err != nil {
		log.Fatal("Failed to select sources:", err)
	}

	nodeId, streamProps, err := portal.Start(sessionPath)
	if err != nil {
		log.Fatal("Failed to start session:", err)
	}

	if size, ok := streamProps["size"]; ok {
		fmt.Printf("  Stream size: %+v\n", size.Value())
	}

	pwFd, err := portal.OpenPipeWireRemote(sessionPath)
	if err != nil {
		fmt.Println("COuld not open pipewire remote ", err)
	}

	recCmd, err := pipeline.StartRecording(pwFd, nodeId, "recording")
	if err != nil {
		log.Fatal(err)
	}

	<-ctx.Done()

	if recCmd != nil && recCmd.Process != nil {
		fmt.Println("Sending SIGTERM to GStreamer...")
		recCmd.Process.Signal(os.Interrupt)

		done := make(chan error, 1)
		go func() { done <- recCmd.Wait() }()

		select {
		case err := <-done:
			if err != nil {
				fmt.Printf("GStreamer exited with error: %v\n", err)
			} else {
				fmt.Println("✅ GStreamer exited cleanly")
			}
		case <-time.After(5 * time.Second):
			fmt.Println("⚠ Timeout, forcing kill...")
			recCmd.Process.Kill()
			recCmd.Wait()
		}
	}
}
