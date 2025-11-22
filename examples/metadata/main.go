package main

import (
	"context"
	"fmt"
	"os"

	"github.com/wudi/pdfkit/builder"
	"github.com/wudi/pdfkit/ir/semantic"
	"github.com/wudi/pdfkit/writer"
)

func main() {
	b := builder.NewBuilder()
	b.NewPage(595, 842).
		DrawText("This PDF has custom metadata.", 100, 700, builder.TextOptions{FontSize: 24}).
		Finish()

	// Set Document Info
	b.SetInfo(&semantic.DocumentInfo{
		Title:    "Metadata Example",
		Author:   "PDFKit User",
		Subject:  "Demonstrating metadata support",
		Keywords: []string{"pdf", "metadata", "example"},
		Creator:  "PDFKit Example Generator",
		Producer: "PDFKit",
	})

	// Set XMP Metadata (optional)
	xmp := []byte(`<?xpacket begin="" id="W5M0MpCehiHzreSzNTczkc9d"?>
<x:xmpmeta xmlns:x="adobe:ns:meta/">
  <rdf:RDF xmlns:rdf="http://www.w3.org/1999/02/22-rdf-syntax-ns#">
    <rdf:Description rdf:about="" xmlns:dc="http://purl.org/dc/elements/1.1/">
      <dc:format>application/pdf</dc:format>
      <dc:title>
        <rdf:Alt>
          <rdf:li xml:lang="x-default">Metadata Example</rdf:li>
        </rdf:Alt>
      </dc:title>
    </rdf:Description>
  </rdf:RDF>
</x:xmpmeta>
<?xpacket end="w"?>`)
	b.SetMetadata(xmp)

	doc, err := b.Build()
	if err != nil {
		panic(err)
	}

	f, err := os.Create("metadata.pdf")
	if err != nil {
		panic(err)
	}
	defer f.Close()

	w := writer.NewWriter()
	if err := w.Write(context.Background(), doc, f, writer.Config{}); err != nil {
		panic(err)
	}

	fmt.Println("Successfully created metadata.pdf")
}
