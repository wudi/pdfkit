package layout

import (
	"testing"

	"github.com/wudi/pdfkit/builder"
)

func TestEngine_RenderMathML(t *testing.T) {
	b := builder.NewBuilder()
	engine := NewEngine(b)

	// Sample from user
	mathml := `<math xmlns="http://www.w3.org/1998/Math/MathML"> <msub> <mi>w</mi> <mi>i</mi> </msub> </math>`

	err := engine.RenderHTML(mathml)
	if err != nil {
		t.Fatalf("RenderHTML with MathML failed: %v", err)
	}

	doc, err := b.Build()
	if err != nil {
		t.Fatalf("Build failed: %v", err)
	}

	if len(doc.Pages) == 0 {
		t.Fatal("Expected at least one page")
	}
}

func TestEngine_RenderLaTeX(t *testing.T) {
	b := builder.NewBuilder()
	engine := NewEngine(b)

	latex := `E = mc^2`

	err := engine.RenderLaTeX(latex)
	if err != nil {
		t.Fatalf("RenderLaTeX failed: %v", err)
	}

	doc, err := b.Build()
	if err != nil {
		t.Fatalf("Build failed: %v", err)
	}

	if len(doc.Pages) == 0 {
		t.Fatal("Expected at least one page")
	}
}

func TestEngine_RenderComplexMath(t *testing.T) {
	b := builder.NewBuilder()
	engine := NewEngine(b)

	// Fraction, sqrt, superscript
	mathml := `
<math>
	<mfrac>
		<mi>x</mi>
		<mrow>
			<mi>y</mi>
			<mo>+</mo>
			<mn>1</mn>
		</mrow>
	</mfrac>
	<mo>=</mo>
	<msqrt>
		<msup>
			<mi>a</mi>
			<mn>2</mn>
		</msup>
		<mo>+</mo>
		<msup>
			<mi>b</mi>
			<mn>2</mn>
		</msup>
	</msqrt>
</math>
`
	err := engine.RenderHTML(mathml)
	if err != nil {
		t.Fatalf("RenderHTML with Complex MathML failed: %v", err)
	}

	doc, err := b.Build()
	if err != nil {
		t.Fatalf("Build failed: %v", err)
	}
	if len(doc.Pages) == 0 {
		t.Fatal("Expected at least one page")
	}
}
