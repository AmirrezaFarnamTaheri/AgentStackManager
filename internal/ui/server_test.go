package ui

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/agentstack/agentstack/internal/app"
	"github.com/agentstack/agentstack/internal/model"
	"github.com/agentstack/agentstack/internal/planner"
)

type fakeBackend struct {
	applyCalls int
	planID     string
	digest     string
}

func (f *fakeBackend) CatalogSnapshot() model.Catalog {
	return model.Catalog{Version: 1, Components: []model.Component{{ID: "git", Name: "Git", Tier: model.TierEssential, Install: model.InstallSpec{Kind: model.InstallWinget}}}, Profiles: []model.Profile{{ID: "essential", Components: []string{"git"}}}}
}
func (f *fakeBackend) Inventory(context.Context) (model.Inventory, error) {
	return model.Inventory{Items: map[string]model.InventoryItem{"git": {ComponentID: "git", Installed: false}}}, nil
}
func (f *fakeBackend) Plan(_ context.Context, request planner.Request) (model.Plan, error) {
	return model.Plan{ID: "plan-1", Digest: "sha256:plan", Profile: request.Profile, Actions: []model.PlanAction{{ComponentID: "git", Kind: model.ActionInstall}}}, nil
}
func (f *fakeBackend) ApplyPlanned(_ context.Context, planID, digest string, confirmed bool) (app.ApplyReport, error) {
	f.applyCalls++
	f.planID, f.digest = planID, digest
	return app.ApplyReport{Plan: model.Plan{ID: planID, Digest: digest}}, nil
}
func (f *fakeBackend) MCPInit(context.Context, app.MCPInitOptions) (app.MCPInitReport, error) {
	return app.MCPInitReport{RouterConfigPath: "router.json"}, nil
}
func (f *fakeBackend) MCPDoctor(context.Context) (app.DoctorReport, error) {
	return app.DoctorReport{Healthy: true}, nil
}

const testBase = "/session/browser-session/"

func newTestHandler(backend Backend) http.Handler {
	return NewHandler(HandlerOptions{Backend: backend, Token: "secret", SessionID: "browser-session", Version: "test", InstallSelf: func() (any, error) { return map[string]any{}, nil }})
}

func authorizedRequest(method, path, body string) *http.Request {
	request := httptest.NewRequest(method, testBase+"api/"+path, strings.NewReader(body))
	request.Header.Set("X-AgentStack-Token", "secret")
	if body != "" {
		request.Header.Set("Content-Type", "application/json")
	}
	return request
}

func TestEveryAPIEndpointRequiresToken(t *testing.T) {
	handler := newTestHandler(&fakeBackend{})
	for _, path := range []string{"status", "catalog", "inventory", "mcp/doctor"} {
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, testBase+"api/"+path, nil))
		if response.Code != http.StatusForbidden {
			t.Fatalf("%s expected 403, got %d", path, response.Code)
		}
	}
}

func TestApplyEndpointRequiresToken(t *testing.T) {
	backend := &fakeBackend{}
	handler := newTestHandler(backend)
	request := httptest.NewRequest(http.MethodPost, testBase+"api/apply", strings.NewReader(`{"planId":"plan-1","digest":"sha256:plan","confirm":true}`))
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusForbidden || backend.applyCalls != 0 {
		t.Fatalf("unauthorized apply status=%d calls=%d", response.Code, backend.applyCalls)
	}
}

func TestApplyEndpointUsesReviewedPlanIdentity(t *testing.T) {
	backend := &fakeBackend{}
	handler := newTestHandler(backend)
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, authorizedRequest(http.MethodPost, "apply", `{"planId":"plan-1","digest":"sha256:plan","confirm":true}`))
	if response.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", response.Code, response.Body.String())
	}
	if backend.planID != "plan-1" || backend.digest != "sha256:plan" {
		t.Fatalf("apply did not use reviewed plan identity: %#v", backend)
	}
}

func TestApplyEndpointRequiresExplicitConfirmation(t *testing.T) {
	backend := &fakeBackend{}
	handler := newTestHandler(backend)
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, authorizedRequest(http.MethodPost, "apply", `{"planId":"plan-1","digest":"sha256:plan","confirm":false}`))
	if response.Code != http.StatusBadRequest || backend.applyCalls != 0 {
		t.Fatalf("confirmation status=%d calls=%d", response.Code, backend.applyCalls)
	}
}

