package main

import (
	"fmt"
	"os"
)

func main() {
	data, err := os.ReadFile("testdata/sRGB.icc")
	if err != nil {
		panic(err)
	}
	fmt.Printf("package pdfa\n\nvar DefaultICCProfile = []byte{")
	for i, b := range data {
		if i%12 == 0 {
			fmt.Printf("\n\t")
		}
		fmt.Printf("0x%02x, ", b)
	}
	fmt.Printf("\n}\n")
}
