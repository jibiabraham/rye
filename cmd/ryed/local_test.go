package main

import (
	"bufio"
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"
	"testing/fstest"

	"rye/internal/api"
)

// TestLocalProvider serves the GUI API from localProvider and dialLocal, with a fake agent-manager on the
// localAgentManager address.
func TestLocalProvider(t *testing.T) {
	manager, err := net.Listen("tcp", localAgentManager)
	if err != nil {
		t.Skipf("port in use: %v", err)
	}
	t.Cleanup(func() { _ = manager.Close() })
	go func() {
		for {
			conn, err := manager.Accept()
			if err != nil {
				return
			}
			go func() {
				defer func() { _ = conn.Close() }()
				line, _ := bufio.NewReader(conn).ReadBytes('\n')
				var request struct {
					ID int `json:"id"`
				}
				_ = json.Unmarshal(line, &request)
				_ = json.NewEncoder(conn).Encode(map[string]any{"id": request.ID, "sessions": []any{}, "live": []any{}})
			}()
		}
	}()
	h := api.New(localProvider{name: "rye-admin"}, dialLocal, fstest.MapFS{})
	tests := []struct {
		name string
		path string
		want int
	}{
		{"list vms", "/api/vms", http.StatusOK},
		{"own sessions", "/api/vms/rye-admin/sessions", http.StatusOK},
		{"other vm", "/api/vms/other/sessions", http.StatusNotFound},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			response := httptest.NewRecorder()
			h.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "http://localhost"+tt.path, nil))
			if response.Code != tt.want {
				t.Errorf("GET %s status = %d, want %d; body %s", tt.path, response.Code, tt.want, response.Body)
			}
		})
	}
}
