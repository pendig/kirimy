package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/exec"
	"sort"
	"strconv"
	"strings"
	"time"
)

type commandDefaults struct {
	defaultSubcommand string
	defaultFlags      []string
}
type cliResponse struct {
	Status   string `json:"status"`
	Message  string `json:"message,omitempty"`
	Error    string `json:"error,omitempty"`
	Duration string `json:"duration"`
	Data     any    `json:"data,omitempty"`
}
type commandResult struct {
	Command  []string `json:"command"`
	Stdout   string   `json:"stdout,omitempty"`
	Stderr   string   `json:"stderr,omitempty"`
	ExitCode int      `json:"exit_code"`
}
type daemonConfig struct {
	listenAddr     string
	binaryPath     string
	defaultStore   string
	defaultAccount string
	defaultJSON    bool
	defaultFull    bool
	defaultEvents  bool
	defaultRO      bool
	reqTimeout     time.Duration
}

const (
	defaultAPIRoot = "/api/v1"
)

var topLevelWithDefault = map[string]commandDefaults{
	"messages": {defaultSubcommand: "list"},
	"calls":    {defaultSubcommand: "list"},
	"chats":    {defaultSubcommand: "list"},
	"contacts": {defaultSubcommand: "search"},
	"channels": {defaultSubcommand: "list"},
	"groups":   {defaultSubcommand: "list"},
	"accounts": {defaultSubcommand: "list"},
	"history":  {defaultSubcommand: "coverage"},
	"polls":    {defaultSubcommand: "list"},
	"store":    {defaultSubcommand: "stats"},
	"sync":     {defaultFlags: []string{"--once"}},
}

var topLevelDirect = map[string]bool{
	"auth":    true,
	"doctor":  true,
	"docs":    true,
	"sync":    true,
	"version": true,
}

var commandCatalog = map[string][]string{
	"accounts": {"list", "add", "use", "show", "remove"},
	"auth":     {"", "status", "logout"},
	"calls":    {"list"},
	"channels": {"list", "info", "join", "leave"},
	"chats":    {"list", "show", "archive", "unarchive", "pin", "unpin", "mute", "unmute", "mark-read", "mark-unread", "cleanup"},
	"contacts": {"search", "show", "refresh", "import-system", "alias", "tags"},
	"doctor":   {""},
	"docs":     {""},
	"history":  {"coverage", "fill", "backfill"},
	"media":    {"download"},
	"messages": {"list", "search", "starred", "show", "context", "export", "delete", "revoke", "edit", "forward"},
	"polls":    {"show", "vote", "list"},
	"presence": {"typing", "paused"},
	"profile":  {"set-picture", "remove-picture", "picture", "set-about", "get-about", "set-name", "business", "set-business"},
	"send":     {"text", "file", "sticker", "voice", "react", "poll", "status", "select"},
	"store":    {"stats", "cleanup"},
	"sync":     {""},
	"version":  {""},
}

var reservedKeys = map[string]struct{}{
	"store":     {},
	"account":   {},
	"read_only": {},
	"json":      {},
	"full":      {},
	"events":    {},
	"timeout":   {},
	"lock_wait": {},
	"command":   {},
	"args":      {},
}

