package main

import (
	"bytes"
	"fmt"
	"log"
	"os"

	"github.com/nijaru/brotli"
)

func main() {
	data, err := os.ReadFile("testdata/Isaac.Newton-Opticks.txt")
	if err != nil {
		log.Fatal(err)
	}

	fmt.Printf("Original size: %d\n", len(data))
	fmt.Printf("%-10s %-15s %-10s\n", "Level", "Compressed Size", "Ratio")

	for level := 0; level <= 11; level++ {
		var buf bytes.Buffer
		w := brotli.NewWriterLevel(&buf, level)
		w.Write(data)
		w.Close()

		compressed := buf.Len()
		ratio := float64(len(data)) / float64(compressed)
		fmt.Printf("%-10d %-15d %-10.3f\n", level, compressed, ratio)
	}
}
