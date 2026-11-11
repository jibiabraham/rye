package platform

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"rye/internal/compute"
	"rye/internal/compute/incus"
)

// fakeIncus serves the part of the Incus API that Admin and its VM provider call, on a unix socket. It keeps
// instances, networks and the files in VMs by path. Exec runs cat, test -e and mv on those files, and answers other
// commands from replies by their joined arguments. It records the calls that change instances or ACLs.
type fakeIncus struct {
	mu        sync.Mutex
	networks  map[string]map[string]string
	instances map[string]*incus.Instance
	files     map[string]string
	replies   map[string]compute.Result
	outputs   map[string]string
	execs     int
	calls     []string
}

// newFakeIncus serves f and returns an Admin connected to it.
func newFakeIncus(t *testing.T, f *fakeIncus) *incus.Admin {
	t.Helper()
	if f.networks == nil {
		f.networks = map[string]map[string]string{}
	}
	if f.instances == nil {
		f.instances = map[string]*incus.Instance{}
	}
	if f.files == nil {
		f.files = map[string]string{}
	}
	f.outputs = map[string]string{}
	socket := filepath.Join(t.TempDir(), "incus.socket")
	listener, err := net.Listen("unix", socket)
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewUnstartedServer(f)
	server.Listener = listener
	server.Start()
	t.Cleanup(server.Close)
	client, err := incus.ConnectAdmin(context.Background(), socket)
	if err != nil {
		t.Fatal(err)
	}
	return client
}

// recorded returns the state changes so far.
func (f *fakeIncus) recorded() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]string(nil), f.calls...)
}

func (f *fakeIncus) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()
	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	path := r.URL.Path
	rest, isInstance := strings.CutPrefix(path, "/1.0/instances/")
	name, sub, _ := strings.Cut(rest, "/")
	switch {
	case r.Method == http.MethodGet && path == "/1.0":
		reply(w, map[string]any{})
	case r.Method == http.MethodGet && strings.HasPrefix(path, "/1.0/networks/"):
		if config, ok := f.networks[strings.TrimPrefix(path, "/1.0/networks/")]; ok {
			reply(w, map[string]any{"config": config})
			return
		}
		notFound(w)
	case r.Method == http.MethodPost && path == "/1.0/instances":
		var create struct {
			Name string `json:"name"`
		}
		_ = json.Unmarshal(body, &create)
		f.calls = append(f.calls, "create "+create.Name)
		f.instances[create.Name] = &incus.Instance{Status: "Stopped"}
		reply(w, map[string]any{})
	case isInstance && strings.HasPrefix(sub, "logs/"):
		// An exec output log, which Run reads and then deletes.
		if r.Method == http.MethodGet {
			_, _ = io.WriteString(w, f.outputs[path])
		}
		delete(f.outputs, path)
		if r.Method != http.MethodGet {
			reply(w, map[string]any{})
		}
	case isInstance && sub == "" && f.instances[name] == nil:
		notFound(w)
	case isInstance && sub == "" && r.Method == http.MethodGet:
		reply(w, f.instances[name])
	case isInstance && sub == "" && r.Method == http.MethodDelete:
		f.calls = append(f.calls, "delete "+name)
		delete(f.instances, name)
		reply(w, map[string]any{})
	case isInstance && sub == "state":
		var state struct {
			Action string `json:"action"`
		}
		_ = json.Unmarshal(body, &state)
		f.calls = append(f.calls, state.Action+" "+name)
		if vm := f.instances[name]; vm != nil {
			vm.Status = map[string]string{"start": "Running", "stop": "Stopped"}[state.Action]
		}
		reply(w, map[string]any{})
	case isInstance && sub == "exec":
		f.exec(w, name, body)
	case isInstance && sub == "files" && r.Method == http.MethodPost:
		f.files[r.URL.Query().Get("path")] = string(body)
		reply(w, map[string]any{})
	case isInstance && sub == "files":
		notFound(w)
	case r.Method == http.MethodDelete && strings.HasPrefix(path, "/1.0/network-acls/"):
		f.calls = append(f.calls, "delete acl "+strings.TrimPrefix(path, "/1.0/network-acls/"))
		reply(w, map[string]any{})
	default:
		notFound(w)
	}
}

// exec runs a command on files or answers it from replies, with its output in log files.
func (f *fakeIncus) exec(w http.ResponseWriter, name string, body []byte) {
	var exec struct {
		Command []string `json:"command"`
	}
	_ = json.Unmarshal(body, &exec)
	result := f.run(exec.Command)
	f.execs++
	output := map[string]string{}
	for stream, data := range map[string]string{"1": result.Stdout, "2": result.Stderr} {
		log := fmt.Sprintf("/1.0/instances/%s/logs/exec-output/%d.%s", name, f.execs, stream)
		f.outputs[log] = data
		output[stream] = log
	}
	reply(w, map[string]any{"return": result.Code, "output": output})
}

// run returns a command's result. cat, test -e and mv act on files.
func (f *fakeIncus) run(args []string) compute.Result {
	switch {
	case len(args) == 2 && args[0] == "cat":
		if data, ok := f.files[args[1]]; ok {
			return compute.Result{Stdout: data}
		}
		return compute.Result{Code: 1, Stderr: "cat: " + args[1] + ": No such file or directory"}
	case len(args) == 3 && args[0] == "test" && args[1] == "-e":
		if _, ok := f.files[args[2]]; ok {
			return compute.Result{}
		}
		return compute.Result{Code: 1}
	case len(args) == 3 && args[0] == "mv":
		data, ok := f.files[args[1]]
		if !ok {
			return compute.Result{Code: 1, Stderr: "mv: cannot stat " + args[1]}
		}
		delete(f.files, args[1])
		f.files[args[2]] = data
		return compute.Result{}
	}
	return f.replies[strings.Join(args, " ")]
}

func reply(w http.ResponseWriter, metadata any) {
	_ = json.NewEncoder(w).Encode(map[string]any{"type": "sync", "metadata": metadata})
}

func notFound(w http.ResponseWriter) {
	w.WriteHeader(http.StatusNotFound)
	_, _ = io.WriteString(w, `{"type":"error","error":"not found"}`)
}
