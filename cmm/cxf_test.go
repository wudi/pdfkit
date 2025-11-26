package cmm

import (
	"testing"
)

func TestParseCxF(t *testing.T) {
	xmlData := `<?xml version="1.0" encoding="UTF-8"?>
<CxF xmlns="http://colorexchangeformat.com/CxF3-core">
  <Resources>
    <ObjectCollection>
      <Object Name="Red">
        <ColorValues>
          <ColorCIELab>
            <L>50.0</L>
            <A>70.0</A>
            <B>60.0</B>
          </ColorCIELab>
        </ColorValues>
      </Object>
    </ObjectCollection>
  </Resources>
</CxF>`

	cxf, err := ParseCxF([]byte(xmlData))
	if err != nil {
		t.Fatalf("ParseCxF failed: %v", err)
	}

	if len(cxf.Resources.ObjectCollection.Objects) != 1 {
		t.Fatalf("Expected 1 object, got %d", len(cxf.Resources.ObjectCollection.Objects))
	}

	obj := cxf.Resources.ObjectCollection.Objects[0]
	if obj.Name != "Red" {
		t.Errorf("Expected object name 'Red', got '%s'", obj.Name)
	}

	if obj.ColorValues.ColorCIELab == nil {
		t.Fatal("Expected ColorCIELab values")
	}

	if obj.ColorValues.ColorCIELab.L != 50.0 {
		t.Errorf("Expected L=50.0, got %f", obj.ColorValues.ColorCIELab.L)
	}
}
