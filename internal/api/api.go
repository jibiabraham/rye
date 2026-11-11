// Package api serves the GUI's JSON API on the service VM, for VMs and for Claude sessions through agent-manager.
package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"net"
	"net/http"
	"time"

	"rye/internal/agentmanager"
	"rye/internal/compute"
)

const (
	maxBody   = 32 << 20
	heartbeat = 15 * time.Second
	// pageSize is how many transcript messages a stream starts with, and how many each earlier page holds.
	pageSize = 100
)

// csp limits the web UI to its own files and keeps it out of frames, where a page could trick a click on Allow.
const csp = "default-src 'self'; img-src 'self' data:; style-src 'self' 'unsafe-inline'; frame-ancestors 'none'"

// Handler serves the API and the web UI. Browsers reach it through an SSH tunnel to 127.0.0.1.
type Handler struct {
	vms  VMs
	dial agentmanager.Dialer
	mux  *http.ServeMux
}

// VMs lists and finds the VMs that the GUI shows.
type VMs interface {
	List(ctx context.Context) ([]compute.VM, error)
	Get(ctx context.Context, name string) (compute.VM, error)
}

// New returns a Handler with its routes. It serves the web UI's built files from web.
func New(vms VMs, dial agentmanager.Dialer, web fs.FS) *Handler {
	h := &Handler{vms: vms, dial: dial, mux: http.NewServeMux()}
	files := http.FileServerFS(web)
	h.mux.Handle("GET /", files)
	// Vite names each asset by its content's hash, and a new build writes new names.
	h.mux.HandleFunc("GET /assets/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
		files.ServeHTTP(w, r)
	})
	// Links into the app load the page, and the app reads the path. Browsers revalidate the page on each load, because
	// each build names new assets.
	page := func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-cache")
		http.ServeFileFS(w, r, web, "index.html")
	}
	h.mux.HandleFunc("GET /{$}", page)
	h.mux.HandleFunc("GET /vms", page)
	h.mux.HandleFunc("GET /vms/", page)
	h.mux.HandleFunc("GET /api/vms", h.listVMs)
	list := h.forward(http.StatusOK, func(*http.Request, map[string]json.RawMessage) map[string]any {
		return map[string]any{"type": "list"}
	})
	h.mux.HandleFunc("GET /api/vms/{vm}/sessions", list)
	h.mux.HandleFunc("POST /api/vms/{vm}/sessions", h.forward(http.StatusCreated, func(_ *http.Request, body map[string]json.RawMessage) map[string]any {
		return map[string]any{"type": "create", "cwd": body["cwd"], "model": body["model"], "effort": body["effort"], "mode": body["mode"]}
	}))
	h.mux.HandleFunc("PATCH /api/vms/{vm}/sessions/{session}", h.forward(http.StatusNoContent, func(r *http.Request, body map[string]json.RawMessage) map[string]any {
		return map[string]any{"type": "rename", "session": r.PathValue("session"), "title": body["title"]}
	}))
	h.mux.HandleFunc("POST /api/vms/{vm}/sessions/{session}/messages", h.forward(http.StatusAccepted, func(r *http.Request, body map[string]json.RawMessage) map[string]any {
		return map[string]any{"type": "send", "session": r.PathValue("session"), "content": body["content"]}
	}))
	h.mux.HandleFunc("POST /api/vms/{vm}/sessions/{session}/interrupt", h.forward(http.StatusAccepted, func(r *http.Request, _ map[string]json.RawMessage) map[string]any {
		return map[string]any{"type": "interrupt", "session": r.PathValue("session")}
	}))
	h.mux.HandleFunc("POST /api/vms/{vm}/sessions/{session}/approvals/{request}", h.forward(http.StatusNoContent, func(r *http.Request, body map[string]json.RawMessage) map[string]any {
		return map[string]any{"type": "answer", "session": r.PathValue("session"), "request": r.PathValue("request"), "result": body}
	}))
	h.mux.HandleFunc("GET /api/vms/{vm}/sessions/{session}/messages", h.forward(http.StatusOK, func(r *http.Request, _ map[string]json.RawMessage) map[string]any {
		request := map[string]any{"type": "messages", "session": r.PathValue("session"), "before": r.URL.Query().Get("before")}
		// all=1 asks for every earlier message, for find-in-page over the whole session.
		if r.URL.Query().Get("all") != "1" {
			request["limit"] = pageSize
		}
		if from := r.URL.Query().Get("from"); from != "" {
			request["from"] = from
		}
		return request
	}))
	h.mux.HandleFunc("GET /api/vms/{vm}/sessions/{session}/events", h.events)
	models := h.forward(http.StatusOK, func(*http.Request, map[string]json.RawMessage) map[string]any {
		return map[string]any{"type": "models"}
	})
	h.mux.HandleFunc("GET /api/vms/{vm}/models", models)
	h.mux.HandleFunc("PUT /api/vms/{vm}/sessions/{session}/mode", h.forward(http.StatusNoContent, func(r *http.Request, body map[string]json.RawMessage) map[string]any {
		return map[string]any{"type": "mode", "session": r.PathValue("session"), "mode": body["mode"]}
	}))
	h.mux.HandleFunc("PUT /api/vms/{vm}/sessions/{session}/effort", h.forward(http.StatusNoContent, func(r *http.Request, body map[string]json.RawMessage) map[string]any {
		return map[string]any{"type": "effort", "session": r.PathValue("session"), "effort": body["effort"]}
	}))
	return h
}

