package main

import (
	"log"
	"screencap/internal/util"
	"screencap/pipeline"
	"screencap/portal"
)

func SetLogFlags() {
	log.SetFlags(log.LstdFlags | log.Lshortfile)
}

func main() {
	SetLogFlags()
	if err := CheckDependencies(); err != nil {
		log.Fatal(err)
	}

	ctx, cancel := util.SignalContext()
	defer cancel()

	portal, err := portal.NewScreenCastPortal()
	if err != nil {
		log.Fatal(err)
	}
	defer portal.Close()

	// if sources, err := portal.GetAvailableSourceTypes(); err != nil {
	// 	fmt.Printf("Warning: Could not get available source types: %v\n", err)
	// } else {
	// 	fmt.Printf("Available source types: %d (1=Monitor, 2=Window, 4=Virtual)\n", sources)
	// }

	sess, err := portal.Capture()
	if err != nil {
		log.Fatal(err)
	}

	rec, err := pipeline.StartRecording(sess, "recording")
	if err != nil {
		log.Fatal(err)
	}

	<-ctx.Done()

	rec.Stop()
}
