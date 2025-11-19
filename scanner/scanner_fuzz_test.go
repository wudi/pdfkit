package scanner

import (
	"bytes"
	"testing"
)

func FuzzScanner(f *testing.F) {
	f.Add([]byte("<< /Type /Page >>"))
	f.Add([]byte("[ 1 2 3 ]"))
	f.Add([]byte("stream\n...data...\nendstream"))
	f.Add([]byte("(Hello World)"))
	f.Add([]byte("<AABBCC>"))

	f.Fuzz(func(t *testing.T, data []byte) {
		r := bytes.NewReader(data)
		s := New(r, Config{
			MaxStringLength: 1024,
			MaxArrayDepth:   10,
			MaxDictDepth:    10,
			MaxStreamLength: 1024,
			WindowSize:      1024,
		})

		for {
			_, err := s.Next()
			if err != nil {
				break
			}
		}
	})
}
