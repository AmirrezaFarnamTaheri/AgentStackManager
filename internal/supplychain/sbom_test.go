package supplychain

import (
	"github.com/agentstack/agentstack/internal/catalog"
	"github.com/agentstack/agentstack/internal/model"
	"regexp"
	"testing"
)

func TestGenerateIncludesEveryAutomaticCatalogComponent(t *testing.T) {
	c, err := catalog.LoadDefault()
	if err != nil {
		t.Fatal(err)
	}
	licenses, err := LoadLicenses("../../supply-chain/component-licenses.json")
	if err != nil {
		t.Fatal(err)
	}
	bom, err := Generate(c, "test", licenses)
	if err != nil {
		t.Fatal(err)
	}
	want := 0
	for _, component := range c.Components {
		if component.Install.Kind != model.InstallManual && component.Install.Kind != model.InstallNone {
			want++
		}
	}
	if len(bom.Components) != want {
		t.Fatalf("components=%d want=%d", len(bom.Components), want)
	}
	for _, component := range bom.Components {
		if len(component.Licenses) == 0 || component.PURL == "" {
			t.Fatalf("incomplete component %#v", component)
		}
	}
}

func TestGenerateRejectsMissingLicense(t *testing.T) {
	c, _ := catalog.LoadDefault()
	if _, err := Generate(c, "test", LicenseInventory{}); err == nil {
		t.Fatal("expected missing license rejection")
	}
}

func TestGenerateUsesSchemaValidDeterministicIdentityAndScopedNpmPURL(t *testing.T) {
	c := model.Catalog{Version: 1, Components: []model.Component{{
		ID: "scoped", Name: "Scoped", Platforms: []string{"windows"},
		Install: model.InstallSpec{Kind: model.InstallNPMGlobal, Package: "@scope/tool@1.2.3", Publisher: "scope", Source: "npmjs"},
	}}}
	licenses := LicenseInventory{"scoped": "MIT"}
	first, err := Generate(c, "1.2.3", licenses)
	if err != nil {
		t.Fatal(err)
	}
	second, err := Generate(c, "1.2.3", licenses)
	if err != nil {
		t.Fatal(err)
	}
	if first.Serial != second.Serial {
		t.Fatalf("serial is not deterministic: %q != %q", first.Serial, second.Serial)
	}
	if matched, _ := regexp.MatchString(`^urn:uuid:[0-9a-f]{8}-[0-9a-f]{4}-5[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$`, first.Serial); !matched {
		t.Fatalf("invalid RFC 4122 UUID serial: %q", first.Serial)
	}
	if first.Schema != "https://cyclonedx.org/schema/bom-1.6.schema.json" {
		t.Fatalf("unexpected schema %q", first.Schema)
	}
	if got := first.Components[0].PURL; got != "pkg:npm/%40scope/tool@1.2.3" {
		t.Fatalf("unexpected scoped npm purl %q", got)
	}
}