func main() {
	listen := flag.String("listen", ":8080", "HTTP listen address")
	binary := flag.String("wacli-binary", "wacli", "Path to wacli binary")
	storeDir := flag.String("store", "", "Default --store override for child commands")
	account := flag.String("account", "", "Default --account override for child commands")
	readOnly := flag.Bool("read-only", false, "Pass --read-only to child commands")
	asJSON := flag.Bool("json", true, "Pass --json to child commands")
	fullOutput := flag.Bool("full", false, "Pass --full to child commands")
	events := flag.Bool("events", false, "Pass --events to child commands")
	cmdTimeout := flag.Duration("command-timeout", 0, "Per-request command timeout (0 = no timeout)")
	apiToken := flag.String("api-token", "", "Optional Bearer token for API auth")
	flag.Parse()

	cfg := daemonConfig{
		listenAddr:     *listen,
		binaryPath:     *binary,
		defaultStore:   *storeDir,
		defaultAccount: *account,
		defaultJSON:    *asJSON,
		defaultFull:    *fullOutput,
		defaultEvents:  *events,
		defaultRO:      *readOnly,
		reqTimeout:     *cmdTimeout,
	}

	if strings.TrimSpace(cfg.binaryPath) == "" {
		cfg.binaryPath = "wacli"
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			writeJSON(w, http.StatusMethodNotAllowed, cliResponse{Status: "error", Message: "method not allowed"})
			return
		}
		writeJSON(w, http.StatusOK, cliResponse{Status: "ok", Message: "daemon is alive"})
	})

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

	mux.HandleFunc(defaultAPIRoot+"/commands", cfg.handleCatalog)
	mux.HandleFunc(defaultAPIRoot+"/exec", cfg.requireToken(*apiToken, cfg.handleExec))
	mux.HandleFunc(defaultAPIRoot+"/", cfg.requireToken(*apiToken, cfg.handleCommandRoute))

	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/" {
			writeJSON(w, http.StatusOK, cliResponse{Status: "ok", Message: "kirimy daemon", Data: map[string]any{"api": defaultAPIRoot}})
			return
		}
		writeJSON(w, http.StatusNotFound, cliResponse{Status: "error", Message: "not found"})
	})

	server := &http.Server{
		Addr:         cfg.listenAddr,
		Handler:      mux,
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 120 * time.Second,
	}

	log.Printf("kirimy daemon listening on %s", cfg.listenAddr)
	log.Printf("kirimy binary: %s", cfg.binaryPath)
	if err := server.ListenAndServe(); err != nil {
		log.Fatalf("daemon stopped: %v", err)
	}
}

func (cfg daemonConfig) requireToken(expected string, next http.HandlerFunc) http.HandlerFunc {
	if strings.TrimSpace(expected) == "" {
		return next
	}
	return func(w http.ResponseWriter, r *http.Request) {
		token := strings.TrimSpace(strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer "))
		if token == "" || token != expected {
			writeJSON(w, http.StatusUnauthorized, cliResponse{Status: "error", Message: "unauthorized"})
			return
		}
		next(w, r)
	}
}

func (cfg daemonConfig) handleCatalog(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, cliResponse{Status: "error", Message: "method not allowed"})
		return
	}
	commands := make(map[string][]string, len(commandCatalog))
	for cmd, sub := range commandCatalog {
		commands[cmd] = append([]string(nil), sub...)
	}
	writeJSON(w, http.StatusOK, cliResponse{Status: "ok", Data: map[string]any{"commands": commands}})
}

