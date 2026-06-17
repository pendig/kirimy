package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestNormalizeFlagName(t *testing.T) {
	tests := []struct {
		in, want string
	}{
		{"read_only", "read-only"},
		{"--read-only", "read-only"},
		{"  lock_wait  ", "lock-wait"},
		{"json", "json"},
		{"--timeout_secs", "timeout-secs"},
	}
	for _, tt := range tests {
		if got := normalizeFlagName(tt.in); got != tt.want {
			t.Errorf("normalizeFlagName(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

func TestParseBoolValue(t *testing.T) {
	trueVals := []string{"1", "true", "True", "TRUE", "t", "yes", "on", "y"}
	for _, v := range trueVals {
		b, err := parseBoolValue(v)
		if err != nil || !b {
			t.Errorf("parseBoolValue(%q) = %v, %v; want true, nil", v, b, err)
		}
	}
	falseVals := []string{"0", "false", "False", "f", "no", "off", "n"}
	for _, v := range falseVals {
		b, err := parseBoolValue(v)
		if err != nil || b {
			t.Errorf("parseBoolValue(%q) = %v, %v; want false, nil", v, b, err)
		}
	}
	if _, err := parseBoolValue("maybe"); err == nil {
		t.Error("parseBoolValue(maybe) should error")
	}
}

func TestSplitPath(t *testing.T) {
	tests := []struct {
		in   string
		want []string
	}{
		{"messages/search", []string{"messages", "search"}},
		{"/api/v1/send/text", []string{"api", "v1", "send", "text"}},
		{"///", nil},
		{"auth", []string{"auth"}},
		{"send/text/", []string{"send", "text"}},
	}
	for _, tt := range tests {
		got := splitPath(tt.in)
		if len(got) != len(tt.want) {
			t.Errorf("splitPath(%q) = %v, want %v", tt.in, got, tt.want)
			continue
		}
		for i := range got {
			if got[i] != tt.want[i] {
				t.Errorf("splitPath(%q)[%d] = %q, want %q", tt.in, i, got[i], tt.want[i])
			}
		}
	}
}

func TestFirstValue(t *testing.T) {
	params := map[string][]string{
		"limit":  {"10", "20"},
		"empty":  {"", "  ", "real"},
		"blank":  {"", ""},
		"stored": {"value"},
	}
	if got := firstValue(params, "limit"); got != "10" {
		t.Errorf("firstValue(limit) = %q, want 10", got)
	}
	if got := firstValue(params, "empty"); got != "real" {
		t.Errorf("firstValue(empty) = %q, want real", got)
	}
	if got := firstValue(params, "blank"); got != "" {
		t.Errorf("firstValue(blank) = %q, want empty", got)
	}
	if got := firstValue(params, "missing"); got != "" {
		t.Errorf("firstValue(missing) = %q, want empty", got)
	}
	if got := firstValue(params, "stored"); got != "value" {
		t.Errorf("firstValue(stored) = %q, want value", got)
	}
}

func TestHasAny(t *testing.T) {
	params := map[string][]string{"follow": {"true"}}
	if !hasAny(params, "follow", "once") {
		t.Error("hasAny should find follow")
	}
	if hasAny(params, "once", "limit") {
		t.Error("hasAny should not find once or limit")
	}
}

func TestFlagArgsFromParams(t *testing.T) {
	params := map[string][]string{
		"limit":     {"10"},
		"read-only": {"true"},
		"store":     {"/tmp/store"},
		"json":      {"1"},
		"verbose":   {"false"},
		"tag":       {"a", "b"},
	}
	got := flagArgsFromParams(params)
	joined := strings.Join(got, " ")
	if !strings.Contains(joined, "--limit 10") {
		t.Errorf("missing --limit 10 in %q", joined)
	}
	if !strings.Contains(joined, "--tag a") || !strings.Contains(joined, "--tag b") {
		t.Errorf("missing --tag flags in %q", joined)
	}
	if strings.Contains(joined, "--store") {
		t.Errorf("reserved key store should be skipped: %q", joined)
	}
	if strings.Contains(joined, "--json") {
		t.Errorf("reserved key json should be skipped: %q", joined)
	}
	if strings.Contains(joined, "--verbose") {
		t.Errorf("false bool should not produce flag: %q", joined)
	}
}

func TestValueToString(t *testing.T) {
	tests := []struct {
		in   any
		want string
	}{
		{nil, ""},
		{"hello", "hello"},
		{true, "true"},
		{false, "false"},
		{float64(42), "42"},
		{float64(3.14), "3.14"},
		{json.Number("99"), "99"},
	}
	for _, tt := range tests {
		got, err := valueToString(tt.in)
		if err != nil {
			t.Errorf("valueToString(%v) error: %v", tt.in, err)
			continue
		}
		if got != tt.want {
			t.Errorf("valueToString(%v) = %q, want %q", tt.in, got, tt.want)
		}
	}
	if _, err := valueToString(map[string]any{}); err == nil {
		t.Error("valueToString(map) should error")
	}
}

func TestNormalizeStringArray(t *testing.T) {
	got, err := normalizeStringArray([]any{"a", "b", "c"})
	if err != nil || len(got) != 3 || got[0] != "a" {
		t.Errorf("array: got %v, err %v", got, err)
	}
	got, err = normalizeStringArray("single")
	if err != nil || len(got) != 1 || got[0] != "single" {
		t.Errorf("string: got %v, err %v", got, err)
	}
	if _, err := normalizeStringArray(42); err == nil {
		t.Error("int should error")
	}
}

func TestParseEmbeddedJSON(t *testing.T) {
	got := parseEmbeddedJSON(`{"key":"val"}`)
	if got == nil || got["key"] != "val" {
		t.Errorf("parseEmbeddedJSON object = %v", got)
	}
	if parseEmbeddedJSON("plain text") != nil {
		t.Error("plain text should return nil")
	}
	if parseEmbeddedJSON("") != nil {
		t.Error("empty should return nil")
	}
	if parseEmbeddedJSON("{invalid") != nil {
		t.Error("invalid JSON should return nil")
	}
}

func TestCommandCatalogCompleteness(t *testing.T) {
	expected := []string{
		"accounts", "auth", "calls", "channels", "chats", "contacts",
		"doctor", "docs", "groups", "history", "media", "messages",
		"poll", "polls", "presence", "profile", "send", "store",
		"sync", "version",
	}
	for _, cmd := range expected {
		if _, ok := commandCatalog[cmd]; !ok {
			t.Errorf("commandCatalog missing %q", cmd)
		}
	}
}

func TestCommandCatalogGroupsHasSubcommands(t *testing.T) {
	subs := commandCatalog["groups"]
	if len(subs) == 0 {
		t.Fatal("groups should have subcommands")
	}
	expected := []string{"create", "list", "refresh", "info", "rename", "participants", "requests", "invite", "join", "leave", "prune"}
	subSet := make(map[string]bool)
	for _, s := range subs {
		subSet[s] = true
	}
	for _, want := range expected {
		if !subSet[want] {
			t.Errorf("groups missing subcommand %q", want)
		}
	}
}

func TestCommandCatalogContactsHasSubcommands(t *testing.T) {
	subs := commandCatalog["contacts"]
	expected := []string{"search", "show", "refresh", "import-system", "alias", "tags"}
	subSet := make(map[string]bool)
	for _, s := range subs {
		subSet[s] = true
	}
	for _, want := range expected {
		if !subSet[want] {
			t.Errorf("contacts missing subcommand %q", want)
		}
	}
}

func TestDeriveDirectCmds(t *testing.T) {
	catalog := map[string][]string{
		"auth":     {"", "status", "logout"},
		"messages": {"list", "search"},
		"version":  {""},
		"doctor":   {""},
	}
	direct := deriveDirectCmds(catalog)
	if !direct["auth"] {
		t.Error("auth should be direct")
	}
	if !direct["version"] {
		t.Error("version should be direct")
	}
	if !direct["doctor"] {
		t.Error("doctor should be direct")
	}
	if direct["messages"] {
		t.Error("messages should not be direct")
	}
}

func newTestConfig() daemonConfig {
	catalog := map[string][]string{
		"auth":     {"", "status", "logout"},
		"messages": {"list", "search", "show"},
		"contacts": {"search", "show"},
		"send":     {"text", "file"},
		"sync":     {""},
		"version":  {""},
		"doctor":   {""},
		"groups":   {"list", "info", "participants"},
	}
	return daemonConfig{
		binaryPath:  "/bin/true",
		defaultJSON: true,
		catalog:     catalog,
		directCmds:  deriveDirectCmds(catalog),
		defaultCmds: topLevelWithDefault,
	}
}

func decodeResponse(t *testing.T, rec *httptest.ResponseRecorder) cliResponse {
	t.Helper()
	var resp cliResponse
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v\nbody: %s", err, rec.Body.String())
	}
	return resp
}

func TestHealthzEndpoint(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			writeJSON(w, http.StatusMethodNotAllowed, cliResponse{Status: "error", Message: "method not allowed"})
			return
		}
		writeJSON(w, http.StatusOK, cliResponse{Status: "ok", Message: "daemon is alive"})
	})

	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	resp := decodeResponse(t, rec)
	if resp.Status != "ok" {
		t.Errorf("status = %q, want ok", resp.Status)
	}
}

