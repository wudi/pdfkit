package main

import (
	"context"
	"fmt"
	"os"
	"pdflib/ir/raw"
	"pdflib/parser"
)

func main() {
	f, err := os.Open("testdata/pdfa-3b.pdf")
	if err != nil {
		panic(err)
	}
	defer f.Close()

	p := parser.NewDocumentParser(parser.Config{})
	doc, err := p.Parse(context.Background(), f)
	if err != nil {
		panic(err)
	}

	// Get Catalog
	rootRefObj, ok := doc.Trailer.Get(raw.NameObj{Val: "Root"})
	if !ok {
		panic("No Root in trailer")
	}
	rootRef, ok := rootRefObj.(raw.RefObj)
	if !ok {
		panic("Root is not a reference")
	}
	catalogObj, ok := doc.Objects[rootRef.R]
	if !ok {
		panic("Catalog object not found")
	}
	catalog, ok := catalogObj.(*raw.DictObj)
	if !ok {
		panic("Catalog is not a dictionary")
	}

	// Get OutputIntents
	oiObj, ok := catalog.Get(raw.NameObj{Val: "OutputIntents"})
	if !ok {
		fmt.Println("No OutputIntents found")
		return
	}
	
	var intents []raw.Object
	if arr, ok := oiObj.(*raw.ArrayObj); ok {
		intents = arr.Items
	} else if ref, ok := oiObj.(raw.RefObj); ok {
		// Indirect array?
		if obj, ok := doc.Objects[ref.R]; ok {
			if arr, ok := obj.(*raw.ArrayObj); ok {
				intents = arr.Items
			}
		}
	}

	for _, item := range intents {
		var dict *raw.DictObj
		if d, ok := item.(*raw.DictObj); ok {
			dict = d
		} else if ref, ok := item.(raw.RefObj); ok {
			if obj, ok := doc.Objects[ref.R]; ok {
				if d, ok := obj.(*raw.DictObj); ok {
					dict = d
				}
			}
		}

		if dict != nil {
			if destProfile, ok := dict.Get(raw.NameObj{Val: "DestOutputProfile"}); ok {
				if ref, ok := destProfile.(raw.RefObj); ok {
					if streamObj, ok := doc.Objects[ref.R]; ok {
						if stream, ok := streamObj.(*raw.StreamObj); ok {
							data := stream.Data
							err := os.WriteFile("testdata/sRGB.icc", data, 0644)
							if err != nil {
								panic(err)
							}
							fmt.Printf("Extracted ICC profile: %d bytes\n", len(data))
							return
						}
					}
				}
			}
		}
	}
	fmt.Println("No ICC profile found in OutputIntents")
}