func (cfg daemonConfig) handleExec(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, cliResponse{Status: "error", Message: "method not allowed"})
		return
	}

	req := struct {
		Command  []string       `json:"command"`
		Args     []string       `json:"args"`
		Store    string         `json:"store"`
		Account  string         `json:"account"`
		ReadOnly *bool          `json:"read_only"`
		JSON     *bool          `json:"json"`
		Full     *bool          `json:"full"`
		Events   *bool          `json:"events"`
		Timeout  string         `json:"timeout"`
		LockWait string         `json:"lock_wait"`
		Async    bool           `json:"async"`
		RawFlags map[string]any `json:"flags"`
	}{}

	if err := decodeJSONBody(r, &req); err != nil {
		writeJSON(w, http.StatusBadRequest, cliResponse{Status: "error", Message: "invalid request body", Error: err.Error()})
		return
	}

	if len(req.Command) == 0 {
		writeJSON(w, http.StatusBadRequest, cliResponse{Status: "error", Message: "command is required"})
		return
	}

	params := map[string][]string{}
	for key, val := range req.RawFlags {
		if err := appendValueFromInterface(params, key, val); err != nil {
			writeJSON(w, http.StatusBadRequest, cliResponse{Status: "error", Message: "invalid flag value", Error: err.Error()})
			return
		}
	}

	runtime := map[string]string{}
	if req.Store != "" {
		runtime["store"] = req.Store
	}
	if req.Account != "" {
		runtime["account"] = req.Account
	}
	if req.ReadOnly != nil {
		if *req.ReadOnly {
			runtime["read_only"] = "1"
		} else {
			runtime["read_only"] = "0"
		}
	}
	if req.JSON != nil {
		if *req.JSON {
			runtime["json"] = "1"
		} else {
			runtime["json"] = "0"
		}
	}
	if req.Full != nil {
		if *req.Full {
			runtime["full"] = "1"
		} else {
			runtime["full"] = "0"
		}
	}
	if req.Events != nil {
		if *req.Events {
			runtime["events"] = "1"
		} else {
			runtime["events"] = "0"
		}
	}
	if req.Timeout != "" {
		runtime["timeout"] = req.Timeout
	}
	if req.LockWait != "" {
		runtime["lock_wait"] = req.LockWait
	}

	for key, val := range runtime {
		params[key] = append(params[key], val)
	}

	result, statusCode, err := cfg.runCLICommand(r.Context(), req.Command, req.Args, params, paramsToStrings(params))
	if err != nil {
		writeJSON(w, statusCode, cliResponse{Status: "error", Message: "command failed", Error: err.Error(), Data: result})
		return
	}
	writeJSON(w, statusCode, cliResponse{Status: "ok", Data: result})
}

func paramsToStrings(params map[string][]string) []string {
	args := make([]string, 0)
	for k, values := range params {
		if _, skip := reservedKeys[k]; skip {
			continue
		}
		for _, value := range values {
			args = append(args, normalizeFlagName(k), value)
		}
	}
	return args
}

func (cfg daemonConfig) handleCommandRoute(w http.ResponseWriter, r *http.Request) {
	if !strings.HasPrefix(r.URL.Path, defaultAPIRoot+"/") {
		writeJSON(w, http.StatusNotFound, cliResponse{Status: "error", Message: "invalid api path"})
		return
	}

	path := strings.TrimPrefix(r.URL.Path, defaultAPIRoot+"/")
	path = strings.Trim(path, "/")
	if path == "" {
		writeJSON(w, http.StatusBadRequest, cliResponse{Status: "error", Message: "missing command path"})
		return
	}

	parts := splitPath(path)
	if len(parts) == 0 {
		writeJSON(w, http.StatusBadRequest, cliResponse{Status: "error", Message: "missing command path"})
		return
	}

	resource := parts[0]
	resource = strings.TrimSpace(resource)
	if resource == "" {
		writeJSON(w, http.StatusBadRequest, cliResponse{Status: "error", Message: "invalid command"})
		return
	}

	command := []string{resource}
	params, positional, err := collectRequestParams(r)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, cliResponse{Status: "error", Message: "invalid request", Error: err.Error()})
		return
	}

	if resource == "send" {
		if len(parts) == 1 {
			kind := firstValue(params, "kind")
			if kind == "" {
				writeJSON(w, http.StatusBadRequest, cliResponse{Status: "error", Message: "send requires kind in path (/api/v1/send/{kind}) or body field kind"})
				return
			}
			command = append(command, kind)
			delete(params, "kind")
		} else {
			command = append(command, parts[1:]...)
		}
	} else if presets, ok := topLevelWithDefault[resource]; ok && len(parts) == 1 {
		if presets.defaultSubcommand != "" {
			command = append(command, presets.defaultSubcommand)
		} else if len(presets.defaultFlags) > 0 {
			command = append(command, presets.defaultFlags...)
		} else if !topLevelDirect[resource] {
			writeJSON(w, http.StatusBadRequest, cliResponse{Status: "error", Message: fmt.Sprintf("%s requires subcommand", resource)})
			return
		}
	} else {
		if len(parts) > 1 {
			command = append(command, parts[1:]...)
		}
	}

	if (resource == "messages" || resource == "contacts") && len(command) >= 2 && command[1] == "search" {
		if positional == nil {
			positional = []string{}
		}
		if q := firstValue(params, "query"); q != "" {
			positional = append([]string{q}, positional...)
			delete(params, "query")
		} else if q := firstValue(params, "q"); q != "" {
			positional = append([]string{q}, positional...)
			delete(params, "q")
		}
	}

	if resource == "sync" && len(parts) == 1 && !hasAny(params, "follow", "once") {
		command = append(command, "--once")
	}

	if !topLevelDirect[resource] && len(command) == 1 {
		writeJSON(w, http.StatusBadRequest, cliResponse{Status: "error", Message: fmt.Sprintf("%s requires a subcommand", resource)})
		return
	}

	// Keep send path backward-compatible for POST and GET.
	result, statusCode, err := cfg.runCLICommand(r.Context(), command, nil, params, positional)
	if err != nil {
		writeJSON(w, statusCode, cliResponse{Status: "error", Message: "command failed", Error: err.Error(), Data: result})
		return
	}
	writeJSON(w, statusCode, cliResponse{Status: "ok", Data: result})
}

