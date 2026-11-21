package ui

import (
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"
)

func embeddedAsset(t *testing.T, name string) string {
	t.Helper()
	value, err := webAssets.ReadFile("web/" + name)
	if err != nil {
		t.Fatal(err)
	}
	return string(value)
}

func TestEmbeddedUILifecycleWorkspaceContract(t *testing.T) {
	html := embeddedAsset(t, "index.html")
	for _, fragment := range []string{
		`class="brand-mark"`, `class="brand-glyph">A<`, `class="brand-glyph">S<`,
		`data-section="home"`, `data-section="environments"`, `data-section="changes"`, `data-section="activity"`,
		`id="home"`, `id="environments"`, `id="changes"`, `id="activity"`,
		`<span>Home</span>`, `<span>Environments</span>`, `<span>Changes</span>`, `<span>Activity</span>`,
		`id="installTracker"`, `id="installProgress"`, `id="installStage"`, `id="installCount"`, `id="installItems"`,
		`id="outcomeSummary"`, `id="outcomeStats"`, `id="causeSummary"`, `id="resultFilters"`,
		`id="retryFailedBtn"`, `id="reviewFailedBtn"`, `id="createFreshPlanBtn"`, `id="technicalDetails"`, `id="technicalDiagnostics"`,
		`id="selectionIssues"`,
		`id="environmentList"`, `id="environmentDetail"`, `id="connectionList"`,
		`id="changeList"`, `id="confirmApply"`, `id="applyBtn"`,
		`aria-live="polite"`, `aria-atomic="true"`,
	} {
		if !strings.Contains(html, fragment) {
			t.Fatalf("lifecycle workspace missing %q", fragment)
		}
	}
	for _, forbidden := range []string{
		`data-section="overview"`, `data-section="components"`, `data-section="plan"`, `data-section="router"`,
		`<span>Setup</span>`, `<span>Tools</span>`, `<span>Review</span>`, `<span>Operate</span>`,
		`id="advancedNav"`, `id="planModal"`,
	} {
		if strings.Contains(html, forbidden) {
			t.Fatalf("lifecycle workspace retains obsolete primary structure %q", forbidden)
		}
	}
}

func TestEmbeddedUIPlainLanguageContract(t *testing.T) {
	content := strings.ToLower(embeddedAsset(t, "index.html") + embeddedAsset(t, "core.js") + embeddedAsset(t, "changes.js") + embeddedAsset(t, "environments.js") + embeddedAsset(t, "activity.js"))
	for _, forbidden := range []string{
		"sealed install plan", "responsibility plane", "record seal", "inventory-bound", "loopback only", "unified fabric", "build safe plan",
	} {
		if strings.Contains(content, forbidden) {
			t.Fatalf("user-facing implementation retains jargon %q", forbidden)
		}
	}
	for _, required := range []string{
		"pending changes", "create changes", "approve and apply", "create fresh plan", "needs attention", "technical details",
	} {
		if !strings.Contains(content, required) {
			t.Fatalf("plain-language contract missing %q", required)
		}
	}
}

func TestEmbeddedUIBrowserModuleContract(t *testing.T) {
	html := embeddedAsset(t, "index.html")
	ordered := []string{"assets/core.js", "assets/changes.js", "assets/environments.js", "assets/activity.js", "assets/app.js"}
	last := -1
	for _, script := range ordered {
		index := strings.Index(html, script)
		if index < 0 || index <= last {
			t.Fatalf("script order is invalid for %s", script)
		}
		last = index
	}
	app := embeddedAsset(t, "app.js")
	if lines := strings.Count(app, "\n") + 1; lines > 160 {
		t.Fatalf("bootstrap app.js has %d lines; expected at most 160", lines)
	}
	all := embeddedAsset(t, "core.js") + embeddedAsset(t, "changes.js") + embeddedAsset(t, "environments.js") + embeddedAsset(t, "activity.js") + app
	if strings.Count(all, "async function api(") != 1 {
		t.Fatalf("API client must have one implementation")
	}
	if strings.Count(all, "async function runOperation(") != 1 {
		t.Fatalf("operation controller must have one implementation")
	}
	for _, fragment := range []string{"window.AgentStack", "planConsumed", "operation.failure", "operation.progress", "renderProgress", "loadTransactions", "await options.onSuccess?.(result)", "await options.onFailure?.(error)"} {
		if !strings.Contains(all, fragment) {
			t.Fatalf("browser module contract missing %q", fragment)
		}
	}
}

