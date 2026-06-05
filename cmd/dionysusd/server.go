package main

import (
	"crypto/subtle"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

type appHandler struct {
	cfg         Config
	serveStatic bool
}

func serveAPI(cfg Config) error {
	fmt.Printf("[dionysusd-api] listening on %s\n", cfg.Listen)
	server := &http.Server{
		Addr:              cfg.Listen,
		Handler:           appHandler{cfg: cfg},
		ReadHeaderTimeout: 5 * time.Second,
	}
	return server.ListenAndServe()
}

func serveProxy(cfg Config) error {
	fmt.Printf("[dionysusd-pveproxy] listening on %s\n", cfg.Listen)
	server := &http.Server{
		Addr:              cfg.Listen,
		Handler:           appHandler{cfg: cfg, serveStatic: true},
		ReadHeaderTimeout: 5 * time.Second,
	}
	return server.ListenAndServe()
}

func (handler appHandler) ServeHTTP(w http.ResponseWriter, request *http.Request) {
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")
	w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")

	if request.Method == http.MethodOptions {
		w.WriteHeader(http.StatusNoContent)
		return
	}

	if strings.HasPrefix(request.URL.Path, "/api2/json/") {
		if err := authorize(handler.cfg, request); err != nil {
			w.Header().Set("WWW-Authenticate", `Bearer realm="Dionysus PVE"`)
			writeAPIError(w, http.StatusUnauthorized, "auth", err.Error())
			return
		}
		data, status, err := handleAPI(handler.cfg, request)
		if err != nil {
			writeAPIError(w, status, "path", err.Error())
			return
		}
		writeAPIData(w, http.StatusOK, data)
		return
	}

	if handler.serveStatic {
		serveStaticFile(handler.cfg, w, request)
		return
	}

	writeAPIError(w, http.StatusNotFound, "path", "not found")
}

func handleAPI(cfg Config, request *http.Request) (any, int, error) {
	switch request.Method + " " + request.URL.Path {
	case "GET /api2/json/version":
		return map[string]any{
			"version": "0.1",
			"release": "dionysus-go",
			"stack":   "go-svelte-systemd",
			"api":     "api2-json",
		}, http.StatusOK, nil
	case "GET /api2/json/nodes":
		return []any{
			map[string]any{
				"node":   "localhost",
				"type":   "node",
				"status": "online",
				"id":     "node/localhost",
			},
		}, http.StatusOK, nil
	case "GET /api2/json/nodes/localhost/status":
		return nodeStatus(cfg), http.StatusOK, nil
	case "GET /api2/json/nodes/localhost/services":
		return serviceStatus(), http.StatusOK, nil
	case "POST /api2/json/nodes/localhost/console/exec":
		body := readJSONBody(request)
		data, err := runConsoleCommand(cfg, body)
		if err != nil {
			return nil, http.StatusBadRequest, err
		}
		return data, http.StatusOK, nil
	case "GET /api2/json/nodes/localhost/network/status":
		return networkStatus(cfg), http.StatusOK, nil
	case "GET /api2/json/nodes/localhost/network/config":
		return networkConfigPayload(cfg.NetworkConfig), http.StatusOK, nil
	case "POST /api2/json/nodes/localhost/network/config":
		body := readJSONBody(request)
		data, err := updateNetworkConfig(cfg, body)
		if err != nil {
			return nil, http.StatusBadRequest, err
		}
		return data, http.StatusOK, nil
	case "POST /api2/json/nodes/localhost/network/apply":
		body := readJSONBody(request)
		return applyNetworkService(cfg, jsonBool(body, "dryRun", false)), http.StatusOK, nil
	case "GET /api2/json/nodes/localhost/ollama/status":
		return ollamaStatus(cfg), http.StatusOK, nil
	case "GET /api2/json/nodes/localhost/ollama/kv-cache/profile":
		return kvCacheProfileStatus(cfg), http.StatusOK, nil
	case "GET /api2/json/nodes/localhost/ollama/history":
		limit, _ := strconv.ParseInt(request.URL.Query().Get("limit"), 10, 64)
		if limit == 0 {
			limit = 120
		}
		return metricsHistory(cfg.MetricsDB, limit), http.StatusOK, nil
	case "POST /api2/json/nodes/localhost/ollama/optimize":
		body := readJSONBody(request)
		return applyOllamaProfile(cfg, jsonBool(body, "dryRun", false)), http.StatusOK, nil
	default:
		return nil, http.StatusNotFound, fmt.Errorf("not found")
	}
}

func readJSONBody(request *http.Request) map[string]any {
	defer request.Body.Close()
	var body map[string]any
	if err := json.NewDecoder(request.Body).Decode(&body); err != nil {
		return map[string]any{}
	}
	return body
}

func serveStaticFile(cfg Config, w http.ResponseWriter, request *http.Request) {
	cleanPath := path.Clean("/" + request.URL.Path)
	if cleanPath == "/" {
		cleanPath = "/index.html"
	}
	target := filepath.Join(cfg.WWWRoot, filepath.FromSlash(strings.TrimPrefix(cleanPath, "/")))
	if fileExists(target) {
		http.ServeFile(w, request, target)
		return
	}

	indexPath := filepath.Join(cfg.WWWRoot, "index.html")
	if fileExists(indexPath) {
		http.ServeFile(w, request, indexPath)
		return
	}

	writeAPIError(w, http.StatusNotFound, "path", "not found")
}

func authorize(cfg Config, request *http.Request) error {
	if !fileExists(cfg.TokenFile) {
		return nil
	}
	raw, err := os.ReadFile(cfg.TokenFile)
	if err != nil {
		return fmt.Errorf("token file is unreadable")
	}
	expected := strings.TrimSpace(string(raw))
	header := request.Header.Get("Authorization")
	provided, ok := strings.CutPrefix(header, "Bearer ")
	if !ok {
		provided, ok = strings.CutPrefix(header, "bearer ")
	}
	if !ok {
		return fmt.Errorf("missing bearer token")
	}
	if subtle.ConstantTimeCompare([]byte(expected), []byte(provided)) == 1 {
		return nil
	}
	return fmt.Errorf("invalid bearer token")
}

func writeAPIData(w http.ResponseWriter, status int, data any) {
	writeJSON(w, status, map[string]any{"data": data})
}

func writeAPIError(w http.ResponseWriter, status int, key string, message string) {
	writeJSON(w, status, map[string]any{
		"errors": map[string]any{key: message},
		"data":   nil,
	})
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}