func (cfg daemonConfig) runCLICommand(ctx context.Context, command []string, args []string, params map[string][]string, positional []string) (*commandResult, int, error) {
	if len(command) == 0 {
		return nil, http.StatusBadRequest, fmt.Errorf("empty command")
	}

	commandFlags := []string{}
	store := firstValue(params, "store")
	account := firstValue(params, "account")
	readOnly := cfg.defaultRO
	asJSON := cfg.defaultJSON
	full := cfg.defaultFull
	events := cfg.defaultEvents
	timeout := cfg.reqTimeout
	lockWait := firstValue(params, "lock_wait")

	if v := firstValue(params, "read_only"); v != "" {
		if b, err := parseBoolValue(v); err == nil {
			readOnly = b
		} else {
			return nil, http.StatusBadRequest, fmt.Errorf("invalid read_only: %w", err)
		}
	}
	if v := firstValue(params, "json"); v != "" {
		if b, err := parseBoolValue(v); err == nil {
			asJSON = b
		} else {
			return nil, http.StatusBadRequest, fmt.Errorf("invalid json: %w", err)
		}
	}
	if v := firstValue(params, "full"); v != "" {
		if b, err := parseBoolValue(v); err == nil {
			full = b
		} else {
			return nil, http.StatusBadRequest, fmt.Errorf("invalid full: %w", err)
		}
	}
	if v := firstValue(params, "events"); v != "" {
		if b, err := parseBoolValue(v); err == nil {
			events = b
		} else {
			return nil, http.StatusBadRequest, fmt.Errorf("invalid events: %w", err)
		}
	}
	if v := firstValue(params, "timeout"); v != "" {
		t, err := time.ParseDuration(v)
		if err != nil {
			return nil, http.StatusBadRequest, fmt.Errorf("invalid timeout: %w", err)
		}
		timeout = t
	}
	if v := firstValue(params, "timeout_secs"); v != "" {
		t, err := strconv.Atoi(v)
		if err != nil {
			return nil, http.StatusBadRequest, fmt.Errorf("invalid timeout_secs: %w", err)
		}
		timeout = time.Duration(t) * time.Second
	}

	if store == "" {
		store = cfg.defaultStore
	}
	if account == "" {
		account = cfg.defaultAccount
	}
	if store != "" {
		commandFlags = append(commandFlags, "--store", store)
	} else if account != "" {
		commandFlags = append(commandFlags, "--account", account)
	}
	if asJSON {
		commandFlags = append(commandFlags, "--json")
	}
	if full {
		commandFlags = append(commandFlags, "--full")
	}
	if events {
		commandFlags = append(commandFlags, "--events")
	}
	if readOnly {
		commandFlags = append(commandFlags, "--read-only")
	}
	if lockWait != "" {
		commandFlags = append(commandFlags, "--lock-wait", lockWait)
	}

	reqArgs := append([]string{}, commandFlags...)
	reqArgs = append(reqArgs, command...)
	reqArgs = append(reqArgs, positional...)

	if len(args) > 0 {
		reqArgs = append(reqArgs, args...)
	}
	for _, flagArg := range flagArgsFromParams(params) {
		reqArgs = append(reqArgs, flagArg)
	}

	ctxToUse := ctx
	cancel := func() {}
	if timeout > 0 {
		ctxToUse, cancel = context.WithTimeout(ctxToUse, timeout)
	}
	defer cancel()

	cmd := exec.CommandContext(ctxToUse, cfg.binaryPath, reqArgs...)
	cmd.Env = os.Environ()
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()
	result := &commandResult{Command: reqArgs, Stdout: strings.TrimSpace(stdout.String()), Stderr: strings.TrimSpace(stderr.String())}
	result.ExitCode = 0

	if err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			result.ExitCode = exitErr.ExitCode()
			if result.ExitCode == 0 {
				result.ExitCode = -1
			}
			return result, http.StatusBadRequest, fmt.Errorf("%v: %s", errors.New("command failed"), exitErrorMessage(result, exitErr, err))
		}
		if errors.Is(ctxToUse.Err(), context.DeadlineExceeded) {
			result.ExitCode = -2
			return result, http.StatusRequestTimeout, fmt.Errorf("command timeout exceeded")
		}
		result.ExitCode = -3
		return result, http.StatusInternalServerError, err
	}

	result.ExitCode = 0
	return &commandResult{Command: result.Command, Stdout: result.Stdout, Stderr: result.Stderr, ExitCode: result.ExitCode}, http.StatusOK, nil
}

