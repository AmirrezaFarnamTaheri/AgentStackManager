package ui

import (
	"context"
	"crypto/rand"
	"embed"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"log"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/agentstack/agentstack/internal/app"
	"github.com/agentstack/agentstack/internal/model"
	"github.com/agentstack/agentstack/internal/planner"
	"github.com/agentstack/agentstack/internal/selfinstall"
)

//go:embed web/*
var webAssets embed.FS

type Backend interface {
	CatalogSnapshot() model.Catalog
	Inventory(context.Context) (model.Inventory, error)
	Plan(context.Context, planner.Request) (model.Plan, error)
	ApplyPlanned(context.Context, string, string, bool) (app.ApplyReport, error)
	MCPInit(context.Context, app.MCPInitOptions) (app.MCPInitReport, error)
	MCPDoctor(context.Context) (app.DoctorReport, error)
}

type fabricStatusBackend interface {
	FabricStatus(time.Time) (app.FabricStatus, error)
}

type HandlerOptions struct {
	Backend     Backend
	Context     context.Context
	Token       string
	SessionID   string
	Version     string
	Revision    string
	InstallSelf func() (any, error)
	Shutdown    func()
}

type RunOptions struct {
	ListenAddress string
	OpenBrowser   bool
	Logger        *log.Logger
	Random        io.Reader
}

type requestLimiter struct {
	mu     sync.Mutex
	window time.Time
	count  int
	limit  int
}

func (l *requestLimiter) allow(now time.Time) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.window.IsZero() || now.Sub(l.window) >= time.Minute {
		l.window = now
		l.count = 0
	}
	l.count++
	return l.count <= l.limit
}