func TestEmbeddedUILifecycleStylesContract(t *testing.T) {
	css := embeddedAsset(t, "styles.css")
	for _, fragment := range []string{
		".workspace-rail", ".brand-glyph", ".environment-list", ".change-workspace", ".install-tracker", ".transaction-list", ".technical-details",
		`[aria-current="page"]`, "progress", ".state-badge", ".result-table", "min-height:44px", ":focus-visible", "prefers-reduced-motion", `html[data-theme="dark"]`,
	} {
		if !strings.Contains(strings.ReplaceAll(css, " ", ""), strings.ReplaceAll(fragment, " ", "")) {
			t.Fatalf("lifecycle styles missing %q", fragment)
		}
	}
	if strings.Contains(css, "color-mix(") {
		t.Fatal("dynamic color mixing is not allowed")
	}
	fontSizePattern := regexp.MustCompile(`font-size:\s*([0-9]+)px`)
	for _, match := range fontSizePattern.FindAllStringSubmatch(css, -1) {
		size, err := strconv.Atoi(match[1])
		if err != nil {
			t.Fatal(err)
		}
		if size < 12 {
			t.Fatalf("UI text size %dpx is below the 12px floor", size)
		}
	}
}

func TestEmbeddedUIOperationAndRecoveryContract(t *testing.T) {
	core := embeddedAsset(t, "core.js")
	changes := embeddedAsset(t, "changes.js")
	activity := embeddedAsset(t, "activity.js")
	for _, fragment := range []string{
		"onProgress?.(operation.progress)", "operation.failure", "error.data = operation", "async function runOperation(",
		"state.planConsumed = true", "createFreshPlan", "Retry failed items", "retryFailedBtn", "reviewFailedBtn", "createFreshPlanBtn",
		"function renderProgress(", "progress.processed", "progress.succeeded", "progress.failed", "progress.total", "MAX_ACTIVITY_ENTRIES = 50",
		"function validateSelection(", "function includeProviderDependencies(", "renderTechnicalDiagnostics", "renderOutcome", "cancelled",
		"resultSort", "updateResultFilterCounts", "item-result-details",
	} {
		if !strings.Contains(core+changes+activity, fragment) {
			t.Fatalf("operation recovery contract missing %q", fragment)
		}
	}
	if strings.Contains(core+changes+activity, `C:\`) || strings.Contains(strings.ToLower(core+changes+activity), "appdata") {
		t.Fatal("browser source contains a private path example")
	}
}

func TestEmbeddedUITruthfulOutcomeLanguageContract(t *testing.T) {
	content := embeddedAsset(t, "index.html") + embeddedAsset(t, "activity.js") + embeddedAsset(t, "changes.js")
	for _, forbidden := range []string{"Changes complete", "of ${total} completed", "Successful changes were kept", "technicalOutput", "tracker-dot"} {
		if strings.Contains(content, forbidden) {
			t.Fatalf("truthful outcome UI retains misleading pattern %q", forbidden)
		}
	}
	for _, required := range []string{"Run finished", "processed", "No requested changes were applied", "Existing verified items were left unchanged", "What needs attention", "Next action", "Retry unfinished items"} {
		if !strings.Contains(content, required) {
			t.Fatalf("truthful outcome UI missing %q", required)
		}
	}
}

func TestEmbeddedUICanonicalHomeCountsContract(t *testing.T) {
	content := embeddedAsset(t, "environments.js") + embeddedAsset(t, "app.js")
	for _, required := range []string{"Object.values(state.inventory?.items || {})", "renderOperationalStatus", "not-connected", "Not connected"} {
		if !strings.Contains(content, required) {
			t.Fatalf("canonical home status contract missing %q", required)
		}
	}
}

func cssAtRuleBlock(t *testing.T, css, header string) string {
	t.Helper()
	start := strings.Index(css, header)
	if start < 0 {
		t.Fatalf("CSS at-rule %q is missing", header)
	}
	open := strings.Index(css[start:], "{")
	if open < 0 {
		t.Fatalf("CSS at-rule %q has no opening brace", header)
	}
	open += start
	depth := 0
	for index := open; index < len(css); index++ {
		switch css[index] {
		case '{':
			depth++
		case '}':
			depth--
			if depth == 0 {
				return css[open+1 : index]
			}
		}
	}
	t.Fatalf("CSS at-rule %q has no closing brace", header)
	return ""
}

func TestEmbeddedUIResponsiveResultTableRulesStayInsideMobileMedia(t *testing.T) {
	css := embeddedAsset(t, "styles.css")
	mobileRules := cssAtRuleBlock(t, css, "@media (max-width: 760px)")
	for _, fragment := range []string{
		".outcome-stats { grid-template-columns: repeat(2",
		".outcome-stats div:last-child { grid-column: 1 / -1; }",
		".result-table, .result-table tbody",
		".result-table td:nth-child(5)::before",
	} {
		if !strings.Contains(mobileRules, fragment) {
			t.Fatalf("mobile result rule %q must stay inside the 760px media block", fragment)
		}
	}
}

func TestEmbeddedUIHomeNextActionResetsTargetForAttentionState(t *testing.T) {
	content := embeddedAsset(t, "environments.js")
	if strings.Count(content, "$('homeNextAction').dataset.targetSection = 'changes';") < 2 {
		t.Fatal("home next action must explicitly reset to Changes in both attention and ready branches")
	}
}

func TestLifecycleWorkspaceDocumentationContract(t *testing.T) {
	paths := []string{
		filepath.Join("..", "..", "README.md"),
		filepath.Join("..", "..", "docs", "UI_LIFECYCLE_WORKSPACE.md"),
		filepath.Join("..", "..", "docs", "OPERATIONS.md"),
	}
	var combined strings.Builder
	for _, path := range paths {
		content, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		combined.Write(content)
		combined.WriteByte('\n')
	}
	text := strings.ToLower(combined.String())
	for _, fragment := range []string{
		"phase and outcome are independent",
		"processed",
		"partially_failed",
		"retry failed items",
		"fresh reviewed plan",
		"sanitized diagnostics",
		"root-cause",
		"not connected",
	} {
		if !strings.Contains(text, fragment) {
			t.Fatalf("lifecycle documentation missing %q", fragment)
		}
	}
	for _, forbidden := range []string{
		"successful changes were kept",
		"completed and total item counts",
	} {
		if strings.Contains(text, forbidden) {
			t.Fatalf("lifecycle documentation retains misleading wording %q", forbidden)
		}
	}
}

func TestEmbeddedUITechnicalDetailsAvoidNestedScrollTrap(t *testing.T) {
	css := embeddedAsset(t, "styles.css")
	if strings.Contains(css, ".technical-diagnostics { display: grid; gap: 8px; max-height:") {
		t.Fatal("technical diagnostics must use page flow instead of a nested vertical scroll region")
	}
}

func TestEmbeddedUIActivityTimelineContract(t *testing.T) {
	activity := embeddedAsset(t, "activity.js")
	for _, fragment := range []string{
		"function transactionMetrics(",
		"function durationLabel(",
		"unchanged",
		"fresh plan",
		"Finished in",
	} {
		if !strings.Contains(activity, fragment) {
			t.Fatalf("activity timeline contract missing %q", fragment)
		}
	}
}

func TestEmbeddedUIEnvironmentDetailAvoidsEmptyHeightTrap(t *testing.T) {
	css := embeddedAsset(t, "styles.css")
	if strings.Contains(css, ".environment-detail { min-height:") {
		t.Fatal("environment detail must size to content instead of reserving an oversized empty panel")
	}
}

func TestEmbeddedUIResultOutputHardeningContract(t *testing.T) {
	activity := embeddedAsset(t, "activity.js")
	core := embeddedAsset(t, "core.js")
	css := embeddedAsset(t, "styles.css")
	for _, forbidden := range []string{
		"Shared root cause above.",
		"Use the recovery action above.",
		"<dt>Recovery</dt>",
		".result-table th, .result-table td { padding: 11px 12px; border-bottom: 1px solid var(--line); text-align: left; vertical-align: top; overflow-wrap: anywhere; }",
	} {
		if strings.Contains(activity+css, forbidden) {
			t.Fatalf("result output retains unreadable pattern %q", forbidden)
		}
	}
	for _, required := range []string{
		"function transactionPresentation(transaction)",
		"metrics.succeeded > 0 && metrics.failed > 0",
		"function formatDiagnosticCode(",
		"errorCode",
		"technical-diagnostic-group",
		"Completed with issues",
		"date.getFullYear() <= 1900",
		".result-action { display: grid;",
		"overflow-wrap: break-word",
	} {
		if !strings.Contains(activity+core+css, required) {
			t.Fatalf("result output hardening is missing %q", required)
		}
	}
}

func TestEmbeddedUIHealthRepairAndHistoryWidgetsContract(t *testing.T) {
	html := embeddedAsset(t, "index.html")
	environments := embeddedAsset(t, "environments.js")
	activity := embeddedAsset(t, "activity.js")
	for _, required := range []string{
		`id="environmentHealthSummary"`, `id="repairQueueSummary"`, `id="historySearch"`, `id="historyStatusFilter"`,
		"function renderEnvironmentHealth(", "function renderRepairQueue(", "function filteredTransactions(",
	} {
		if !strings.Contains(html+environments+activity, required) {
			t.Fatalf("health/repair/history contract missing %q", required)
		}
	}
}

func TestEmbeddedUIBrandGlyphsStayHorizontal(t *testing.T) {
	css := embeddedAsset(t, "styles.css")
	if strings.Contains(css, ".brand-block strong, .brand-block span { display: block; }") {
		t.Fatal("broad brand span rule overrides the horizontal logo layout")
	}
	for _, required := range []string{
		".brand-mark { width: 38px; height: 38px; display: inline-flex;",
		".brand-block > div strong, .brand-block > div span { display: block; }",
	} {
		if !strings.Contains(css, required) {
			t.Fatalf("brand layout contract missing %q", required)
		}
	}
}

func TestEmbeddedUISharingSyncWorkspaceContract(t *testing.T) {
	html := embeddedAsset(t, "index.html")
	core := embeddedAsset(t, "core.js")
	app := embeddedAsset(t, "app.js")
	environments := embeddedAsset(t, "environments.js")
	css := embeddedAsset(t, "styles.css")
	for _, required := range []string{
		`data-section="sync"`, `<span>Sharing &amp; Sync</span>`, `id="sync"`, `id="syncStats"`,
		`id="syncResourceList"`, `id="syncDuplicateList"`, `id="syncPlanPanel"`, `assets/sync.js`,
		"sharing-sync/plan", "sharing-sync/apply", "Connect selected", "Pause selected",
		"environment-targets/batch", "sync-card-grid", "sync-resource-row", "sync-duplicate-row",
	} {
		if !strings.Contains(html+core+app+environments+css+embeddedAsset(t, "sync.js"), required) {
			t.Fatalf("sharing and sync workspace missing %q", required)
		}
	}
	if !strings.Contains(app, `['home', 'environments', 'sync', 'changes', 'activity']`) {
		t.Fatal("Sharing & Sync is not routable as a primary workspace")
	}
	environmentsIndex := strings.Index(app, "await AS.environments.load()")
	syncIndex := strings.Index(app, "AS.sync.load()")
	if environmentsIndex < 0 || syncIndex < 0 || environmentsIndex > syncIndex {
		t.Fatal("target discovery must finish before Sharing & Sync renders connected targets")
	}
}
