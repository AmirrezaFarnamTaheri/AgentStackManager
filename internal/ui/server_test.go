package ui

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/agentstack/agentstack/internal/app"
	"github.com/agentstack/agentstack/internal/model"
	"github.com/agentstack/agentstack/internal/planner"
	"github.com/agentstack/agentstack/internal/resourcehub"
	"github.com/agentstack/agentstack/internal/reviewedplan"
	"github.com/agentstack/agentstack/internal/workspace"
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

func waitForAcceptedOperation(t *testing.T, handler http.Handler, response *httptest.ResponseRecorder) json.RawMessage {
	t.Helper()
	if response.Code != http.StatusAccepted {
		t.Fatalf("expected 202, got %d: %s", response.Code, response.Body.String())
	}
	var receipt struct {
		StatusURL string `json:"statusUrl"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &receipt); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(2 * time.Second)
	for {
		status := httptest.NewRecorder()
		request := httptest.NewRequest(http.MethodGet, receipt.StatusURL, nil)
		request.Header.Set("X-AgentStack-Token", "secret")
		handler.ServeHTTP(status, request)
		if status.Code != http.StatusOK {
			t.Fatalf("operation status=%d: %s", status.Code, status.Body.String())
		}
		var operation struct {
			Status string          `json:"status"`
			Result json.RawMessage `json:"result"`
			Error  string          `json:"error"`
		}
		if err := json.Unmarshal(status.Body.Bytes(), &operation); err != nil {
			t.Fatal(err)
		}
		switch operation.Status {
		case "succeeded":
			return operation.Result
		case "failed":
			t.Fatalf("operation failed: %s", operation.Error)
		}
		if time.Now().After(deadline) {
			t.Fatalf("operation did not complete: %s", status.Body.String())
		}
		time.Sleep(time.Millisecond)
	}
}

func TestEveryAPIEndpointRequiresToken(t *testing.T) {
	handler := newTestHandler(&fakeBackend{})
	for _, path := range []string{"status", "fabric", "catalog", "inventory", "mcp/doctor"} {
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
	waitForAcceptedOperation(t, handler, response)
	if backend.planID != "plan-1" || backend.digest != "sha256:plan" {
		t.Fatalf("apply did not use reviewed plan identity: %#v", backend)
	}
}

type unavailablePlanBackend struct{ fakeBackend }

func (b *unavailablePlanBackend) ApplyPlanned(context.Context, string, string, bool) (app.ApplyReport, error) {
	return app.ApplyReport{Transaction: model.Transaction{ID: "tx-partial", Status: model.TransactionFailed}},
		fmt.Errorf(`open C:\Users\ACER\AppData\Local\AgentStack\plans\missing.json: %w`, reviewedplan.ErrPlanUnavailable)
}

func TestApplyOperationMapsConsumedPlanToPathFreeRecovery(t *testing.T) {
	handler := newTestHandler(&unavailablePlanBackend{})
	accepted := httptest.NewRecorder()
	handler.ServeHTTP(accepted, authorizedRequest(http.MethodPost, "apply", `{"planId":"plan-1","digest":"sha256:plan","confirm":true}`))
	if accepted.Code != http.StatusAccepted {
		t.Fatalf("apply status=%d body=%s", accepted.Code, accepted.Body.String())
	}
	var receipt operationReceipt
	if err := json.Unmarshal(accepted.Body.Bytes(), &receipt); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(2 * time.Second)
	for {
		status := httptest.NewRecorder()
		request := httptest.NewRequest(http.MethodGet, receipt.StatusURL, nil)
		request.Header.Set("X-AgentStack-Token", "secret")
		handler.ServeHTTP(status, request)
		if status.Code != http.StatusOK {
			t.Fatalf("operation status=%d body=%s", status.Code, status.Body.String())
		}
		var operation operationStatus
		if err := json.Unmarshal(status.Body.Bytes(), &operation); err != nil {
			t.Fatal(err)
		}
		if operation.Status == "failed" {
			if operation.Failure == nil || operation.Failure.Code != "plan_unavailable" || !operation.Failure.Retryable {
				t.Fatalf("failure = %#v", operation.Failure)
			}
			body := strings.ToLower(status.Body.String())
			for _, forbidden := range []string{`c:\\`, "appdata", "missing.json"} {
				if strings.Contains(body, forbidden) {
					t.Fatalf("operation leaked %q: %s", forbidden, status.Body.String())
				}
			}
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("operation did not fail: %s", status.Body.String())
		}
		time.Sleep(time.Millisecond)
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
	waitForAcceptedOperation(t, handler, response)
	if calls != 1 {
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

type blockingApplyBackend struct {
	fakeBackend
	started chan struct{}
	release chan struct{}
}

func (b *blockingApplyBackend) ApplyPlanned(_ context.Context, planID, digest string, confirmed bool) (app.ApplyReport, error) {
	close(b.started)
	<-b.release
	return app.ApplyReport{Plan: model.Plan{ID: planID, Digest: digest}}, nil
}

func TestApplyReturnsAcceptedAndCompletesThroughOperationEndpoint(t *testing.T) {
	backend := &blockingApplyBackend{started: make(chan struct{}), release: make(chan struct{})}
	t.Cleanup(func() {
		select {
		case <-backend.release:
		default:
			close(backend.release)
		}
	})
	handler := newTestHandler(backend)
	response := httptest.NewRecorder()
	done := make(chan struct{})
	go func() {
		handler.ServeHTTP(response, authorizedRequest(http.MethodPost, "apply", `{"planId":"plan-1","digest":"sha256:plan","confirm":true}`))
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(250 * time.Millisecond):
		close(backend.release)
		t.Fatal("apply handler remained synchronously blocked on the long mutation")
	}
	if response.Code != http.StatusAccepted {
		t.Fatalf("long mutation must return 202 immediately, got %d: %s", response.Code, response.Body.String())
	}
	var accepted struct {
		OperationID string `json:"operationId"`
		Status      string `json:"status"`
		StatusURL   string `json:"statusUrl"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &accepted); err != nil {
		t.Fatal(err)
	}
	if accepted.OperationID == "" || accepted.Status != "running" || accepted.StatusURL == "" {
		t.Fatalf("invalid operation receipt: %#v", accepted)
	}
	select {
	case <-backend.started:
	case <-time.After(time.Second):
		t.Fatal("background mutation did not start")
	}
	close(backend.release)
	deadline := time.Now().Add(2 * time.Second)
	for {
		status := httptest.NewRecorder()
		request := httptest.NewRequest(http.MethodGet, accepted.StatusURL, nil)
		request.Header.Set("X-AgentStack-Token", "secret")
		handler.ServeHTTP(status, request)
		if status.Code != http.StatusOK {
			t.Fatalf("operation status=%d: %s", status.Code, status.Body.String())
		}
		var operation struct {
			Status string          `json:"status"`
			Result json.RawMessage `json:"result"`
		}
		if err := json.Unmarshal(status.Body.Bytes(), &operation); err != nil {
			t.Fatal(err)
		}
		if operation.Status == "succeeded" {
			if len(operation.Result) == 0 || string(operation.Result) == "null" {
				t.Fatal("completed operation omitted its result")
			}
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("operation did not complete: %#v", operation)
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func TestShutdownIsLockedWhileBackgroundMutationRuns(t *testing.T) {
	backend := &blockingApplyBackend{started: make(chan struct{}), release: make(chan struct{})}
	t.Cleanup(func() {
		select {
		case <-backend.release:
		default:
			close(backend.release)
		}
	})
	called := false
	handler := NewHandler(HandlerOptions{Backend: backend, Token: "secret", SessionID: "browser-session", Version: "test", Shutdown: func() { called = true }})
	applyResponse := httptest.NewRecorder()
	go handler.ServeHTTP(applyResponse, authorizedRequest(http.MethodPost, "apply", `{"planId":"plan-1","digest":"sha256:plan","confirm":true}`))
	<-backend.started
	shutdownResponse := httptest.NewRecorder()
	handler.ServeHTTP(shutdownResponse, authorizedRequest(http.MethodPost, "shutdown", `{}`))
	close(backend.release)
	if shutdownResponse.Code != http.StatusLocked || called {
		t.Fatalf("shutdown raced active mutation: status=%d called=%v", shutdownResponse.Code, called)
	}
}

func TestWriteJSONEmitsExactlyOneDocument(t *testing.T) {
	response := httptest.NewRecorder()
	writeJSON(response, http.StatusOK, map[string]any{"ok": true})
	decoder := json.NewDecoder(response.Body)
	var first map[string]any
	if err := decoder.Decode(&first); err != nil {
		t.Fatal(err)
	}
	var extra any
	if err := decoder.Decode(&extra); !errors.Is(err, io.EOF) {
		t.Fatalf("response contains more than one JSON document: %v body=%q", err, response.Body.String())
	}
}

func TestLongMutationSurvivesHTTPWriteTimeoutViaOperationReceipt(t *testing.T) {
	backend := &blockingApplyBackend{started: make(chan struct{}), release: make(chan struct{})}
	handler := newTestHandler(backend)
	server := httptest.NewUnstartedServer(handler)
	server.Config.WriteTimeout = 40 * time.Millisecond
	server.Start()
	defer server.Close()
	t.Cleanup(func() {
		select {
		case <-backend.release:
		default:
			close(backend.release)
		}
	})
	request, err := http.NewRequest(http.MethodPost, server.URL+testBase+"api/apply", strings.NewReader(`{"planId":"plan-1","digest":"sha256:plan","confirm":true}`))
	if err != nil {
		t.Fatal(err)
	}
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("X-AgentStack-Token", "secret")
	response, err := server.Client().Do(request)
	if err != nil {
		t.Fatalf("client lost operation receipt before write timeout: %v", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusAccepted {
		body, _ := io.ReadAll(response.Body)
		t.Fatalf("expected 202 receipt, got %d: %s", response.StatusCode, body)
	}
	var receipt operationReceipt
	if err := json.NewDecoder(response.Body).Decode(&receipt); err != nil {
		t.Fatal(err)
	}
	<-backend.started
	time.Sleep(80 * time.Millisecond)
	close(backend.release)
	deadline := time.Now().Add(2 * time.Second)
	for {
		statusRequest, err := http.NewRequest(http.MethodGet, server.URL+receipt.StatusURL, nil)
		if err != nil {
			t.Fatal(err)
		}
		statusRequest.Header.Set("X-AgentStack-Token", "secret")
		statusResponse, err := server.Client().Do(statusRequest)
		if err != nil {
			t.Fatal(err)
		}
		var operation operationStatus
		decodeErr := json.NewDecoder(statusResponse.Body).Decode(&operation)
		statusResponse.Body.Close()
		if decodeErr != nil {
			t.Fatal(decodeErr)
		}
		if operation.Status == "succeeded" {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("backend completed but operation was not observable: %+v", operation)
		}
		time.Sleep(10 * time.Millisecond)
	}
}

type fakeFabricBackend struct{ fakeBackend }

func (f *fakeFabricBackend) FabricStatus(time.Time) (app.FabricStatus, error) {
	return app.FabricStatus{Resources: 7, Workspaces: 2, Routines: 3, DueRoutines: 1}, nil
}

func TestFabricStatusEndpointReturnsUnifiedCounts(t *testing.T) {
	handler := newTestHandler(&fakeFabricBackend{})
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, authorizedRequest(http.MethodGet, "fabric", ""))
	if response.Code != http.StatusOK {
		t.Fatalf("fabric status=%d body=%s", response.Code, response.Body.String())
	}
	var status app.FabricStatus
	if err := json.Unmarshal(response.Body.Bytes(), &status); err != nil {
		t.Fatal(err)
	}
	if status.Resources != 7 || status.Workspaces != 2 || status.DueRoutines != 1 {
		t.Fatalf("unexpected fabric status: %#v", status)
	}
}

type progressApplyFakeBackend struct {
	fakeBackend
	started chan struct{}
	release chan struct{}
}

func (b *progressApplyFakeBackend) ApplyPlannedWithProgress(_ context.Context, planID, digest string, confirmed bool, onProgress func(app.ApplyProgress)) (app.ApplyReport, error) {
	onProgress(app.ApplyProgress{
		Phase: "installing", Completed: 1, Total: 2, CurrentID: "tool-b", CurrentLabel: "Tool B",
		Items: []app.ApplyProgressItem{
			{ID: "tool-a", Label: "Tool A", Action: "install", Status: "succeeded"},
			{ID: "tool-b", Label: "Tool B", Action: "install", Status: "running"},
		},
	})
	close(b.started)
	<-b.release
	return app.ApplyReport{Plan: model.Plan{ID: planID, Digest: digest}}, nil
}

func TestApplyEndpointPublishesProgress(t *testing.T) {
	backend := &progressApplyFakeBackend{started: make(chan struct{}), release: make(chan struct{})}
	t.Cleanup(func() {
		select {
		case <-backend.release:
		default:
			close(backend.release)
		}
	})
	handler := newTestHandler(backend)
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, authorizedRequest(http.MethodPost, "apply", `{"planId":"plan-1","digest":"sha256:plan","confirm":true}`))
	if response.Code != http.StatusAccepted {
		t.Fatalf("apply status=%d body=%s", response.Code, response.Body.String())
	}
	var accepted operationReceipt
	if err := json.Unmarshal(response.Body.Bytes(), &accepted); err != nil {
		t.Fatal(err)
	}
	select {
	case <-backend.started:
	case <-time.After(time.Second):
		t.Fatal("progress backend did not start")
	}
	status := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, accepted.StatusURL, nil)
	request.Header.Set("X-AgentStack-Token", "secret")
	handler.ServeHTTP(status, request)
	if status.Code != http.StatusOK {
		t.Fatalf("operation status=%d body=%s", status.Code, status.Body.String())
	}
	var operation operationStatus
	if err := json.Unmarshal(status.Body.Bytes(), &operation); err != nil {
		t.Fatal(err)
	}
	if operation.Progress == nil || operation.Progress.Phase != "installing" || operation.Progress.Completed != 1 || operation.Progress.CurrentID != "tool-b" {
		t.Fatalf("operation progress = %#v", operation.Progress)
	}
	close(backend.release)
}

type environmentBackend struct {
	fakeBackend
	resources    []resourcehub.Resource
	targets      []resourcehub.Target
	workspaces   []workspace.Item
	transactions []model.Transaction
}

func (b *environmentBackend) ListResources() ([]resourcehub.Resource, error) { return b.resources, nil }
func (b *environmentBackend) ListResourceTargets() ([]resourcehub.Target, error) {
	return b.targets, nil
}
func (b *environmentBackend) ListWorkspaces() ([]workspace.Item, error) { return b.workspaces, nil }
func (b *environmentBackend) ListTransactions(limit int) ([]model.Transaction, error) {
	if limit < len(b.transactions) {
		return b.transactions[:limit], nil
	}
	return b.transactions, nil
}

func TestEnvironmentAndTransactionEndpointsAreAuthenticated(t *testing.T) {
	backend := &environmentBackend{
		fakeBackend:  fakeBackend{},
		resources:    []resourcehub.Resource{{ID: "rules", Name: "Rules", Kind: resourcehub.KindRule, Enabled: true, Targets: []resourcehub.Agent{resourcehub.AgentCodex}}},
		targets:      []resourcehub.Target{{ID: "codex", Agent: resourcehub.AgentCodex, Enabled: true}},
		transactions: []model.Transaction{{ID: "tx-1", Status: model.TransactionSucceeded}},
	}
	handler := newTestHandler(backend)
	for _, endpoint := range []string{"environments", "transactions?limit=20"} {
		unauthorized := httptest.NewRecorder()
		handler.ServeHTTP(unauthorized, httptest.NewRequest(http.MethodGet, testBase+"api/"+endpoint, nil))
		if unauthorized.Code != http.StatusForbidden {
			t.Fatalf("%s unauthorized status=%d", endpoint, unauthorized.Code)
		}
		authorized := httptest.NewRecorder()
		request := httptest.NewRequest(http.MethodGet, testBase+"api/"+endpoint, nil)
		request.Header.Set("X-AgentStack-Token", "secret")
		handler.ServeHTTP(authorized, request)
		if authorized.Code != http.StatusOK {
			t.Fatalf("%s status=%d body=%s", endpoint, authorized.Code, authorized.Body.String())
		}
		if strings.Contains(strings.ToLower(authorized.Body.String()), "appdata") || strings.Contains(authorized.Body.String(), `C:\\`) {
			t.Fatalf("%s leaked private path: %s", endpoint, authorized.Body.String())
		}
	}
}

type failedApplyBackend struct{ fakeBackend }

func (b *failedApplyBackend) ApplyPlanned(_ context.Context, planID, digest string, confirmed bool) (app.ApplyReport, error) {
	plan := model.Plan{ID: planID, Digest: digest, Actions: []model.PlanAction{
		{ComponentID: "uv", Name: "uv", Kind: model.ActionInstall, Install: model.InstallSpec{Kind: model.InstallWinget}},
		{ComponentID: "git", Name: "Git", Kind: model.ActionKeep},
	}}
	tx := model.Transaction{ID: "tx-failed", Status: model.TransactionFailed, Actions: []model.TransactionAction{
		{ComponentID: "uv", Kind: model.ActionInstall, ExitCode: -1, Error: `exec: "winget": executable file not found in %PATH%`},
		{ComponentID: "git", Kind: model.ActionKeep, Verified: true},
	}}
	return app.ApplyReport{Plan: plan, Transaction: tx}, errors.New("one or more selected installations failed")
}

func TestApplyOperationReturnsTruthfulPublicOutcomeOnFailure(t *testing.T) {
	handler := newTestHandler(&failedApplyBackend{})
	accepted := httptest.NewRecorder()
	handler.ServeHTTP(accepted, authorizedRequest(http.MethodPost, "apply", `{"planId":"plan-1","digest":"sha256:plan","confirm":true}`))
	if accepted.Code != http.StatusAccepted {
		t.Fatalf("apply status=%d body=%s", accepted.Code, accepted.Body.String())
	}
	var receipt operationReceipt
	if err := json.Unmarshal(accepted.Body.Bytes(), &receipt); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(2 * time.Second)
	for {
		status := httptest.NewRecorder()
		request := httptest.NewRequest(http.MethodGet, receipt.StatusURL, nil)
		request.Header.Set("X-AgentStack-Token", "secret")
		handler.ServeHTTP(status, request)
		var operation struct {
			Status  string               `json:"status"`
			Failure *ClientFailure       `json:"failure"`
			Result  applyOperationResult `json:"result"`
		}
		if err := json.Unmarshal(status.Body.Bytes(), &operation); err != nil {
			t.Fatal(err)
		}
		if operation.Status == "failed" {
			if operation.Failure == nil || operation.Failure.Message != "No requested changes were applied." {
				t.Fatalf("failure = %#v", operation.Failure)
			}
			if operation.Result.Outcome.Requested != 1 || operation.Result.Outcome.Failed != 1 || operation.Result.Outcome.Unchanged != 1 {
				t.Fatalf("outcome = %#v", operation.Result.Outcome)
			}
			body := strings.ToLower(status.Body.String())
			for _, forbidden := range []string{"%path%", "executable file not found", "stderr", "stdout"} {
				if strings.Contains(body, forbidden) {
					t.Fatalf("operation leaked %q: %s", forbidden, status.Body.String())
				}
			}
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("operation did not finish: %s", status.Body.String())
		}
		time.Sleep(time.Millisecond)
	}
}

func TestEmbeddedAssetsAreCacheProofAndVersioned(t *testing.T) {
	handler := NewHandler(HandlerOptions{Backend: &fakeBackend{}, Token: "secret", SessionID: "browser-session", Version: "1.2.3", Revision: "rev-abc", InstallSelf: func() (any, error) { return map[string]any{}, nil }})

	page := httptest.NewRecorder()
	handler.ServeHTTP(page, httptest.NewRequest(http.MethodGet, testBase, nil))
	if page.Code != http.StatusOK {
		t.Fatalf("page status=%d body=%s", page.Code, page.Body.String())
	}
	if cache := page.Header().Get("Cache-Control"); !strings.Contains(cache, "no-store") || !strings.Contains(cache, "max-age=0") {
		t.Fatalf("page cache-control=%q", cache)
	}
	body := page.Body.String()
	for _, asset := range []string{"favicon.svg", "styles.css", "core.js", "changes.js", "environments.js", "activity.js", "app.js"} {
		if !strings.Contains(body, "assets/"+asset+"?v=") {
			t.Fatalf("versioned asset %s missing from page", asset)
		}
	}

	asset := httptest.NewRecorder()
	handler.ServeHTTP(asset, httptest.NewRequest(http.MethodGet, testBase+"assets/styles.css?v=test", nil))
	if asset.Code != http.StatusOK {
		t.Fatalf("asset status=%d body=%s", asset.Code, asset.Body.String())
	}
	if cache := asset.Header().Get("Cache-Control"); !strings.Contains(cache, "no-store") || !strings.Contains(cache, "max-age=0") {
		t.Fatalf("asset cache-control=%q", cache)
	}
}

func (b *environmentBackend) RegisterResourceTarget(target resourcehub.Target) error {
	for index := range b.targets {
		if b.targets[index].ID == target.ID {
			b.targets[index] = target
			return nil
		}
	}
	b.targets = append(b.targets, target)
	return nil
}

func TestEnvironmentTargetCanBeConnectedFromDetectedHome(t *testing.T) {
	home := t.TempDir()
	if err := os.MkdirAll(filepath.Join(home, ".codex"), 0o700); err != nil {
		t.Fatal(err)
	}
	backend := &environmentBackend{fakeBackend: fakeBackend{}}
	handler := NewHandler(HandlerOptions{Backend: backend, Token: "secret", SessionID: "browser-session", Version: "test", HomeDir: home})

	candidates := httptest.NewRecorder()
	handler.ServeHTTP(candidates, authorizedRequest(http.MethodGet, "environment-targets", ""))
	if candidates.Code != http.StatusOK || !strings.Contains(candidates.Body.String(), `"agent":"codex"`) || !strings.Contains(candidates.Body.String(), `"detected":true`) {
		t.Fatalf("candidates status=%d body=%s", candidates.Code, candidates.Body.String())
	}

	connected := httptest.NewRecorder()
	handler.ServeHTTP(connected, authorizedRequest(http.MethodPost, "environment-targets/connect", `{"agent":"codex","enabled":true}`))
	if connected.Code != http.StatusOK {
		t.Fatalf("connect status=%d body=%s", connected.Code, connected.Body.String())
	}

	overview := httptest.NewRecorder()
	handler.ServeHTTP(overview, authorizedRequest(http.MethodGet, "environments", ""))
	if overview.Code != http.StatusOK || !strings.Contains(overview.Body.String(), `"id":"codex"`) || !strings.Contains(overview.Body.String(), `"state":"connected"`) {
		t.Fatalf("overview status=%d body=%s", overview.Code, overview.Body.String())
	}
}

type targetMutationBackend struct {
	fakeBackend
	targets []resourcehub.Target
	writes  []resourcehub.Target
}

func (b *targetMutationBackend) ListResources() ([]resourcehub.Resource, error) { return nil, nil }
func (b *targetMutationBackend) ListResourceTargets() ([]resourcehub.Target, error) {
	return append([]resourcehub.Target(nil), b.targets...), nil
}
func (b *targetMutationBackend) ListWorkspaces() ([]workspace.Item, error) { return nil, nil }
func (b *targetMutationBackend) RegisterResourceTarget(target resourcehub.Target) error {
	b.writes = append(b.writes, target)
	for i := range b.targets {
		if b.targets[i].ID == target.ID {
			b.targets[i] = target
			return nil
		}
	}
	b.targets = append(b.targets, target)
	return nil
}

func TestEnvironmentTargetBatchConnectsMultipleProfiles(t *testing.T) {
	home := t.TempDir()
	project := filepath.Join(home, "project")
	if err := os.MkdirAll(project, 0o700); err != nil {
		t.Fatal(err)
	}
	backend := &targetMutationBackend{}
	handler := NewHandler(HandlerOptions{Backend: backend, Token: "secret", SessionID: "browser-session", Version: "test", HomeDir: home})
	body := `{"targets":[{"agent":"codex","id":"codex-personal","scope":"global","label":"Personal","enabled":true},{"agent":"claude","id":"claude-project","scope":"project","label":"Project","root":"` + filepath.ToSlash(project) + `","enabled":true}]}`
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, authorizedRequest(http.MethodPost, "environment-targets/batch", body))
	if response.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
	if len(backend.writes) != 2 {
		t.Fatalf("writes=%#v", backend.writes)
	}
	if backend.writes[0].ID == backend.writes[1].ID || !backend.writes[0].Enabled || !backend.writes[1].Enabled {
		t.Fatalf("targets=%#v", backend.writes)
	}
}

func TestEnvironmentTargetBatchRejectsKnownReadOnlyTargetBeforeWriting(t *testing.T) {
	backend := &targetMutationBackend{}
	handler := NewHandler(HandlerOptions{Backend: backend, Token: "secret", SessionID: "browser-session", Version: "test", HomeDir: t.TempDir()})
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, authorizedRequest(http.MethodPost, "environment-targets/batch", `{"targets":[{"agent":"codex","enabled":true},{"agent":"vscode","enabled":true}]}`))
	if response.Code != http.StatusBadRequest || len(backend.writes) != 0 {
		t.Fatalf("status=%d writes=%#v body=%s", response.Code, backend.writes, response.Body.String())
	}
}

