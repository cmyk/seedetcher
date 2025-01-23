// command controller is the user interface for engraving SeedHammer plates.
// It runs on a Raspberry Pi Zero, in the same configuration as SeedSigner.
package main



import (
	"fmt"
	"log"
	"os"
	"time"

	"seedhammer.com/gui"
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
	fmt.Println("Inside run() function...")  // New debug point
	version := os.Getenv("sh_version")
	fmt.Println("Version detected:", version)

	fmt.Println("Before GUI initialization...")
	p, err := Init()
	if err != nil {
		log.Fatalf("Initialization failed: %v", err)
		return err
	}

	fmt.Println("Initialization successful, running GUI...")

	for range gui.Run(p, version) {
		fmt.Println("GUI running...")
	}
	fmt.Println("Exiting run()")
	return nil
}

var debug = false

func (p *Platform) Debug() bool {
	return debug
}

func (p *Platform) Now() time.Time {
	return time.Now()
}
