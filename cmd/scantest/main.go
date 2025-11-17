package main

import (
	"fmt"
	"io"
	"os"
	"pdflib/scanner"
	p "pdflib/parser"
)

func main() {
	if len(os.Args) < 2 {
		fmt.Println("usage: scantest <pdf>")
		os.Exit(1)
	}
	f, err := os.Open(os.Args[1])
	if err != nil { panic(err) }
	defer f.Close()

	s := scanner.New(f, scanner.Config{})
	ps := p.NewStreamAware(s)
	for i := 0; i < 200000; i++ { // limit to avoid flooding
		tok, err := ps.Next()
		if err == io.EOF { break }
		if err != nil { fmt.Printf("ERR: %v\n", err); break }
		fmt.Printf("%d@%d %T %v\n", tok.Type, tok.Pos, tok.Value, tok.Value)
	}
}
