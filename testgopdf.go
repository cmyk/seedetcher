package main

import (
	"github.com/jung-kurt/gofpdf/v2"
)

func main() {
	pdf := gofpdf.New("P", "mm", "A4", "")
	pdf.AddPage()
	pdf.SetFont("Arial", "B", 16)
	pdf.Cell(40, 10, "Hello, SeedPrinter!")
	pdf.OutputFileAndClose("test.pdf")
}
