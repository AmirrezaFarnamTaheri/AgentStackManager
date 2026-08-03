package redact

import (
	"encoding/json"
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

func TestValueRedactsTypedMapsAndStructs(t *testing.T) {
	type payload struct {
		Name     string            `json:"name"`
		Token    string            `json:"token"`
		Metadata map[string]string `json:"metadata"`
	}

	value := Value(payload{
		Name:  "safe",
		Token: "top-secret",
		Metadata: map[string]string{
			"authorization": "Bearer nested-secret",
			"note":          "api_key=inline-secret",
		},
	})
	data, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	text := string(data)
	for _, secret := range []string{"top-secret", "nested-secret", "inline-secret"} {
		if strings.Contains(text, secret) {
			t.Fatalf("structured redaction leaked %q: %s", secret, text)
		}
	}
	if !strings.Contains(text, `"name":"safe"`) {
		t.Fatalf("non-sensitive value was not preserved: %s", text)
	}
}

func TestValueFailsClosedForUnserializableStructuredValue(t *testing.T) {
	value := Value(map[string]any{"callback": func() {}})
	fields, ok := value.(map[string]any)
	if !ok {
		t.Fatalf("unexpected value type %T", value)
	}
	if fields["callback"] != Replacement {
		t.Fatalf("unserializable value did not fail closed: %#v", fields)
	}
}
