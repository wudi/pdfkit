package optimize

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"sort"

	"github.com/wudi/pdfkit/ir/raw"
)

func hashObject(obj raw.Object) string {
	h := sha256.New()
	writeHash(h, obj)
	return hex.EncodeToString(h.Sum(nil))
}

func writeHash(h interface{ Write([]byte) (int, error) }, obj raw.Object) {
	if obj == nil {
		fmt.Fprint(h, "nil")
		return
	}
	fmt.Fprint(h, obj.Type(), ":")
	switch t := obj.(type) {
	case raw.Name:
		fmt.Fprint(h, t.Value())
	case raw.Number:
		if t.IsInteger() {
			fmt.Fprint(h, t.Int())
		} else {
			fmt.Fprint(h, t.Float())
		}
	case raw.Boolean:
		fmt.Fprint(h, t.Value())
	case raw.String:
		fmt.Fprint(h, string(t.Value()))
	case raw.Reference:
		fmt.Fprintf(h, "%d %d R", t.Ref().Num, t.Ref().Gen)
	case raw.Array:
		fmt.Fprint(h, "[")
		for i := 0; i < t.Len(); i++ {
			v, _ := t.Get(i)
			writeHash(h, v)
			fmt.Fprint(h, ",")
		}
		fmt.Fprint(h, "]")
	case raw.Dictionary:
		fmt.Fprint(h, "<<")
		keys := t.Keys()
		// Sort keys for consistent hashing
		sort.Slice(keys, func(i, j int) bool {
			return keys[i].Value() < keys[j].Value()
		})
		for _, k := range keys {
			fmt.Fprint(h, k.Value())
			v, _ := t.Get(k)
			writeHash(h, v)
		}
		fmt.Fprint(h, ">>")
	case raw.Stream:
		// Hash dictionary and data
		writeHash(h, t.Dictionary())
		h.Write(t.RawData())
	case raw.Null:
		fmt.Fprint(h, "null")
	}
}
