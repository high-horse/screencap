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

	sess, err := portal.Capture()
	if err != nil {
		log.Fatal(err)
	}

	recCmd, err := pipeline.StartRecording(sess, "recording")
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
