package catalog

import "testing"

func FuzzCatalogDecode(f *testing.F) {
	f.Add([]byte(`{"version":1,"components":[],"profiles":[]}`))
	f.Add([]byte(`{}`))
	f.Fuzz(func(t *testing.T, data []byte) {
		_, _ = decode(data)
	})
}
