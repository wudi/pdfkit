package main

import (
	"context"
	"fmt"
	"os"

	"github.com/wudi/pdfkit/builder"
	"github.com/wudi/pdfkit/ir/raw"
	"github.com/wudi/pdfkit/writer"
)

func main() {
	b := builder.NewBuilder()
	b.NewPage(595, 842).
		DrawText("This document is encrypted.", 100, 700, builder.TextOptions{FontSize: 24}).
		DrawText("Owner Password: owner", 100, 650, builder.TextOptions{FontSize: 14}).
		DrawText("User Password: user", 100, 630, builder.TextOptions{FontSize: 14}).
		Finish()

	// Set encryption
	// Owner password, User password, Permissions, Encrypt Metadata
	b.SetEncryption("owner", "user", raw.Permissions{
		Print:             true,
		Modify:            false,
		Copy:              false,
		ModifyAnnotations: false,
	}, true)

	doc, err := b.Build()
	if err != nil {
		panic(err)
	}

	f, err := os.Create("encrypted.pdf")
	if err != nil {
		panic(err)
	}
	defer f.Close()

	w := writer.NewWriter()
	if err := w.Write(context.Background(), doc, f, writer.Config{}); err != nil {
		panic(err)
	}

	fmt.Println("Successfully created encrypted.pdf")
}
