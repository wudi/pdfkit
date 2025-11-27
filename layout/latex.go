package layout

import (
	"bytes"

	treeblood "github.com/wyatt915/goldmark-treeblood"
	"github.com/yuin/goldmark"
)

// RenderLaTeX renders a LaTeX string to the PDF by converting it to MathML.
func (e *Engine) RenderLaTeX(latex string) error {
	// Wrap LaTeX in display math delimiters for goldmark processing
	source := "$$" + latex + "$$"

	md := goldmark.New(
		goldmark.WithExtensions(
			treeblood.MathML(),
		),
	)

	var buf bytes.Buffer
	if err := md.Convert([]byte(source), &buf); err != nil {
		return err
	}

	return e.RenderHTML(buf.String())
}