func TestInstallSelfEndpointRequiresExplicitConfirmation(t *testing.T) {
	calls := 0
	handler := NewHandler(HandlerOptions{
		Backend:   &fakeBackend{},
		Token:     "secret",
		SessionID: "browser-session",
		Version:   "test",
		InstallSelf: func() (any, error) {
			calls++
			return map[string]any{"installed": true}, nil
		},
	})
	for _, body := range []string{`{}`, `{"confirm":false}`} {
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, authorizedRequest(http.MethodPost, "install-self", body))
		if response.Code != http.StatusBadRequest || calls != 0 {
			t.Fatalf("unconfirmed install-self status=%d calls=%d body=%s", response.Code, calls, response.Body.String())
		}
	}
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, authorizedRequest(http.MethodPost, "install-self", `{"confirm":true}`))
	if response.Code != http.StatusOK || calls != 1 {
		t.Fatalf("confirmed install-self status=%d calls=%d body=%s", response.Code, calls, response.Body.String())
	}
}

func TestPlanEndpointReturnsSealedPlan(t *testing.T) {
	backend := &fakeBackend{}
	handler := newTestHandler(backend)
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, authorizedRequest(http.MethodPost, "plan", `{"profile":"essential"}`))
	if response.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", response.Code, response.Body.String())
	}
	var decoded model.Plan
	if err := json.Unmarshal(response.Body.Bytes(), &decoded); err != nil {
		t.Fatal(err)
	}
	if decoded.ID == "" || decoded.Digest == "" {
		t.Fatalf("plan identity missing: %#v", decoded)
	}
}

func TestIndexExistsOnlyAtUnguessableSessionPath(t *testing.T) {
	handler := newTestHandler(&fakeBackend{})
	root := httptest.NewRecorder()
	handler.ServeHTTP(root, httptest.NewRequest(http.MethodGet, "/", nil))
	if root.Code != http.StatusNotFound {
		t.Fatalf("root must not reveal session UI, got %d", root.Code)
	}
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, testBase, nil))
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), `content="secret"`) || !strings.Contains(response.Body.String(), `content="/session/browser-session/"`) {
		t.Fatalf("session page mismatch: status=%d body=%s", response.Code, response.Body.String())
	}
}

func TestShutdownEndpointRequiresTokenAndInvokesCallback(t *testing.T) {
	called := false
	handler := NewHandler(HandlerOptions{Backend: &fakeBackend{}, Token: "secret", SessionID: "browser-session", Version: "test", Shutdown: func() { called = true }})
	unauthorizedResponse := httptest.NewRecorder()
	handler.ServeHTTP(unauthorizedResponse, httptest.NewRequest(http.MethodPost, testBase+"api/shutdown", strings.NewReader(`{}`)))
	if unauthorizedResponse.Code != http.StatusForbidden || called {
		t.Fatalf("unauthorized shutdown changed state: status=%d called=%v", unauthorizedResponse.Code, called)
	}
	authorizedResponse := httptest.NewRecorder()
	handler.ServeHTTP(authorizedResponse, authorizedRequest(http.MethodPost, "shutdown", `{}`))
	if authorizedResponse.Code != http.StatusOK || !called {
		t.Fatalf("authorized shutdown failed: status=%d called=%v body=%s", authorizedResponse.Code, called, authorizedResponse.Body.String())
	}
}

type failingReader struct{}

func (failingReader) Read([]byte) (int, error) { return 0, errors.New("entropy unavailable") }

func TestRandomTokenFailureIsFatal(t *testing.T) {
	if _, err := randomToken(failingReader{}, 24); err == nil {
		t.Fatal("entropy failure must not produce a predictable token")
	}
}

func TestRunStartsLoopbackServerAndStopsOnContextCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	err := Run(ctx, HandlerOptions{Backend: &fakeBackend{}, Version: "test"}, RunOptions{ListenAddress: "127.0.0.1:0", OpenBrowser: false, Random: strings.NewReader(strings.Repeat("r", 128))})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("expected cancellation, got %v", err)
	}
}

func TestRunRejectsNonLoopbackAddress(t *testing.T) {
	err := Run(context.Background(), HandlerOptions{Backend: &fakeBackend{}, Version: "test"}, RunOptions{ListenAddress: "0.0.0.0:0", OpenBrowser: false, Random: strings.NewReader(strings.Repeat("r", 128))})
	if err == nil || !strings.Contains(err.Error(), "non-loopback") {
		t.Fatalf("expected non-loopback refusal, got %v", err)
	}
}