func NewHandler(options HandlerOptions) http.Handler {
	if options.InstallSelf == nil {
		options.InstallSelf = func() (any, error) { return selfinstall.InstallSelf() }
	}
	if options.SessionID == "" {
		options.SessionID = "test-session"
	}
	if options.Context == nil {
		options.Context = context.Background()
	}
	base := "/session/" + url.PathEscape(options.SessionID) + "/"
	apiBase := base + "api/"
	mutationGate := make(chan struct{}, 1)
	operations := newOperationStore()
	limiter := &requestLimiter{limit: 120}
	mux := http.NewServeMux()
	mux.HandleFunc(base, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != base {
			http.NotFound(w, r)
			return
		}
		if r.Method != http.MethodGet {
			methodNotAllowed(w)
			return
		}
		data, err := webAssets.ReadFile("web/index.html")
		if err != nil {
			writeError(w, http.StatusInternalServerError, err)
			return
		}
		page := strings.ReplaceAll(string(data), "__AGENTSTACK_TOKEN__", htmlAttribute(options.Token))
		page = strings.ReplaceAll(page, "__AGENTSTACK_VERSION__", htmlAttribute(options.Version))
		page = strings.ReplaceAll(page, "__AGENTSTACK_BASE__", htmlAttribute(base))
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = io.WriteString(w, page)
	})
	staticFS, _ := fs.Sub(webAssets, "web")
	mux.Handle(base+"assets/", http.StripPrefix(base+"assets/", http.FileServer(http.FS(staticFS))))

	authorized := func(next http.HandlerFunc) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			if !limiter.allow(time.Now()) {
				writeError(w, http.StatusTooManyRequests, fmt.Errorf("local API request rate exceeded"))
				return
			}
			if !authorizeRequest(w, r, options.Token) {
				return
			}
			next(w, r)
		}
	}
	mutation := func(next http.HandlerFunc) http.HandlerFunc {
		return authorized(func(w http.ResponseWriter, r *http.Request) {
			select {
			case mutationGate <- struct{}{}:
				defer func() { <-mutationGate }()
			default:
				writeError(w, http.StatusLocked, fmt.Errorf("another AgentStack UI operation is running"))
				return
			}
			next(w, r)
		})
	}
	startMutation := func(w http.ResponseWriter, kind string, work func(context.Context) (any, error)) {
		select {
		case mutationGate <- struct{}{}:
		default:
			writeError(w, http.StatusLocked, fmt.Errorf("another AgentStack UI operation is running"))
			return
		}
		receipt, err := operations.start(options.Context, kind, apiBase+"operations/", func(ctx context.Context) (any, error) {
			defer func() { <-mutationGate }()
			return work(ctx)
		})
		if err != nil {
			<-mutationGate
			writeError(w, http.StatusInternalServerError, err)
			return
		}
		writeJSON(w, http.StatusAccepted, receipt)
	}

	mux.HandleFunc(apiBase+"status", authorized(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			methodNotAllowed(w)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"name": "AgentStack Manager", "version": options.Version, "revision": options.Revision, "localOnly": true})
	}))
	mux.HandleFunc(apiBase+"fabric", authorized(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			methodNotAllowed(w)
			return
		}
		backend, ok := options.Backend.(fabricStatusBackend)
		if !ok {
			writeError(w, http.StatusNotImplemented, fmt.Errorf("unified fabric status is unavailable"))
			return
		}
		result, err := backend.FabricStatus(time.Now().UTC())
		if err != nil {
			writeError(w, http.StatusInternalServerError, err)
			return
		}
		writeJSON(w, http.StatusOK, result)
	}))
	mux.HandleFunc(apiBase+"catalog", authorized(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			methodNotAllowed(w)
			return
		}
		writeJSON(w, http.StatusOK, options.Backend.CatalogSnapshot())
	}))
	mux.HandleFunc(apiBase+"inventory", authorized(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			methodNotAllowed(w)
			return
		}
		result, err := options.Backend.Inventory(r.Context())
		if err != nil {
			writeError(w, http.StatusInternalServerError, err)
			return
		}
		writeJSON(w, http.StatusOK, result)
	}))
	mux.HandleFunc(apiBase+"plan", mutation(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			methodNotAllowed(w)
			return
		}
		var request planner.Request
		if !decodeJSON(w, r, &request) {
			return
		}
		result, err := options.Backend.Plan(r.Context(), request)
		if err != nil {
			writeError(w, http.StatusBadRequest, err)
			return
		}
		writeJSON(w, http.StatusOK, result)
	}))
	mux.HandleFunc(apiBase+"apply", authorized(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			methodNotAllowed(w)
			return
		}
		var request struct {
			PlanID  string `json:"planId"`
			Digest  string `json:"digest"`
			Confirm bool   `json:"confirm"`
		}
		if !decodeJSON(w, r, &request) {
			return
		}
		if !request.Confirm {
			writeError(w, http.StatusBadRequest, app.ErrConfirmationRequired)
			return
		}
		startMutation(w, "apply", func(ctx context.Context) (any, error) {
			result, err := options.Backend.ApplyPlanned(ctx, request.PlanID, request.Digest, true)
			if err != nil {
				return map[string]any{"report": result}, err
			}
			return result, nil
		})
	}))
	mux.HandleFunc(apiBase+"mcp/init", authorized(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			methodNotAllowed(w)
			return
		}
		var request app.MCPInitOptions
		if !decodeJSON(w, r, &request) {
			return
		}
		startMutation(w, "mcp-init", func(ctx context.Context) (any, error) {
			result, err := options.Backend.MCPInit(ctx, request)
			if err != nil {
				return map[string]any{"report": result}, err
			}
			return result, nil
		})
	}))
	mux.HandleFunc(apiBase+"mcp/doctor", authorized(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			methodNotAllowed(w)
			return
		}
		result, err := options.Backend.MCPDoctor(r.Context())
		if err != nil {
			writeJSON(w, http.StatusConflict, map[string]any{"error": err.Error(), "report": result})
			return
		}
		writeJSON(w, http.StatusOK, result)
	}))
	mux.HandleFunc(apiBase+"install-self", authorized(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			methodNotAllowed(w)
			return
		}
		var request struct {
			Confirm bool `json:"confirm"`
		}
		if !decodeJSON(w, r, &request) {
			return
		}
		if !request.Confirm {
			writeError(w, http.StatusBadRequest, app.ErrConfirmationRequired)
			return
		}
		startMutation(w, "install-self", func(context.Context) (any, error) {
			return options.InstallSelf()
		})
	}))
	mux.HandleFunc(apiBase+"operations/", authorized(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			methodNotAllowed(w)
			return
		}
		id := strings.TrimPrefix(r.URL.Path, apiBase+"operations/")
		if id == "" || strings.Contains(id, "/") {
			http.NotFound(w, r)
			return
		}
		operation, ok := operations.get(id)
		if !ok {
			http.NotFound(w, r)
			return
		}
		writeJSON(w, http.StatusOK, operation)
	}))
	mux.HandleFunc(apiBase+"shutdown", mutation(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			methodNotAllowed(w)
			return
		}
		if options.Shutdown == nil {
			writeError(w, http.StatusNotImplemented, fmt.Errorf("shutdown is unavailable"))
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"stopping": true})
		options.Shutdown()
	}))

	return securityHeaders(mux)
}