// ServeHTTP serves requests on localhost, which arrive through the SSH tunnel.
func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if !localHost(r.Host) {
		problem(w, http.StatusForbidden, "host must be localhost")
		return
	}
	h.serve(w, r, "http://"+r.Host)
}

// Tailnet returns the handler for ryed's tailnet listener, which only tailscale serve reaches. tailscale serve
// sets Tailscale-User-Login to the requesting device's owner. Requests must name host and come from login.
func (h *Handler) Tailnet(host, login string) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Host != host {
			problem(w, http.StatusForbidden, "host must be the tailnet name")
			return
		}
		if r.Header.Get("Tailscale-User-Login") != login {
			problem(w, http.StatusForbidden, "tailnet user is not the owner")
			return
		}
		h.serve(w, r, "https://"+host)
	})
}

// serve rejects a browser Origin other than origin, then routes the request.
func (h *Handler) serve(w http.ResponseWriter, r *http.Request, origin string) {
	if o := r.Header.Get("Origin"); o != "" && o != origin {
		problem(w, http.StatusForbidden, "cross-origin request")
		return
	}
	w.Header().Set("Content-Security-Policy", csp)
	w.Header().Set("X-Content-Type-Options", "nosniff")
	h.mux.ServeHTTP(w, r)
}

// localHost rejects other names, because a DNS rebinding page sends its own name.
func localHost(host string) bool {
	name, _, err := net.SplitHostPort(host)
	if err != nil {
		name = host
	}
	return name == "localhost" || name == "127.0.0.1" || name == "[::1]" || name == "::1"
}

// listVMs lists every VM.
func (h *Handler) listVMs(w http.ResponseWriter, r *http.Request) {
	vms, err := h.vms.List(r.Context())
	if err != nil {
		slog.Error("list VMs", "err", err)
		problem(w, http.StatusBadGateway, "cannot list VMs")
		return
	}
	writeJSON(w, http.StatusOK, vms)
}

// forward sends one agent-manager request built from the path and the JSON body, and returns agent-manager's reply.
func (h *Handler) forward(status int, build func(*http.Request, map[string]json.RawMessage) map[string]any) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		body := map[string]json.RawMessage{}
		if r.Method != http.MethodGet && r.ContentLength != 0 {
			if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxBody)).Decode(&body); err != nil {
				problem(w, http.StatusBadRequest, "body must be a JSON object")
				return
			}
		}
		vm, ok := h.findVM(w, r)
		if !ok {
			return
		}
		request := build(r, body)
		for key, value := range request {
			if raw, isRaw := value.(json.RawMessage); isRaw && raw == nil {
				delete(request, key)
			}
		}
		reply, err := agentmanager.Call(r.Context(), h.dial, vm, request)
		if err != nil {
			agentManagerProblem(w, vm, err)
			return
		}
		if status == http.StatusNoContent || status == http.StatusAccepted {
			w.WriteHeader(status)
			return
		}
		writeJSON(w, status, reply)
	}
}