func TestHealthzRejectsPost(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			writeJSON(w, http.StatusMethodNotAllowed, cliResponse{Status: "error", Message: "method not allowed"})
			return
		}
		writeJSON(w, http.StatusOK, cliResponse{Status: "ok"})
	})

	req := httptest.NewRequest(http.MethodPost, "/healthz", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("status = %d, want 405", rec.Code)
	}
}

func TestReadyzEndpoint(t *testing.T) {
	cfg := newTestConfig()
	mux := http.NewServeMux()
	mux.HandleFunc("/readyz", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			writeJSON(w, http.StatusMethodNotAllowed, cliResponse{Status: "error", Message: "method not allowed"})
			return
		}
		status := "ready"
		if cfg.defaultRO {
			status = "ready (daemon read-only)"
		}
		writeJSON(w, http.StatusOK, cliResponse{Status: "ok", Message: status, Data: map[string]any{"read_only": cfg.defaultRO}})
	})

	req := httptest.NewRequest(http.MethodGet, "/readyz", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	resp := decodeResponse(t, rec)
	if resp.Message != "ready" {
		t.Errorf("message = %q, want ready", resp.Message)
	}
}

func TestReadyzReadOnly(t *testing.T) {
	cfg := newTestConfig()
	cfg.defaultRO = true
	mux := http.NewServeMux()
	mux.HandleFunc("/readyz", func(w http.ResponseWriter, r *http.Request) {
		status := "ready"
		if cfg.defaultRO {
			status = "ready (daemon read-only)"
		}
		writeJSON(w, http.StatusOK, cliResponse{Status: "ok", Message: status, Data: map[string]any{"read_only": cfg.defaultRO}})
	})

	req := httptest.NewRequest(http.MethodGet, "/readyz", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	resp := decodeResponse(t, rec)
	if resp.Message != "ready (daemon read-only)" {
		t.Errorf("message = %q, want ready (daemon read-only)", resp.Message)
	}
}

func TestCatalogEndpoint(t *testing.T) {
	cfg := newTestConfig()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/commands", nil)
	rec := httptest.NewRecorder()
	cfg.handleCatalog(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	resp := decodeResponse(t, rec)
	data, ok := resp.Data.(map[string]any)
	if !ok {
		t.Fatal("data should be an object")
	}
	commands, ok := data["commands"].(map[string]any)
	if !ok {
		t.Fatal("commands should be an object")
	}
	if _, ok := commands["messages"]; !ok {
		t.Error("catalog should include messages")
	}
	if _, ok := commands["groups"]; !ok {
		t.Error("catalog should include groups")
	}
}

func TestCatalogRejectsPost(t *testing.T) {
	cfg := newTestConfig()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/commands", nil)
	rec := httptest.NewRecorder()
	cfg.handleCatalog(rec, req)

	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("status = %d, want 405", rec.Code)
	}
}

func TestRequireTokenEmpty(t *testing.T) {
	cfg := newTestConfig()
	called := false
	handler := cfg.requireToken("", func(w http.ResponseWriter, r *http.Request) {
		called = true
		writeJSON(w, http.StatusOK, cliResponse{Status: "ok"})
	})

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	rec := httptest.NewRecorder()
	handler(rec, req)

	if !called {
		t.Error("handler should be called when no token is configured")
	}
}

func TestRequireTokenValid(t *testing.T) {
	cfg := newTestConfig()
	called := false
	handler := cfg.requireToken("secret123", func(w http.ResponseWriter, r *http.Request) {
		called = true
		writeJSON(w, http.StatusOK, cliResponse{Status: "ok"})
	})

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	req.Header.Set("Authorization", "Bearer secret123")
	rec := httptest.NewRecorder()
	handler(rec, req)

	if !called {
		t.Error("handler should be called with valid token")
	}
}

func TestRequireTokenInvalid(t *testing.T) {
	cfg := newTestConfig()
	handler := cfg.requireToken("secret123", func(w http.ResponseWriter, r *http.Request) {
		t.Error("handler should not be called with invalid token")
	})

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	req.Header.Set("Authorization", "Bearer wrong")
	rec := httptest.NewRecorder()
	handler(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", rec.Code)
	}
}

func TestRequireTokenMissing(t *testing.T) {
	cfg := newTestConfig()
	handler := cfg.requireToken("secret123", func(w http.ResponseWriter, r *http.Request) {
		t.Error("handler should not be called without token")
	})

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	rec := httptest.NewRecorder()
	handler(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", rec.Code)
	}
}

func TestHandleExecRejectsGet(t *testing.T) {
	cfg := newTestConfig()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/exec", nil)
	rec := httptest.NewRecorder()
	cfg.handleExec(rec, req)

	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("status = %d, want 405", rec.Code)
	}
}

