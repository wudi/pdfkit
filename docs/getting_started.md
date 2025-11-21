# Getting Started with PDFKit

## Installation

To install PDFKit, run the following command:

```bash
go get github.com/wudi/pdfkit
```

## Basic Usage

### Importing the Library

```go
import (
    "github.com/wudi/pdfkit/builder"
    "github.com/wudi/pdfkit/parser"
    "github.com/wudi/pdfkit/writer"
)
```

### Creating a Simple PDF

```go
package main

import (
    "context"
    "os"
    "github.com/wudi/pdfkit/builder"
    "github.com/wudi/pdfkit/writer"
)

func main() {
    // Create a new builder
    b := builder.NewBuilder()
    
    // Add a page and draw some text
    b.NewPage(612, 792). // US Letter size
        DrawText("Hello, PDFKit!", 100, 700, builder.TextOptions{
            Font:     "Helvetica",
            FontSize: 24,
        }).
        Finish()
    
    // Build the document
    doc, err := b.Build()
    if err != nil {
        panic(err)
    }
    
    // Write to file
    f, err := os.Create("output.pdf")
    if err != nil {
        panic(err)
    }
    defer f.Close()
    
    w := writer.NewWriter()
    if err := w.Write(context.Background(), doc, f, writer.Config{}); err != nil {
        panic(err)
    }
}
```

### Parsing a PDF

```go
package main

import (
    "context"
    "fmt"
    "os"
    "github.com/wudi/pdfkit/parser"
)

func main() {
    f, err := os.Open("input.pdf")
    if err != nil {
        panic(err)
    }
    defer f.Close()
    
    p := parser.NewParser(parser.Config{})
    doc, err := p.Parse(context.Background(), f)
    if err != nil {
        panic(err)
    }
    
    fmt.Printf("PDF Version: %s\n", doc.Version)
    fmt.Printf("Number of Pages: %d\n", len(doc.Pages))
}
```

## Running Tests

To run the full test suite:

```bash
go test ./...
```

To run tests with native OpenJPEG support (requires `libopenjp2`):

```bash
go test ./... -tags openjpeg
```
