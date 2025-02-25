// command controller is the user interface for engraving SeedEtcher plates.
// It runs on a Raspberry Pi Zero, in the same configuration as SeedSigner.
package main

import (
	"fmt"
	"log"
	"os"

	"seedetcher.com/gui"
)

func main() {
	fmt.Println("Application started...")
	if err := run(); err != nil {
		fmt.Fprintf(os.Stderr, "controller: %v\n", err)
		os.Exit(2)
	}
}

func run() error {
	log.SetFlags(log.Flags() &^ (log.Ldate | log.Ltime))
	version := os.Getenv("sh_version")
	p, err := Init()
	if err != nil {
		log.Fatalf("Initialization failed: %v", err)
		return err
	}

	for range gui.Run(p, version) {
	}
	return nil
}

var debug = false

// Remove Platform struct and methods here; delegate to debug_rpi.go or platform_rpi.go