func TestHandleExecEmptyBody(t *testing.T) {
	cfg := newTestConfig()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/exec", strings.NewReader(""))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	cfg.handleExec(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", rec.Code)
	}
}

func TestHandleExecMissingCommand(t *testing.T) {
	cfg := newTestConfig()
	body := `{"command":[]}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/exec", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	cfg.handleExec(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", rec.Code)
	}
	resp := decodeResponse(t, rec)
	if !strings.Contains(resp.Message, "command is required") {
		t.Errorf("message = %q, want command is required", resp.Message)
	}
}

func TestHandleCommandRouteMissingPath(t *testing.T) {
	cfg := newTestConfig()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/", nil)
	rec := httptest.NewRecorder()
	cfg.handleCommandRoute(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", rec.Code)
	}
}

func TestHandleCommandRouteRejectsDelete(t *testing.T) {
	cfg := newTestConfig()
	req := httptest.NewRequest(http.MethodDelete, "/api/v1/messages", nil)
	rec := httptest.NewRecorder()
	cfg.handleCommandRoute(rec, req)

	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("status = %d, want 405", rec.Code)
	}
}

func TestHandleCommandRouteNonDirectRequiresSubcommand(t *testing.T) {
	cfg := newTestConfig()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/media", nil)
	rec := httptest.NewRecorder()
	cfg.handleCommandRoute(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400 (media needs subcommand)", rec.Code)
	}
}

func TestRunCLICommandUsesMockBinary(t *testing.T) {
	dir := t.TempDir()
	script := filepath.Join(dir, "mock-wacli")
	if err := os.WriteFile(script, []byte("#!/bin/sh\necho '{\"ok\":true}'\n"), 0755); err != nil {
		t.Fatal(err)
	}

	cfg := newTestConfig()
	cfg.binaryPath = script

	result, status, err := cfg.runCLICommand(
		t.Context(),
		[]string{"version"},
		nil,
		map[string][]string{},
		nil,
	)
	if err != nil {
		t.Fatalf("runCLICommand: %v", err)
	}
	if status != http.StatusOK {
		t.Fatalf("status = %d, want 200", status)
	}
	if !strings.Contains(result.Stdout, "ok") {
		t.Errorf("stdout = %q, want ok", result.Stdout)
	}
}

func TestRunCLICommandEmptyCommand(t *testing.T) {
	cfg := newTestConfig()
	_, status, err := cfg.runCLICommand(t.Context(), nil, nil, nil, nil)
	if err == nil {
		t.Fatal("expected error for empty command")
	}
	if status != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", status)
	}
}

func TestRunCLICommandReadOnlyEnforced(t *testing.T) {
	cfg := newTestConfig()
	cfg.defaultRO = true

	_, status, err := cfg.runCLICommand(
		t.Context(),
		[]string{"send", "text"},
		nil,
		map[string][]string{"read-only": {"false"}},
		nil,
	)
	if err == nil {
		t.Fatal("expected error when read-only is enforced but client requests writable")
	}
	if status != http.StatusForbidden {
		t.Fatalf("status = %d, want 403", status)
	}
}

func TestRunCLICommandFailingBinary(t *testing.T) {
	dir := t.TempDir()
	script := filepath.Join(dir, "mock-wacli")
	if err := os.WriteFile(script, []byte("#!/bin/sh\necho 'bad thing' >&2\nexit 1\n"), 0755); err != nil {
		t.Fatal(err)
	}

	cfg := newTestConfig()
	cfg.binaryPath = script

	result, status, err := cfg.runCLICommand(
		t.Context(),
		[]string{"messages", "list"},
		nil,
		map[string][]string{},
		nil,
	)
	if err == nil {
		t.Fatal("expected error for failing command")
	}
	if status != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", status)
	}
	if result.ExitCode != 1 {
		t.Errorf("exit_code = %d, want 1", result.ExitCode)
	}
	if !strings.Contains(result.Stderr, "bad thing") {
		t.Errorf("stderr = %q, want 'bad thing'", result.Stderr)
	}
}

func TestRunCLICommandPassesFlags(t *testing.T) {
	dir := t.TempDir()
	script := filepath.Join(dir, "mock-wacli")
	if err := os.WriteFile(script, []byte("#!/bin/sh\necho \"$@\"\n"), 0755); err != nil {
		t.Fatal(err)
	}

	cfg := newTestConfig()
	cfg.binaryPath = script
	cfg.defaultStore = "/tmp/test-store"

	result, _, err := cfg.runCLICommand(
		t.Context(),
		[]string{"messages", "list"},
		nil,
		map[string][]string{"limit": {"50"}},
		nil,
	)
	if err != nil {
		t.Fatalf("runCLICommand: %v", err)
	}
	if !strings.Contains(result.Stdout, "--store /tmp/test-store") {
		t.Errorf("stdout should contain store flag: %q", result.Stdout)
	}
	if !strings.Contains(result.Stdout, "--json") {
		t.Errorf("stdout should contain --json: %q", result.Stdout)
	}
	if !strings.Contains(result.Stdout, "--limit 50") {
		t.Errorf("stdout should contain --limit 50: %q", result.Stdout)
	}
}

func TestRunCLICommandAccountFlag(t *testing.T) {
	dir := t.TempDir()
	script := filepath.Join(dir, "mock-wacli")
	if err := os.WriteFile(script, []byte("#!/bin/sh\necho \"$@\"\n"), 0755); err != nil {
		t.Fatal(err)
	}

	cfg := newTestConfig()
	cfg.binaryPath = script
	cfg.defaultAccount = "work"

	result, _, err := cfg.runCLICommand(
		t.Context(),
		[]string{"version"},
		nil,
		map[string][]string{},
		nil,
	)
	if err != nil {
		t.Fatalf("runCLICommand: %v", err)
	}
	if !strings.Contains(result.Stdout, "--account work") {
		t.Errorf("stdout should contain --account work: %q", result.Stdout)
	}
}

func TestHandleCommandRouteWithMockBinary(t *testing.T) {
	dir := t.TempDir()
	script := filepath.Join(dir, "mock-wacli")
	if err := os.WriteFile(script, []byte("#!/bin/sh\necho '{\"messages\":[]}'\n"), 0755); err != nil {
		t.Fatal(err)
	}

	cfg := newTestConfig()
	cfg.binaryPath = script

	req := httptest.NewRequest(http.MethodGet, "/api/v1/messages/list", nil)
	rec := httptest.NewRecorder()
	cfg.handleCommandRoute(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200\nbody: %s", rec.Code, rec.Body.String())
	}
	resp := decodeResponse(t, rec)
	if resp.Status != "ok" {
		t.Errorf("status = %q, want ok", resp.Status)
	}
}

func TestHandleCommandRouteDefaultSubcommand(t *testing.T) {
	dir := t.TempDir()
	script := filepath.Join(dir, "mock-wacli")
	if err := os.WriteFile(script, []byte("#!/bin/sh\necho \"$@\"\n"), 0755); err != nil {
		t.Fatal(err)
	}

	cfg := newTestConfig()
	cfg.binaryPath = script

	req := httptest.NewRequest(http.MethodGet, "/api/v1/messages", nil)
	rec := httptest.NewRecorder()
	cfg.handleCommandRoute(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200\nbody: %s", rec.Code, rec.Body.String())
	}
	resp := decodeResponse(t, rec)
	data, ok := resp.Data.(map[string]any)
	if !ok {
		t.Fatal("data should be object")
	}
	stdout, _ := data["stdout"].(string)
	if !strings.Contains(stdout, "messages list") {
		t.Errorf("should default to messages list, got: %q", stdout)
	}
}

func TestHandleCommandRouteSyncAutoOnce(t *testing.T) {
	dir := t.TempDir()
	script := filepath.Join(dir, "mock-wacli")
	if err := os.WriteFile(script, []byte("#!/bin/sh\necho \"$@\"\n"), 0755); err != nil {
		t.Fatal(err)
	}

	cfg := newTestConfig()
	cfg.binaryPath = script

	req := httptest.NewRequest(http.MethodGet, "/api/v1/sync", nil)
	rec := httptest.NewRecorder()
	cfg.handleCommandRoute(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	resp := decodeResponse(t, rec)
	data, _ := resp.Data.(map[string]any)
	stdout, _ := data["stdout"].(string)
	if !strings.Contains(stdout, "--once") {
		t.Errorf("sync without follow/once should add --once, got: %q", stdout)
	}
}

func TestHandleCommandRouteSearchPositional(t *testing.T) {
	dir := t.TempDir()
	script := filepath.Join(dir, "mock-wacli")
	if err := os.WriteFile(script, []byte("#!/bin/sh\necho \"$@\"\n"), 0755); err != nil {
		t.Fatal(err)
	}

	cfg := newTestConfig()
	cfg.binaryPath = script

	req := httptest.NewRequest(http.MethodGet, "/api/v1/messages/search?query=hello", nil)
	rec := httptest.NewRecorder()
	cfg.handleCommandRoute(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	resp := decodeResponse(t, rec)
	data, _ := resp.Data.(map[string]any)
	stdout, _ := data["stdout"].(string)
	if !strings.Contains(stdout, "hello") {
		t.Errorf("search query should be positional arg, got: %q", stdout)
	}
}

func TestHandleExecWithMockBinary(t *testing.T) {
	dir := t.TempDir()
	script := filepath.Join(dir, "mock-wacli")
	if err := os.WriteFile(script, []byte("#!/bin/sh\necho '{\"version\":\"0.11.0\"}'\n"), 0755); err != nil {
		t.Fatal(err)
	}

	cfg := newTestConfig()
	cfg.binaryPath = script

	body := `{"command":["version"],"json":true}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/exec", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	cfg.handleExec(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200\nbody: %s", rec.Code, rec.Body.String())
	}
}

