package main

import (
	"context"
	"fmt"
	"os"

	"github.com/wudi/pdfkit/ir"
	"github.com/wudi/pdfkit/ir/semantic"
)

func main() {
	if len(os.Args) < 2 {
		fmt.Println("Usage: go run main.go <pdf_file>")
		return
	}

	filePath := os.Args[1]
	f, err := os.Open(filePath)
	if err != nil {
		panic(err)
	}
	defer f.Close()

	fmt.Printf("Parsing %s...\n", filePath)

	// 1. Parse the PDF
	pipeline := ir.NewDefault()
	doc, err := pipeline.Parse(context.Background(), f)
	if err != nil {
		panic(err)
	}

	// 2. Check for Structure Tree
	if doc.StructTree == nil {
		fmt.Println("No logical structure tree found in this PDF.")
		return
	}

	fmt.Println("Traversing structure tree...")
	count := 0

	// 3. Recursive function to walk the tree
	var walk func(elem *semantic.StructureElement)
	walk = func(elem *semantic.StructureElement) {
		// Check for Associated Files
		if len(elem.AssociatedFiles) > 0 {
			for _, af := range elem.AssociatedFiles {
				// Check for MathML mimetype or XML extension
				isMathML := af.Subtype == "application/mathml+xml" ||
					af.Subtype == "text/xml" || // Some generators use generic XML
					(len(af.Name) > 4 && af.Name[len(af.Name)-4:] == ".xml")

				if isMathML {
					count++
					fmt.Printf("\n[Found MathML #%d]\n", count)
					fmt.Printf("Parent Element: %s\n", elem.S) // e.g., "Formula"
					fmt.Printf("File Name:      %s\n", af.Name)
					fmt.Printf("Object Ref:     %s\n", af.OriginalRef)
					fmt.Printf("Subtype:        %s\n", af.Subtype)
					fmt.Printf("Description:    %s\n", af.Description)
					fmt.Printf("Content:\n%s\n", string(af.Data))
					fmt.Println("------------------------------------------------")
				}
			}
		}

		// Recurse into children
		for _, kid := range elem.K {
			if kid.Element != nil {
				walk(kid.Element)
			}
		}
	}

	// Start walking from the root children
	for _, k := range doc.StructTree.K {
		if k != nil {
			walk(k)
		}
	}

	if count == 0 {
		fmt.Println("No MathML associated files found.")
	} else {
		fmt.Printf("Found %d MathML fragments.\n", count)
	}
}