type batchSyncBackend struct {
	fakeBackend
	planTargets   []string
	planResources []string
	parallel      int
	appliedID     string
	appliedDigest string
}

func (b *batchSyncBackend) PlanResourceBatchSync(targetIDs, resourceIDs []string, maxParallel int) (resourcehub.BatchSyncPlan, error) {
	b.planTargets = append([]string(nil), targetIDs...)
	b.planResources = append([]string(nil), resourceIDs...)
	b.parallel = maxParallel
	return resourcehub.BatchSyncPlan{ID: "batch-1", Digest: "sha256:batch", MaxParallel: maxParallel}, nil
}

func (b *batchSyncBackend) ApplyResourceBatchSync(_ context.Context, planID, digest string, confirmed bool) (resourcehub.BatchSyncReport, error) {
	if !confirmed {
		return resourcehub.BatchSyncReport{}, resourcehub.ErrConfirmationRequired
	}
	b.appliedID, b.appliedDigest = planID, digest
	return resourcehub.BatchSyncReport{PlanID: planID, Succeeded: 2}, nil
}

func TestSharingSyncBatchPlanAndApplyUseReviewedIdentity(t *testing.T) {
	backend := &batchSyncBackend{}
	handler := newTestHandler(backend)
	planned := httptest.NewRecorder()
	handler.ServeHTTP(planned, authorizedRequest(http.MethodPost, "sharing-sync/plan", `{"targetIds":["codex-a","claude-a"],"resourceIds":["skill-a"],"maxParallel":2}`))
	if planned.Code != http.StatusOK {
		t.Fatalf("plan status=%d body=%s", planned.Code, planned.Body.String())
	}
	if strings.Join(backend.planTargets, ",") != "codex-a,claude-a" || strings.Join(backend.planResources, ",") != "skill-a" || backend.parallel != 2 {
		t.Fatalf("batch plan inputs = %#v %#v %d", backend.planTargets, backend.planResources, backend.parallel)
	}
	applied := httptest.NewRecorder()
	handler.ServeHTTP(applied, authorizedRequest(http.MethodPost, "sharing-sync/apply", `{"planId":"batch-1","digest":"sha256:batch","confirm":true}`))
	waitForAcceptedOperation(t, handler, applied)
	if backend.appliedID != "batch-1" || backend.appliedDigest != "sha256:batch" {
		t.Fatalf("batch apply identity = %q %q", backend.appliedID, backend.appliedDigest)
	}
}

func TestRunDesktopLauncherOwnsServerLifetimeWithoutPrintingURL(t *testing.T) {
	launched := make(chan string, 1)
	release := make(chan struct{})
	var output strings.Builder
	runDone := make(chan error, 1)
	go func() {
		runDone <- Run(context.Background(), HandlerOptions{Backend: &fakeBackend{}, Version: "test"}, RunOptions{
			ListenAddress: "127.0.0.1:0",
			Random:        strings.NewReader(strings.Repeat("d", 128)),
			Output:        &output,
			PrintURL:      false,
			Launcher: func(_ context.Context, target string) error {
				launched <- target
				<-release
				return nil
			},
		})
	}()
	select {
	case target := <-launched:
		if !strings.HasPrefix(target, "http://127.0.0.1:") || !strings.Contains(target, "/session/") {
			t.Fatalf("launcher target=%q", target)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("desktop launcher was not invoked")
	}
	if output.Len() != 0 {
		t.Fatalf("desktop launch leaked loopback URL to output: %q", output.String())
	}
	close(release)
	select {
	case err := <-runDone:
		if err != nil {
			t.Fatalf("desktop shutdown=%v", err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("server did not stop when desktop window closed")
	}
}
