package xfa

import (
	"encoding/xml"
	"testing"
)

func TestBinder_Bind(t *testing.T) {
	xmlData := `
	<xdp:xdp xmlns:xdp="http://ns.adobe.com/xdp/">
		<template>
			<subform name="form1">
				<field name="firstName">
					<value><text>Default</text></value>
				</field>
				<field name="lastName" />
				<subform name="address">
					<field name="street" />
					<field name="city" />
				</subform>
			</subform>
		</template>
		<datasets>
			<data>
				<form1>
					<firstName>John</firstName>
					<lastName>Doe</lastName>
					<address>
						<street>123 Main St</street>
						<city>Anytown</city>
					</address>
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

	binder := NewBinder(&form)
	binder.Bind()

	if form.Template == nil || form.Template.Subform == nil {
		t.Fatal("Template or Subform is nil")
	}

	// Verify firstName
	if len(form.Template.Subform.Fields) < 2 {
		t.Fatal("Expected at least 2 fields in root subform")
	}
	f1 := form.Template.Subform.Fields[0]
	if f1.Name != "firstName" {
		t.Fatalf("Expected first field to be firstName, got %s", f1.Name)
	}
	if f1.Value == nil {
		t.Fatal("firstName Value is nil")
	}
	if f1.Value.Text != "John" {
		t.Errorf("Expected firstName to be 'John', got '%s'", f1.Value.Text)
	}

	// Verify lastName
	f2 := form.Template.Subform.Fields[1]
	if f2.Name != "lastName" {
		t.Fatalf("Expected second field to be lastName, got %s", f2.Name)
	}
	if f2.Value == nil || f2.Value.Text != "Doe" {
		t.Errorf("Expected lastName to be 'Doe', got '%v'", f2.Value)
	}

	// Verify address subform
	if len(form.Template.Subform.Subforms) < 1 {
		t.Fatal("Expected at least 1 subform in root subform")
	}
	sub := form.Template.Subform.Subforms[0]
	if sub.Name != "address" {
		t.Fatalf("Expected subform to be address, got %s", sub.Name)
	}
	
	if len(sub.Fields) < 2 {
		t.Fatal("Expected at least 2 fields in address subform")
	}
	s1 := sub.Fields[0]
	if s1.Name != "street" {
		t.Fatalf("Expected first field of address to be street, got %s", s1.Name)
	}
	if s1.Value == nil || s1.Value.Text != "123 Main St" {
		t.Errorf("Expected street to be '123 Main St', got '%v'", s1.Value)
	}
}

func TestBinder_ExplicitBind(t *testing.T) {
	xmlData := `
	<xdp:xdp xmlns:xdp="http://ns.adobe.com/xdp/">
		<template>
			<subform name="form1">
				<field name="fullName">
					<bind match="dataRef" ref="name"/>
				</field>
			</subform>
		</template>
		<datasets>
			<data>
				<form1>
					<name>Jane Doe</name>
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

	binder := NewBinder(&form)
	binder.Bind()

	f1 := form.Template.Subform.Fields[0]
	if f1.Name != "fullName" {
		t.Fatalf("Expected field to be fullName, got %s", f1.Name)
	}
	if f1.Value == nil || f1.Value.Text != "Jane Doe" {
		t.Errorf("Expected fullName to be 'Jane Doe', got '%v'", f1.Value)
	}
}