func exitErrorMessage(res *commandResult, exitErr *exec.ExitError, err error) string {
	if res != nil && strings.TrimSpace(res.Stderr) != "" {
		return strings.TrimSpace(res.Stderr)
	}
	if len(exitErr.Stderr) > 0 {
		return string(exitErr.Stderr)
	}
	if err != nil {
		return strings.TrimSpace(err.Error())
	}
	return "command failed"
}

func flagArgsFromParams(params map[string][]string) []string {
	keys := make([]string, 0, len(params))
	for key := range params {
		if _, skip := reservedKeys[key]; skip {
			continue
		}
		keys = append(keys, key)
	}
	sort.Strings(keys)
	out := make([]string, 0)
	for _, key := range keys {
		flagKey := "--" + normalizeFlagName(key)
		for _, raw := range params[key] {
			if b, err := parseBoolValue(raw); err == nil {
				if b {
					out = append(out, flagKey)
				}
				continue
			}
			if raw == "" {
				out = append(out, flagKey)
				continue
			}
			out = append(out, flagKey, raw)
		}
	}
	return out
}

func collectRequestParams(r *http.Request) (map[string][]string, []string, error) {
	values := map[string][]string{}
	for key, vals := range r.URL.Query() {
		for _, value := range vals {
			if strings.TrimSpace(value) == "" {
				continue
			}
			values[key] = append(values[key], value)
		}
	}

	if r.Body == nil || (r.ContentLength == 0 && r.Method == http.MethodGet) {
		return values, nil, nil
	}

	body, err := io.ReadAll(r.Body)
	if err != nil {
		return nil, nil, fmt.Errorf("read request body: %w", err)
	}
	if len(body) == 0 {
		return values, nil, nil
	}

	contentType := strings.ToLower(r.Header.Get("Content-Type"))
	if !strings.Contains(contentType, "application/json") {
		return nil, nil, fmt.Errorf("unsupported content type: only application/json")
	}

	payload := map[string]any{}
	if err := json.Unmarshal(body, &payload); err != nil {
		return nil, nil, fmt.Errorf("invalid json body: %w", err)
	}

	positional := []string{}
	for key, raw := range payload {
		if key == "args" || key == "positional" {
			additional, err := normalizeStringArray(raw)
			if err != nil {
				return nil, nil, fmt.Errorf("invalid %s: %w", key, err)
			}
			positional = append(positional, additional...)
			continue
		}
		if err := appendValueFromInterface(values, key, raw); err != nil {
			return nil, nil, fmt.Errorf("invalid %s: %w", key, err)
		}
	}

	return values, positional, nil
}

