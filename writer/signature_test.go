package writer

import (
	"bytes"
	"context"
	"testing"

	"pdflib/ir/raw"
	"pdflib/ir/semantic"
	"pdflib/parser"
)

func TestWriter_SignatureField(t *testing.T) {
	doc := &semantic.Document{
		Pages: []*semantic.Page{
			{
				MediaBox: semantic.Rectangle{URX: 100, URY: 100},
			},
		},
		AcroForm: &semantic.AcroForm{
			Fields: []semantic.FormField{
				&semantic.SignatureFormField{
					BaseFormField: semantic.BaseFormField{
						Name:      "SigField",
						Rect:      semantic.Rectangle{LLX: 10, LLY: 10, URX: 90, URY: 40},
						PageIndex: 0,
					},
					Signature: &semantic.Signature{
						Filter:      "Adobe.PPKLite",
						SubFilter:   "adbe.pkcs7.detached",
						Name:        "John Doe",
						Reason:      "I agree",
						Location:    "New York",
						ContactInfo: "john@example.com",
						M:           "D:20230101120000Z",
						Contents:    []byte("signature_data"),
						ByteRange:   []int{0, 100, 200, 300},
						Reference: []semantic.SigRef{
							{
								Type:            "SigRef",
								TransformMethod: "DocMDP",
								TransformParams: &semantic.SigTransformParams{
									Type: "TransformParams",
									P:    2,
									V:    "1.2",
								},
								DigestMethod: "SHA1",
							},
						},
					},
				},
			},
		},
	}

	var buf bytes.Buffer
	w := (&WriterBuilder{}).Build()
	if err := w.Write(staticCtx{}, doc, &buf, Config{Deterministic: true}); err != nil {
		t.Fatalf("write pdf: %v", err)
	}

	// Parse back and verify
	rawParser := parser.NewDocumentParser(parser.Config{})
	rawDoc, err := rawParser.Parse(context.Background(), bytes.NewReader(buf.Bytes()))
	if err != nil {
		t.Fatalf("parse raw: %v", err)
	}

	foundSig := false
	for _, obj := range rawDoc.Objects {
		d, ok := obj.(*raw.DictObj)
		if !ok {
			continue
		}
		if typ, ok := d.Get(raw.NameLiteral("Type")); ok {
			if n, ok := typ.(raw.NameObj); ok && n.Value() == "Annot" {
				if subtype, ok := d.Get(raw.NameLiteral("Subtype")); ok {
					if s, ok := subtype.(raw.NameObj); ok && s.Value() == "Widget" {
						if ft, ok := d.Get(raw.NameLiteral("FT")); ok {
							if f, ok := ft.(raw.NameObj); ok && f.Value() == "Sig" {
								// Found Signature Widget
								v, ok := d.Get(raw.NameLiteral("V"))
								if !ok {
									t.Errorf("Signature field missing V entry")
									continue
								}
								ref, ok := v.(raw.RefObj)
								if !ok {
									t.Errorf("V entry is not a reference")
									continue
								}
								sigObj := rawDoc.Objects[ref.Ref()]
								sigDict, ok := sigObj.(*raw.DictObj)
								if !ok {
									t.Errorf("Signature object is not a dict")
									continue
								}

								// Verify Signature Dict
								checkString(t, sigDict, "Filter", "Adobe.PPKLite")
								checkString(t, sigDict, "SubFilter", "adbe.pkcs7.detached")
								checkString(t, sigDict, "Name", "John Doe")
								checkString(t, sigDict, "Reason", "I agree")
								checkString(t, sigDict, "Location", "New York")
								checkString(t, sigDict, "ContactInfo", "john@example.com")
								checkString(t, sigDict, "M", "D:20230101120000Z")

								// Check ByteRange
								if br, ok := sigDict.Get(raw.NameLiteral("ByteRange")); ok {
									if arr, ok := br.(*raw.ArrayObj); ok && arr.Len() == 4 {
										// ok
									} else {
										t.Errorf("Invalid ByteRange")
									}
								} else {
									t.Errorf("Missing ByteRange")
								}

								// Check Reference
								if refs, ok := sigDict.Get(raw.NameLiteral("Reference")); ok {
									if arr, ok := refs.(*raw.ArrayObj); ok && arr.Len() == 1 {
										refDict, _ := arr.Items[0].(*raw.DictObj)
										checkString(t, refDict, "TransformMethod", "DocMDP")
										if tp, ok := refDict.Get(raw.NameLiteral("TransformParams")); ok {
											tpDict, _ := tp.(*raw.DictObj)
											if p, ok := tpDict.Get(raw.NameLiteral("P")); ok {
												if num, ok := p.(raw.NumberObj); ok && num.Int() == 2 {
													// ok
												} else {
													t.Errorf("Invalid TransformParams P")
												}
											}
										}
									} else {
										t.Errorf("Invalid Reference array")
									}
								}

								foundSig = true
							}
						}
					}
				}
			}
		}
	}

	if !foundSig {
		t.Fatalf("Signature field not found")
	}
}

func checkString(t *testing.T, d *raw.DictObj, key, expected string) {
	if val, ok := d.Get(raw.NameLiteral(key)); ok {
		if s, ok := val.(raw.StringObj); ok {
			if string(s.Value()) != expected {
				t.Errorf("Expected %s=%s, got %s", key, expected, s.Value())
			}
		} else if n, ok := val.(raw.NameObj); ok {
			if n.Value() != expected {
				t.Errorf("Expected %s=%s, got %s", key, expected, n.Value())
			}
		} else {
			t.Errorf("Expected %s=%s, got wrong type", key, expected)
		}
	} else {
		t.Errorf("Missing %s", key)
	}
}
