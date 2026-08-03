package ui

import (
	"strings"
	"testing"
)

func TestEmbeddedUIOperationFeedbackContract(t *testing.T) {
	htmlBytes, err := webAssets.ReadFile("web/index.html")
	if err != nil {
		t.Fatal(err)
	}
	jsBytes, err := webAssets.ReadFile("web/app.js")
	if err != nil {
		t.Fatal(err)
	}
	cssBytes, err := webAssets.ReadFile("web/styles.css")
	if err != nil {
		t.Fatal(err)
	}
	html := string(htmlBytes)
	js := string(jsBytes)
	css := string(cssBytes)

	for _, fragment := range []string{
		`id="operationStatus"`,
		`role="status"`,
		`aria-live="polite"`,
		`aria-atomic="true"`,
		`data-state="idle"`,
	} {
		if !strings.Contains(html, fragment) {
			t.Fatalf("operation status contract missing %q", fragment)
		}
	}

	for _, id := range []string{
		"refreshBtn", "refreshFabricBtn", "installSelfBtn", "exitBtn", "buildPlanBtn", "doctorBtn",
		"applyBtn", "mcpInitBtn", "mcpDoctorBtn", "retryLoadBtn",
	} {
		marker := `id="` + id + `"`
		index := strings.Index(html, marker)
		if index < 0 {
			t.Fatalf("button %s is missing", id)
		}
		end := strings.Index(html[index:], ">")
		if end < 0 {
			t.Fatalf("button %s opening tag is malformed", id)
		}
		tag := html[index : index+end]
		if !strings.Contains(tag, "data-operation") || !strings.Contains(tag, "data-busy-label=") {
			t.Fatalf("button %s does not declare the shared busy-state contract: %s", id, tag)
		}
	}

	for _, fragment := range []string{
		"async function runOperation(",
		"function setOperationStatus(",
		"aria-busy",
		"data-original-label",
		"main.setAttribute('aria-busy', 'true')",
		"function isVisible(",
	} {
		if !strings.Contains(js, fragment) {
			t.Fatalf("busy-state implementation missing %q", fragment)
		}
	}

	if !strings.Contains(html, `id="confirmApply" data-operation-lock`) {
		t.Fatal("plan confirmation is not locked during active operations")
	}
	if !strings.Contains(js, `data-operation-lock data-id=`) || !strings.Contains(js, `input.dataset.wasDisabled`) {
		t.Fatal("dynamic component controls do not preserve operation-lock state")
	}

	for _, fragment := range []string{
		".operation-status",
		".button.is-busy",
		"@keyframes operation-spin",
		"prefers-reduced-motion",
	} {
		if !strings.Contains(css, fragment) {
			t.Fatalf("busy-state styling missing %q", fragment)
		}
	}
	if !strings.Contains(css, "transition-duration:0s!important;animation-duration:0s!important") {
		t.Fatal("reduced-motion contract must fully disable transition and animation durations")
	}
	if strings.Contains(css, "transition-duration:.01ms!important") || strings.Contains(css, "animation-duration:.01ms!important") {
		t.Fatal("reduced-motion contract retains nonzero animation timing")
	}

	if strings.Contains(js, "Promise.all([api('catalog'), api('inventory'), api('fabric')])") {
		t.Fatal("optional fabric status must not block the primary catalog and inventory workflow")
	}
	if !strings.Contains(js, "api('fabric').then(value => ({ value })).catch(error => ({ error }))") || !strings.Contains(js, "showFabricError(fabricResult.error)") {
		t.Fatal("fabric status does not degrade independently when its endpoint fails")
	}

	if !strings.Contains(js, "void runOperation(null, 'Load AgentStack Manager', refresh)") {
		t.Fatal("startup refresh must not steal initial keyboard focus from the skip link")
	}
	for _, fragment := range []string{
		`id="overviewLoadError"`, `id="retryLoadBtn"`, `role="alert"`,
		`id="componentSearchStatus"`, `aria-describedby=`, `health-message`,
		`$('shutdownTitle').focus`, `state.selected.delete(previousProvider)`,
		`setCatalogControlsAvailable(false)`, `Reviewed plan applied.`,
		`control.dataset.wasDisabled = available ? 'false' : 'true'`,
		`$('planContent').hidden = true`,
	} {
		if !strings.Contains(html+js, fragment) {
			t.Fatalf("recovery and accessibility contract missing %q", fragment)
		}
	}
}

func TestEmbeddedUINavigationUsesOneIconLanguage(t *testing.T) {
	htmlBytes, err := webAssets.ReadFile("web/index.html")
	if err != nil {
		t.Fatal(err)
	}
	html := string(htmlBytes)
	for _, icon := range []string{"lucide-layout-dashboard", "lucide-package", "lucide-list-check", "lucide-router"} {
		if !strings.Contains(html, icon) {
			t.Fatalf("navigation icon %s is missing", icon)
		}
	}
	if strings.Contains(html, `aria-hidden="true">⌂`) || strings.Contains(html, `aria-hidden="true">□`) || strings.Contains(html, `aria-hidden="true">⌘`) {
		t.Fatal("navigation still mixes Unicode symbols with the SVG icon system")
	}
}

func TestWebFabricStatusFailureDoesNotBlockPrimaryRefresh(t *testing.T) {
	jsBytes, err := webAssets.ReadFile("web/app.js")
	if err != nil {
		t.Fatal(err)
	}
	js := string(jsBytes)
	if strings.Contains(js, "Promise.all([api('catalog'), api('inventory'), api('fabric')])") {
		t.Fatal("optional fabric status must not block the primary catalog and inventory workflow")
	}
	for _, fragment := range []string{
		"api('fabric').then(value => ({ value })).catch(error => ({ error }))",
		"showFabricError(fabricResult.error)",
	} {
		if !strings.Contains(js, fragment) {
			t.Fatalf("independent fabric degradation contract missing %q", fragment)
		}
	}
}
