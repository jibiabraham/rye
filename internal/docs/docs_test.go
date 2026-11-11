package docs

import (
	"archive/tar"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// writeArchive writes a tar in git archive's layout, with a global header holding the commit followed by the files.
func writeArchive(t *testing.T, path string, files map[string]string) {
	t.Helper()
	out, err := os.Create(filepath.Clean(path))
	if err != nil {
		t.Fatal(err)
	}
	w := tar.NewWriter(out)
	if err := w.WriteHeader(&tar.Header{Typeflag: tar.TypeXGlobalHeader, Name: "pax_global_header",
		PAXRecords: map[string]string{"comment": "0123abc"}}); err != nil {
		t.Fatal(err)
	}
	if err := w.WriteHeader(&tar.Header{Typeflag: tar.TypeDir, Name: "docs/", Mode: 0o755}); err != nil {
		t.Fatal(err)
	}
	for name, body := range files {
		if err := w.WriteHeader(&tar.Header{Typeflag: tar.TypeReg, Name: name, Mode: 0o644, Size: int64(len(body))}); err != nil {
			t.Fatal(err)
		}
		if _, err := io.WriteString(w, body); err != nil {
			t.Fatal(err)
		}
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	if err := out.Close(); err != nil {
		t.Fatal(err)
	}
}

var fixture = map[string]string{
	"README.md":                     "# rye\n\nrye is a service\nfor VMs.\n\n- more\n",
	"docs/how-to/b.md":              "# Do B\n\ntext\n",
	"docs/how-to/a.md":              "intro\n\n# Do A\n",
	"docs/tutorial/first-run.md":    "# First run\n",
	"docs/explanation/security.md":  "# Security model\n",
	"docs/reference/nested/deep.md": "# Not listed\n",
	"deployments/forge-vm/c.yaml":   "#cloud-config\n",
}

func get(t *testing.T, h http.Handler, target string) *httptest.ResponseRecorder {
	t.Helper()
	return serve(t, h, http.MethodGet, target)
}

func serve(t *testing.T, h http.Handler, method, target string) *httptest.ResponseRecorder {
	t.Helper()
	r := httptest.NewRequest(method, target, nil)
	r.Host = "docs.int.rye"
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	return w
}

const index = `# rye

> rye is a service for VMs.

These docs and code are rye at commit 0123abc. Every file in the repo is at http://docs.int.rye/<path>, and http://docs.int.rye/llms-full.txt has every page below in one file.

## Overview

- [rye](http://docs.int.rye/README.md)

## Tutorials

- [First run](http://docs.int.rye/docs/tutorial/first-run.md)

## How-to guides

- [Do A](http://docs.int.rye/docs/how-to/a.md)
- [Do B](http://docs.int.rye/docs/how-to/b.md)

## Explanation

- [Security model](http://docs.int.rye/docs/explanation/security.md)
`

func TestHandler_ServeHTTP(t *testing.T) {
	path := filepath.Join(t.TempDir(), "source.tar")
	writeArchive(t, path, fixture)
	tests := []struct {
		name         string
		source       string
		method       string
		target       string
		wantCode     int
		wantBody     string
		wantOrder    []string
		wantContains string
		wantNot      []string
	}{
		{name: "index", target: "/llms.txt", wantCode: http.StatusOK, wantBody: index},
		{name: "root", target: "/", wantCode: http.StatusOK, wantBody: index},
		{
			name:     "full",
			target:   "/llms-full.txt",
			wantCode: http.StatusOK,
			wantOrder: []string{"<!-- README.md -->", "<!-- docs/tutorial/first-run.md -->", "<!-- docs/how-to/a.md -->",
				"<!-- docs/how-to/b.md -->", "# Do B\n\ntext\n", "<!-- docs/explanation/security.md -->"},
			wantNot: []string{"Not listed"},
		},
		{name: "raw file", target: "/deployments/forge-vm/c.yaml", wantCode: http.StatusOK, wantBody: "#cloud-config\n"},
		{name: "unclean path", target: "/docs/../README.md", wantCode: http.StatusOK, wantBody: fixture["README.md"]},
		{name: "missing file", target: "/nope.md", wantCode: http.StatusNotFound},
		{name: "directory", target: "/docs", wantCode: http.StatusNotFound},
		{name: "directory with slash", target: "/docs/", wantCode: http.StatusNotFound},
		{name: "post", method: http.MethodPost, target: "/llms.txt", wantCode: http.StatusMethodNotAllowed},
		{
			name:         "no archive",
			source:       filepath.Join(t.TempDir(), "missing.tar"),
			target:       "/llms.txt",
			wantCode:     http.StatusServiceUnavailable,
			wantContains: "make up",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			source, method := tt.source, tt.method
			if source == "" {
				source = path
			}
			if method == "" {
				method = http.MethodGet
			}
			w := serve(t, &Handler{Source: source}, method, tt.target)
			body := w.Body.String()
			if w.Code != tt.wantCode {
				t.Errorf("%s %s: code = %d, want %d", method, tt.target, w.Code, tt.wantCode)
			}
			if tt.wantBody != "" && body != tt.wantBody {
				t.Errorf("%s %s: body = %q, want %q", method, tt.target, body, tt.wantBody)
			}
			if tt.wantContains != "" && !strings.Contains(body, tt.wantContains) {
				t.Errorf("%s %s: body = %q, want it to contain %q", method, tt.target, body, tt.wantContains)
			}
			at := 0
			for _, want := range tt.wantOrder {
				i := strings.Index(body[at:], want)
				if i < 0 {
					t.Fatalf("%s %s: body = %s, want %q after the previous entry", method, tt.target, body, want)
				}
				at += i
			}
			for _, unwanted := range tt.wantNot {
				if strings.Contains(body, unwanted) {
					t.Errorf("%s %s: body = %s, want it to lack %q", method, tt.target, body, unwanted)
				}
			}
		})
	}
}

func TestHandler_current(t *testing.T) {
	path := filepath.Join(t.TempDir(), "source.tar")
	h := &Handler{Source: path}
	if got := get(t, h, "/docs/how-to/b.md").Code; got != http.StatusServiceUnavailable {
		t.Fatalf("before the archive exists: status = %d, want %d", got, http.StatusServiceUnavailable)
	}
	writeArchive(t, path, fixture)
	if got, want := get(t, h, "/docs/how-to/b.md").Body.String(), "# Do B\n\ntext\n"; got != want {
		t.Errorf("once the archive exists: body = %q, want %q", got, want)
	}
}