// events streams a session as server-sent events. The first event is `state` with any pending approvals, then
// `history` with the transcript, then each agent-manager event. A reconnect starts over, and clients dedupe messages
// by uuid.
func (h *Handler) events(w http.ResponseWriter, r *http.Request) {
	vm, ok := h.findVM(w, r)
	if !ok {
		return
	}
	session := r.PathValue("session")
	conn, err := agentmanager.Dial(r.Context(), h.dial, vm)
	if err != nil {
		agentManagerProblem(w, vm, err)
		return
	}
	defer func() { _ = conn.Close() }()
	// Reading starts before the requests, because a write waits for a reader.
	lines := make(chan agentmanager.Line)
	failed := make(chan error, 1)
	go func() {
		for {
			line, err := conn.Next()
			if err != nil {
				failed <- err
				return
			}
			select {
			case lines <- line:
			case <-r.Context().Done():
				return
			}
		}
	}()

	// Subscribing before requesting history keeps events from falling between the two.
	if err := conn.Send(1, map[string]any{"type": "subscribe", "session": session}); err != nil {
		agentManagerProblem(w, vm, err)
		return
	}
	if err := conn.Send(2, map[string]any{"type": "messages", "session": session, "limit": pageSize}); err != nil {
		agentManagerProblem(w, vm, err)
		return
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-store")
	rc := http.NewResponseController(w)
	ticker := time.NewTicker(heartbeat)
	defer ticker.Stop()
	for {
		var err error
		select {
		case <-r.Context().Done():
			return
		case err = <-failed:
			_, _ = fmt.Fprintf(w, "event: ended\ndata: %s\n\n", mustJSON(map[string]any{"session": session, "ended": true, "error": err.Error()}))
			_ = rc.Flush()
			return
		case <-ticker.C:
			// An event, because browsers hide SSE comments, and the GUI treats a quiet stream as lost.
			_, err = fmt.Fprint(w, "event: ping\ndata: {}\n\n")
		case line := <-lines:
			_, err = fmt.Fprintf(w, "event: %s\ndata: %s\n\n", eventName(line), line.Data)
		}
		if err != nil {
			return
		}
		_ = rc.Flush()
	}
}

func eventName(line agentmanager.Line) string {
	switch {
	case line.Error != "":
		return "agent-manager-error"
	case line.ID == 1:
		return "state"
	case line.ID == 2:
		return "history"
	default:
		return "message"
	}
}

// findVM returns the VM the path names.
func (h *Handler) findVM(w http.ResponseWriter, r *http.Request) (string, bool) {
	vm, err := h.vms.Get(r.Context(), r.PathValue("vm"))
	if errors.Is(err, compute.ErrNotFound) {
		problem(w, http.StatusNotFound, "no VM "+r.PathValue("vm"))
		return "", false
	}
	if err != nil {
		slog.Error("get VM", "vm", r.PathValue("vm"), "err", err)
		problem(w, http.StatusBadGateway, "cannot read VM "+r.PathValue("vm"))
		return "", false
	}
	return vm.Name, true
}

func agentManagerProblem(w http.ResponseWriter, vm string, err error) {
	var refused *agentmanager.RequestError
	if errors.As(err, &refused) {
		problem(w, http.StatusBadRequest, refused.Message)
		return
	}
	if errors.Is(err, context.Canceled) {
		return
	}
	slog.Error("agent-manager request", "vm", vm, "err", err)
	problem(w, http.StatusBadGateway, fmt.Sprintf("agent-manager in %s: %v", vm, err))
}

// problem writes an RFC 9457 problem details response.
func problem(w http.ResponseWriter, status int, detail string) {
	w.Header().Set("Content-Type", "application/problem+json")
	w.WriteHeader(status)
	_, _ = w.Write(mustJSON(map[string]any{"title": http.StatusText(status), "status": status, "detail": detail}))
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}

func mustJSON(value any) []byte {
	data, err := json.Marshal(value)
	if err != nil {
		panic(err)
	}
	return data
}
