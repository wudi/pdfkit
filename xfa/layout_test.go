package xfa

import (
	"context"
	"encoding/xml"
	"testing"
)

func TestLayoutEngine_Render(t *testing.T) {
	xmlData := `
	<xdp:xdp xmlns:xdp="http://ns.adobe.com/xdp/">
		<template>
			<subform name="form1" layout="tb">
				<draw name="title" x="1in" y="1in">
					<value><text>Hello XFA</text></value>
				</draw>
				<field name="name" x="1in" y="2in">
					<caption><value><text>Name:</text></value></caption>
					<value><text>John Doe</text></value>
				</field>
			</subform>
		</template>
		<datasets>
			<data>
				<form1>
					<name>John Doe</name>
				</form1>
			</data>
		</datasets>
	</xdp:xdp>
	`

	var form Form
	err := xml.Unmarshal([]byte(xmlData), &form)
	if err != nil {
		t.Fatalf("Failed to unmarshal XML: %v", err)
	}

	engine := NewLayoutEngine()
	pages, err := engine.Render(context.Background(), &form)
	if err != nil {
		t.Fatalf("Render failed: %v", err)
	}

	if len(pages) != 1 {
		t.Fatalf("Expected 1 page, got %d", len(pages))
	}

	page := pages[0]
	if len(page.Contents) == 0 {
		t.Fatal("Page has no content")
	}

	// Check for operations
	ops := page.Contents[0].Operations
	if len(ops) == 0 {
		t.Fatal("No operations generated")
	}

	// Basic check for "Tj" operator
	foundText := false
	for _, op := range ops {
		if op.Operator == "Tj" {
			foundText = true
			break
		}
	}

	if !foundText {
		t.Error("Expected Tj operator (text rendering)")
	}
}