func Run(ctx context.Context, handlerOptions HandlerOptions, options RunOptions) error {
	if handlerOptions.Backend == nil {
		return fmt.Errorf("UI backend is nil")
	}
	reader := options.Random
	if reader == nil {
		reader = rand.Reader
	}
	if handlerOptions.Token == "" {
		value, err := randomToken(reader, 24)
		if err != nil {
			return fmt.Errorf("create UI authorization token: %w", err)
		}
		handlerOptions.Token = value
	}
	if handlerOptions.SessionID == "" {
		value, err := randomToken(reader, 18)
		if err != nil {
			return fmt.Errorf("create UI session path: %w", err)
		}
		handlerOptions.SessionID = value
	}
	handlerOptions.Context = ctx
	shutdownRequested := make(chan struct{})
	var shutdownOnce sync.Once
	externalShutdown := handlerOptions.Shutdown
	handlerOptions.Shutdown = func() {
		if externalShutdown != nil {
			externalShutdown()
		}
		shutdownOnce.Do(func() { close(shutdownRequested) })
	}
	address := options.ListenAddress
	if address == "" {
		address = "127.0.0.1:0"
	}
	host, _, err := net.SplitHostPort(address)
	if err == nil && host != "127.0.0.1" && host != "localhost" && host != "::1" {
		return fmt.Errorf("refusing non-loopback UI address %q", address)
	}
	listener, err := net.Listen("tcp", address)
	if err != nil {
		return err
	}
	server := &http.Server{Handler: NewHandler(handlerOptions), ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 30 * time.Second, WriteTimeout: 60 * time.Second, IdleTimeout: 60 * time.Second, MaxHeaderBytes: 1 << 20}
	browserURL := "http://" + listener.Addr().String() + "/session/" + url.PathEscape(handlerOptions.SessionID) + "/"
	if strings.HasPrefix(browserURL, "http://[::]") {
		browserURL = strings.Replace(browserURL, "http://[::]", "http://127.0.0.1", 1)
	}
	fmt.Fprintln(os.Stdout, "AgentStack Manager:", browserURL)
	if options.OpenBrowser {
		if err := openBrowser(browserURL); err != nil && options.Logger != nil {
			options.Logger.Printf("open browser: %v", err)
		}
	}
	errCh := make(chan error, 1)
	go func() { errCh <- server.Serve(listener) }()
	select {
	case <-ctx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = server.Shutdown(shutdownCtx)
		return ctx.Err()
	case <-shutdownRequested:
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		return server.Shutdown(shutdownCtx)
	case err := <-errCh:
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return err
	}
}

func authorizeRequest(w http.ResponseWriter, r *http.Request, token string) bool {
	if origin := r.Header.Get("Origin"); origin != "" {
		parsed, err := url.Parse(origin)
		if err != nil || !strings.EqualFold(parsed.Host, r.Host) {
			writeError(w, http.StatusForbidden, fmt.Errorf("cross-origin request denied"))
			return false
		}
	}
	if token == "" || r.Header.Get("X-AgentStack-Token") != token {
		writeError(w, http.StatusForbidden, fmt.Errorf("invalid AgentStack session token"))
		return false
	}
	return true
}

func decodeJSON(w http.ResponseWriter, r *http.Request, destination any) bool {
	if !strings.HasPrefix(strings.ToLower(r.Header.Get("Content-Type")), "application/json") {
		writeError(w, http.StatusUnsupportedMediaType, fmt.Errorf("Content-Type must be application/json"))
		return false
	}
	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(destination); err != nil {
		writeError(w, http.StatusBadRequest, fmt.Errorf("decode request: %w", err))
		return false
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		writeError(w, http.StatusBadRequest, fmt.Errorf("request must contain one JSON value"))
		return false
	}
	return true
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}
func writeError(w http.ResponseWriter, status int, err error) {
	writeJSON(w, status, map[string]any{"error": err.Error()})
}
func methodNotAllowed(w http.ResponseWriter) {
	writeError(w, http.StatusMethodNotAllowed, fmt.Errorf("method not allowed"))
}

func securityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("X-Frame-Options", "DENY")
		w.Header().Set("Referrer-Policy", "no-referrer")
		w.Header().Set("Content-Security-Policy", "default-src 'self'; script-src 'self'; style-src 'self'; img-src 'self' data:; connect-src 'self'; frame-ancestors 'none'; base-uri 'none'; form-action 'self'")
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("Cross-Origin-Opener-Policy", "same-origin")
		w.Header().Set("Cross-Origin-Resource-Policy", "same-origin")
		next.ServeHTTP(w, r)
	})
}

func randomToken(reader io.Reader, byteCount int) (string, error) {
	if reader == nil {
		return "", fmt.Errorf("random source is nil")
	}
	if byteCount < 16 {
		byteCount = 16
	}
	data := make([]byte, byteCount)
	if _, err := io.ReadFull(reader, data); err != nil {
		return "", err
	}
	return hex.EncodeToString(data), nil
}

func htmlAttribute(value string) string {
	replacer := strings.NewReplacer("&", "&amp;", `"`, "&quot;", "<", "&lt;", ">", "&gt;")
	return replacer.Replace(value)
}

func openBrowser(target string) error {
	switch runtime.GOOS {
	case "windows":
		var candidates []string
		for _, envVar := range []string{"ProgramFiles(x86)", "ProgramFiles", "LocalAppData"} {
			base := os.Getenv(envVar)
			if base != "" {
				candidates = append(candidates,
					filepath.Join(base, "Microsoft", "Edge", "Application", "msedge.exe"),
					filepath.Join(base, "Google", "Chrome", "Application", "chrome.exe"),
				)
			}
		}
		for _, exe := range candidates {
			if _, err := os.Stat(exe); err == nil {
				if err := exec.Command(exe, "--app="+target).Start(); err == nil {
					return nil
				}
			}
		}
		return exec.Command("rundll32", "url.dll,FileProtocolHandler", target).Start()
	case "darwin":
		return exec.Command("open", target).Start()
	default:
		return exec.Command("xdg-open", target).Start()
	}
}