func TestHandleCommandRouteSendRequiresKind(t *testing.T) {
	cfg := newTestConfig()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/send", nil)
	rec := httptest.NewRecorder()
	cfg.handleCommandRoute(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", rec.Code)
	}
	resp := decodeResponse(t, rec)
	if !strings.Contains(resp.Message, "send requires kind") {
		t.Errorf("message = %q", resp.Message)
	}
}

func TestWriteJSONContentType(t *testing.T) {
	rec := httptest.NewRecorder()
	writeJSON(rec, http.StatusOK, cliResponse{Status: "ok"})

	ct := rec.Header().Get("Content-Type")
	if ct != "application/json" {
		t.Errorf("Content-Type = %q, want application/json", ct)
	}
}

func TestCollectRequestParamsQuery(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/api/v1/messages?limit=10&chat_jid=123", nil)
	params, positional, err := collectRequestParams(req)
	if err != nil {
		t.Fatalf("collectRequestParams: %v", err)
	}
	if positional != nil {
		t.Errorf("positional should be nil for GET, got %v", positional)
	}
	if firstValue(params, "limit") != "10" {
		t.Errorf("limit = %q, want 10", firstValue(params, "limit"))
	}
}

func TestCollectRequestParamsJSONBody(t *testing.T) {
	body := `{"limit": "20", "args": ["hello"]}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/messages/search", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.ContentLength = int64(len(body))

	params, positional, err := collectRequestParams(req)
	if err != nil {
		t.Fatalf("collectRequestParams: %v", err)
	}
	if firstValue(params, "limit") != "20" {
		t.Errorf("limit = %q, want 20", firstValue(params, "limit"))
	}
	if len(positional) != 1 || positional[0] != "hello" {
		t.Errorf("positional = %v, want [hello]", positional)
	}
}

func TestCollectRequestParamsRejectsNonJSON(t *testing.T) {
	req := httptest.NewRequest(http.MethodPost, "/api/v1/test", strings.NewReader("plain text"))
	req.Header.Set("Content-Type", "text/plain")
	req.ContentLength = int64(len("plain text"))

	_, _, err := collectRequestParams(req)
	if err == nil {
		t.Error("expected error for non-JSON content type")
	}
}
