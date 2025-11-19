package fonts

import (
	"os"
	"path/filepath"
	"testing"
)

func TestComputeClosureGSUB(t *testing.T) {
	// Load a font that likely has GSUB (e.g. Rubik or NotoSansJP)
	// We'll use Rubik-Regular.ttf if available
	fontPath := filepath.Join("..", "testdata", "Rubik-Regular.ttf")
	fontData, err := os.ReadFile(fontPath)
	if err != nil {
		t.Skipf("skipping test, font not found: %v", err)
	}

	// We don't know exact GIDs without parsing cmap, but we can try some low GIDs.
	// Usually GID 0 is .notdef, GID 1, 2, 3... are some glyphs.
	// If we pick a GID that is part of a ligature, we might see closure expansion.
	// But without knowing the font structure, it's hard to assert exact results.
	// However, we can assert that it runs without error and returns a superset.

	initial := map[int]bool{
		10: true,
		20: true,
	}

	closure, err := ComputeClosureGSUB(fontData, initial)
	if err != nil {
		t.Fatalf("ComputeClosureGSUB failed: %v", err)
	}

	if len(closure) < len(initial) {
		t.Errorf("closure size %d < initial size %d", len(closure), len(initial))
	}

	for gid := range initial {
		if !closure[gid] {
			t.Errorf("initial GID %d not in closure", gid)
		}
	}
	
	t.Logf("Initial: %d, Closure: %d", len(initial), len(closure))
}
