package main

import (
	"log"
	"os"

	"seedetcher.com/bip39"
	"seedetcher.com/print"
)

func main() {
	TestPCL()
}

func TestPCL() {
	mnemonic := bip39.Mnemonic([]bip39.Word{0, 1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11})
	qrData := []byte("test QR data")
	f, err := os.Create("test.pcl")
	if err != nil {
		log.Printf("Failed to create test.pcl: %v", err)
		return
	}
	print.PrintPCL(f, mnemonic, qrData)
	f.Close()
	log.Printf("Generated test.pcl")
}