func decodeJSONBody(r *http.Request, target any) error {
	if r.Body == nil {
		return fmt.Errorf("empty body")
	}
	decoder := json.NewDecoder(r.Body)
	if err := decoder.Decode(target); err != nil {
		return err
	}
	return nil
}

func normalizeFlagName(key string) string {
	key = strings.TrimSpace(key)
	key = strings.TrimPrefix(key, "--")
	key = strings.ReplaceAll(key, "_", "-")
	return key
}

func parseBoolValue(raw string) (bool, error) {
	s := strings.ToLower(strings.TrimSpace(raw))
	switch s {
	case "1", "true", "t", "yes", "on", "y":
		return true, nil
	case "0", "false", "f", "no", "off", "n":
		return false, nil
	default:
		return false, fmt.Errorf("invalid bool value %q", raw)
	}
}

func appendValueFromInterface(values map[string][]string, key string, raw any) error {
	normalized := normalizeFlagName(key)
	switch v := raw.(type) {
	case []any:
		for _, item := range v {
			str, err := valueToString(item)
			if err != nil {
				return fmt.Errorf("%s", err)
			}
			values[normalized] = append(values[normalized], str)
		}
		return nil
	case map[string]any:
		return fmt.Errorf("object is not supported")
	default:
		str, err := valueToString(v)
		if err != nil {
			return err
		}
		values[normalized] = append(values[normalized], str)
		return nil
	}
}

func valueToString(v any) (string, error) {
	switch x := v.(type) {
	case nil:
		return "", nil
	case string:
		return x, nil
	case bool:
		if x {
			return "true", nil
		}
		return "false", nil
	case float64:
		if x == float64(int64(x)) {
			return fmt.Sprintf("%d", int64(x)), nil
		}
		return fmt.Sprintf("%g", x), nil
	case json.Number:
		return x.String(), nil
	default:
		return "", fmt.Errorf("unsupported type %T", v)
	}
}

func firstValue(values map[string][]string, key string) string {
	key = normalizeFlagName(key)
	list := values[key]
	if len(list) == 0 {
		return ""
	}
	for i := range list {
		if strings.TrimSpace(list[i]) != "" {
			return list[i]
		}
	}
	return ""
}

func hasAny(values map[string][]string, keys ...string) bool {
	for _, key := range keys {
		if firstValue(values, key) != "" {
			return true
		}
	}
	return false
}

func writeJSON(w http.ResponseWriter, status int, payload cliResponse) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}

func splitPath(path string) []string {
	parts := strings.Split(path, "/")
	filtered := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		filtered = append(filtered, part)
	}
	return filtered
}

func normalizeStringArray(value any) ([]string, error) {
	slice, ok := value.([]any)
	if ok {
		out := make([]string, 0, len(slice))
		for _, item := range slice {
			str, err := valueToString(item)
			if err != nil {
				return nil, err
			}
			out = append(out, str)
		}
		return out, nil
	}

	str, ok := value.(string)
	if !ok {
		return nil, fmt.Errorf("expected array or string")
	}
	return []string{str}, nil
}

func parseEmbeddedJSON(raw string) map[string]any {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil
	}
	if raw[0] != '{' && raw[0] != '[' {
		return nil
	}
	var parsed map[string]any
	if err := json.Unmarshal([]byte(raw), &parsed); err != nil {
		return nil
	}
	return parsed
}
