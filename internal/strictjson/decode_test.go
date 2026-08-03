package strictjson

import (
	"strings"
	"testing"
)

func TestDecodeAcceptsExactlyOneKnownObject(t *testing.T) {
	var value struct {
		Name string `json:"name"`
	}
	if err := Decode([]byte(`{"name":"agentstack"}`), &value); err != nil {
		t.Fatal(err)
	}
	if value.Name != "agentstack" {
		t.Fatalf("unexpected decoded value: %#v", value)
	}
}

func TestDecodeRejectsUnknownFieldsAndTrailingContent(t *testing.T) {
	for _, input := range []string{
		`{"name":"agentstack","unknown":true}`,
		`{"name":"agentstack"} {}`,
		`{"name":"agentstack"} trailing`,
	} {
		var value struct {
			Name string `json:"name"`
		}
		if err := Decode([]byte(input), &value); err == nil {
			t.Fatalf("invalid document was accepted: %s", input)
		}
	}
}

func TestDecodeLabelsTrailingSyntaxErrors(t *testing.T) {
	var value map[string]any
	err := Decode([]byte(`{} trailing`), &value)
	if err == nil || !strings.Contains(err.Error(), "trailing JSON content") {
		t.Fatalf("unexpected trailing error: %v", err)
	}
}

func TestDecodeRejectsDuplicateObjectKeysAtAnyDepth(t *testing.T) {
	for _, input := range []string{
		`{"name":"first","name":"second"}`,
		`{"outer":{"token":"first","token":"second"}}`,
		`[{"id":1,"id":2}]`,
	} {
		var value any
		if err := Decode([]byte(input), &value); err == nil || !strings.Contains(err.Error(), "duplicate JSON object key") {
			t.Fatalf("duplicate-key document was accepted: input=%s err=%v", input, err)
		}
	}
}
