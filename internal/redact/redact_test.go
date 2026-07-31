package redact

import (
	"strings"
	"testing"
)

func TestTextRedactsCommonCredentialForms(t *testing.T) {
	jwt := "eyJhbGciOiJIUzI1NiJ9.eyJzdWIiOiJ0ZXN0In0.signature123"
	input := `Authorization: Bearer alpha {"access_token":"beta"} api_key=gamma ` + jwt
	output := Text(input)
	for _, value := range []string{"alpha", "beta", "gamma", jwt} {
		if strings.Contains(output, value) {
			t.Fatalf("redaction missed %q: %s", value, output)
		}
	}
}

func BenchmarkRedactText(b *testing.B) {
	input := `child failed: Authorization: Bearer alpha {"access_token":"beta"} api_key=gamma eyJhbGciOiJIUzI1NiJ9.eyJzdWIiOiJ0ZXN0In0.signature123`
	b.ReportAllocs()
	for index := 0; index < b.N; index++ {
		_ = Text(input)
	}
}
