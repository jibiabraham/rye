package main

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"maps"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"

	"rye/internal/compute"
	"rye/internal/integration"
	"rye/internal/vmconfig"
)

type stubProvider struct {
	compute.Provider
	vms           map[string]bool
	running       map[string]bool
	created       compute.Spec
	deleted       string
	resized       compute.Size
	agentManagers map[string]bool
	pushed        string
	marked        []string
	started       string
}

func (s *stubProvider) Push(_ context.Context, _ string, file compute.File) error {
	data, err := io.ReadAll(file.Body)
	s.pushed = string(data)
	return err
}

// Run answers id with 1000 and every other command with success.
func (s *stubProvider) Run(_ context.Context, _ string, c compute.Command) (compute.Result, error) {
	if c.Args[0] == "id" {
		return compute.Result{Stdout: "1000\n"}, nil
	}
	return compute.Result{}, nil
}

func (s *stubProvider) Resize(_ context.Context, _ string, size compute.Size) error {
	s.resized = size
	return nil
}

func (s *stubProvider) Create(_ context.Context, spec compute.Spec, out io.Writer) (compute.VM, error) {
	s.created = spec
	_, _ = io.WriteString(out, "step...\n")
	return compute.VM{Name: spec.Name}, nil
}

func (s *stubProvider) MarkAgentManager(_ context.Context, name string) error {
	s.marked = append(s.marked, name)
	return nil
}

func (s *stubProvider) Start(_ context.Context, name string) error {
	s.started = name
	return nil
}

func (s *stubProvider) Delete(_ context.Context, name string) error {
	s.deleted = name
	return nil
}

func (s *stubProvider) Get(_ context.Context, name string) (compute.VM, error) {
	if !s.vms[name] {
		return compute.VM{}, fmt.Errorf("/1.0/instances/%s: %w", name, compute.ErrNotFound)
	}
	vm := compute.VM{Name: name, Status: compute.StatusStopped, AgentManager: s.agentManagers[name]}
	if s.running[name] {
		vm.Status = compute.StatusRunning
	}
	return vm, nil
}

// List returns the VMs in name order, with their status and agent-manager mark.
func (s *stubProvider) List(context.Context) ([]compute.VM, error) {
	var vms []compute.VM
	for _, name := range slices.Sorted(maps.Keys(s.vms)) {
		vm, _ := s.Get(context.Background(), name)
		vms = append(vms, vm)
	}
	return vms, nil
}

// testStateDir returns an empty state directory.
func testStateDir(t *testing.T) string {
	t.Helper()
	return t.TempDir()
}

// TestIntegrationCommand runs integration commands in order against one store and provider.
func TestIntegrationCommand(t *testing.T) {
	dir := testStateDir(t)
	p := &stubProvider{vms: map[string]bool{"a": true}}
	integrationCmd := func(stdin string, args ...string) (string, error) {
		var out bytes.Buffer
		err := dispatch(context.Background(), p, dir, "integration", args, strings.NewReader(stdin), &out)
		return out.String(), err
	}
	newVM := func() error {
		return dispatch(context.Background(), p, dir, "new", []string{"a"}, strings.NewReader(""), &bytes.Buffer{})
	}
	attached := func() bool {
		t.Helper()
		store, err := integration.Load(storeIn(dir))
		if err != nil {
			t.Fatal(err)
		}
		return store.Attached("forgejo", "a")
	}

	if _, err := integrationCmd("secret\n", "add", "--url", "http://f", "forgejo"); err != nil {
		t.Fatalf("add forgejo = %v, want nil", err)
	}
	if _, err := integrationCmd("", "attach", "forgejo", "a"); err != nil {
		t.Fatalf("attach forgejo a = %v, want nil", err)
	}
	out, err := integrationCmd("", "ls")
	if err != nil {
		t.Fatalf("ls = %v, want nil", err)
	}
	if strings.Contains(out, "secret") || !strings.Contains(out, "forgejo") {
		t.Errorf("ls output = %q, want forgejo without the secret", out)
	}
	if err := newVM(); err == nil {
		t.Fatal("new a on an existing VM = nil error, want an error")
	}
	if !attached() {
		t.Error("attached(forgejo, a) = false after a failed new, want true")
	}
	delete(p.vms, "a")
	if _, err := integrationCmd("", "detach", "missing", "a"); err == nil {
		t.Error("detach missing a = nil, want an error")
	}
	if _, err := integrationCmd("", "detach", "forgejo", "a"); err != nil {
		t.Fatalf("detach forgejo a after deleting a = %v, want nil", err)
	}
	if _, err := integrationCmd("", "attach", "forgejo", "gone"); err == nil {
		t.Error("attach forgejo gone = nil error, want an error")
	}
	p.vms["a"] = true
	if _, err := integrationCmd("", "attach", "forgejo", "a"); err != nil {
		t.Fatalf("attach forgejo a again = %v, want nil", err)
	}
	delete(p.vms, "a")
	if err := newVM(); err != nil {
		t.Fatalf("new a after deleting a = %v, want nil", err)
	}
	if attached() {
		t.Error("attached(forgejo, a) = true after recreating a, want false")
	}
}

func TestCommandArgs(t *testing.T) {
	tests := []struct {
		name       string
		args       []string
		sshCommand string
		want       []string
	}{
		{"os args", []string{"ls"}, "", []string{"ls"}},
		{"ssh command", nil, "show a --json", []string{"show", "a", "--json"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := commandArgs(tt.args, tt.sshCommand); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("commandArgs(%q, %q) = %q, want %q", tt.args, tt.sshCommand, got, tt.want)
			}
		})
	}
}

func TestRun(t *testing.T) {
	tests := []struct {
		name string
		args []string
	}{
		{"no arguments", nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var out bytes.Buffer
			if err := run(context.Background(), tt.args, nil, &out); err != nil {
				t.Fatalf("run(%q) error = %v", tt.args, err)
			}
			if !strings.HasPrefix(out.String(), "usage: rye") {
				t.Errorf("run(%q) output = %q, want usage", tt.args, out.String())
			}
		})
	}
}

func TestDispatch(t *testing.T) {
	tests := []struct {
		name    string
		command string
		args    []string
		wantErr string // the exact error; "" accepts any error
	}{
		{"unknown command", "nope", nil, ""},
		{"missing vm", "show", []string{"nosuch"}, "no VM nosuch; ssh rye ls lists them"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := dispatch(context.Background(), &stubProvider{}, testStateDir(t), tt.command, tt.args, nil, &bytes.Buffer{})
			if err == nil {
				t.Fatalf("dispatch(%q, %q) error = nil, want an error", tt.command, tt.args)
			}
			if tt.wantErr != "" && err.Error() != tt.wantErr {
				t.Errorf("dispatch(%q, %q) error = %q, want %q", tt.command, tt.args, err, tt.wantErr)
			}
		})
	}
}

func TestNewVM(t *testing.T) {
	tests := []struct {
		name          string
		args          []string
		stdin         string
		wantErr       bool
		wantCloudInit string
	}{
		{"cloud-init from stdin", []string{"a", "--cloud-init"}, "#cloud-config", false, "#cloud-config"},
		{"flags before the name", []string{"--cpu", "4", "a"}, "", false, ""},
		{"no name", nil, "", true, ""},
		{"two names", []string{"a", "b"}, "", true, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := &stubProvider{}
			var out bytes.Buffer
			err := dispatch(context.Background(), p, testStateDir(t), "new", tt.args, strings.NewReader(tt.stdin), &out)
			if (err != nil) != tt.wantErr {
				t.Fatalf("new %q error = %v, want error %v", tt.args, err, tt.wantErr)
			}
			if tt.wantErr {
				return
			}
			if p.created.CloudInit != tt.wantCloudInit {
				t.Errorf("cloud-init = %q, want %q", p.created.CloudInit, tt.wantCloudInit)
			}
			if !strings.Contains(out.String(), "Ready: a") {
				t.Errorf("output = %q, want it to contain %q", out.String(), "Ready: a")
			}
		})
	}
}

func TestRemove(t *testing.T) {
	tests := []struct {
		name           string
		args           []string
		wantErr        bool
		wantDeleted    string
		wantOut        string
		wantKnownHosts string
	}{
		{"without yes", []string{"box"}, true, "", "", ""},
		{"with yes", []string{"box", "--yes"}, false, "box",
			"Removed integration onbox, which pointed at box\nRemoved box\n", "other.rye ssh-ed25519 B\n"},
		{"missing VM", []string{"gone", "--yes"}, false, "gone",
			"Removed integration ongone, which pointed at gone\nNo VM gone; removed its host key and integrations\n",
			"box.rye ssh-ed25519 A\nother.rye ssh-ed25519 B\n"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			store := testStateDir(t)
			knownHosts := filepath.Join(store, vmconfig.KnownHostsFile)
			if err := os.WriteFile(knownHosts, []byte("box.rye ssh-ed25519 A\nother.rye ssh-ed25519 B\n"), 0o600); err != nil {
				t.Fatal(err)
			}
			p := &stubProvider{vms: map[string]bool{"box": true}}
			for name, url := range map[string]string{
				"onbox": "http://box.rye:3000", "ongone": "http://gone.rye:3000", "outside": "https://example.com",
			} {
				err := dispatch(context.Background(), p, store, "integration", []string{"add", name, "--url", url},
					strings.NewReader("token"), io.Discard)
				if err != nil {
					t.Fatal(err)
				}
			}
			var out bytes.Buffer
			err := dispatch(context.Background(), p, store, "rm", tt.args, strings.NewReader(""), &out)
			if (err != nil) != tt.wantErr {
				t.Fatalf("rm %q error = %v, want error %v", tt.args, err, tt.wantErr)
			}
			if p.deleted != tt.wantDeleted {
				t.Errorf("deleted = %q, want %q", p.deleted, tt.wantDeleted)
			}
			if tt.wantErr {
				return
			}
			if out.String() != tt.wantOut {
				t.Errorf("output = %q, want %q", out.String(), tt.wantOut)
			}
			if got, _ := os.ReadFile(filepath.Clean(knownHosts)); string(got) != tt.wantKnownHosts {
				t.Errorf("known_hosts = %q, want %q", got, tt.wantKnownHosts)
			}
			var list bytes.Buffer
			if err := dispatch(context.Background(), p, store, "integration", []string{"ls"}, strings.NewReader(""), &list); err != nil {
				t.Fatal(err)
			}
			if removed := "on" + tt.wantDeleted; strings.Contains(list.String(), removed) || !strings.Contains(list.String(), "outside") {
				t.Errorf("integration ls = %q, want outside and no %s", list.String(), removed)
			}
		})
	}
}

func TestBootOutput(t *testing.T) {
	lines := "Creating VM...\n  Get:1 http://archive.ubuntu.com resolute InRelease\n" +
		"  Cloud-init v. 26.2 running 'modules:final' at now. Up 9 seconds.\n  W: something to warn about\n" +
		"  Setting up openssh-server\n  Unpacking liberror-perl (0.17030-1)\n  2026-11-07 ubuntu.py[ERROR]: Failed to wait for network\n  cloud-init: done"
	tests := []struct {
		name    string
		verbose bool
		want    string
	}{
		{"quiet", false, "Creating VM...\n  Cloud-init v. 26.2 running 'modules:final' at now. Up 9 seconds.\n  W: something to warn about\n  2026-11-07 ubuntu.py[ERROR]: Failed to wait for network\n  cloud-init: done\n"},
		{"verbose", true, lines + "\n"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var out bytes.Buffer
			w := bootOutput(&out, tt.verbose)
			_, _ = io.WriteString(w, lines)
			w.Flush()
			if out.String() != tt.want {
				t.Errorf("output = %q, want %q", out.String(), tt.want)
			}
		})
	}
}

func TestResize(t *testing.T) {
	p := &stubProvider{vms: map[string]bool{"a": true}}
	if err := dispatch(context.Background(), p, testStateDir(t), "resize", []string{"a", "--cpu", "4", "--disk", "40GiB"}, nil, nil); err != nil {
		t.Fatal(err)
	}
	if want := (compute.Size{CPU: 4, Disk: "40GiB"}); p.resized != want {
		t.Errorf("resized = %+v, want %+v", p.resized, want)
	}
}

func TestSSHConfig(t *testing.T) {
	tests := []struct {
		name       string
		connection string
		want       string
		wantErr    bool
	}{
		{name: "third field", connection: "192.0.2.9 50000 192.0.2.1 22",
			want: "HostName 192.0.2.1"},
		{name: "wrong field count", connection: "192.0.2.9 50000 192.0.2.1", wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var out bytes.Buffer
			err := sshConfig(tt.connection, &out)
			if (err != nil) != tt.wantErr {
				t.Fatalf("sshConfig(%q) error = %v, want error %v", tt.connection, err, tt.wantErr)
			}
			if !strings.Contains(out.String(), tt.want) {
				t.Errorf("sshConfig(%q) output = %q, want it to contain %q", tt.connection, out.String(), tt.want)
			}
		})
	}
}

func TestAgentManagerCommand(t *testing.T) {
	tests := []struct {
		name    string
		running bool
		wantErr string // "" means success
	}{
		{"install", true, ""},
		{"stopped vm", false, "box is stopped; ssh rye start box"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			store := testStateDir(t)
			p := &stubProvider{vms: map[string]bool{"box": true}, running: map[string]bool{"box": tt.running}}
			err := dispatch(context.Background(), p, store, "agent-manager", []string{"install", "box"}, strings.NewReader(""), &bytes.Buffer{})
			if tt.wantErr != "" {
				if err == nil || err.Error() != tt.wantErr {
					t.Errorf("error = %v, want %q", err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if p.pushed == "" || !slices.Equal(p.marked, []string{"box"}) {
				t.Errorf("pushed %d bytes and marked %q, want the package pushed and %q marked", len(p.pushed), p.marked, []string{"box"})
			}
		})
	}
}

func TestAgentManagerCommandAll(t *testing.T) {
	store := testStateDir(t)
	p := &stubProvider{
		vms:           map[string]bool{"a": true, "b": true, "c": true, "d": true},
		running:       map[string]bool{"a": true, "b": true, "d": true},
		agentManagers: map[string]bool{"a": true, "c": true, "d": true},
	}
	var out bytes.Buffer
	if err := dispatch(context.Background(), p, store, "agent-manager", []string{"install", "--all"}, strings.NewReader(""), &out); err != nil {
		t.Fatal(err)
	}
	if want := []string{"a", "d"}; !slices.Equal(p.marked, want) {
		t.Errorf("installed in %q, want %q", p.marked, want)
	}
	if want := "Skipped c, which is stopped"; !strings.Contains(out.String(), want) {
		t.Errorf("output = %q, want it to contain %q", out.String(), want)
	}
}

func TestStart(t *testing.T) {
	tests := []struct {
		name         string
		agentManager bool
		wantMarked   []string
	}{
		{name: "with agent-manager", agentManager: true, wantMarked: []string{"a"}},
		{name: "without agent-manager"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := &stubProvider{vms: map[string]bool{"a": true}, agentManagers: map[string]bool{"a": tt.agentManager}}
			if err := dispatch(context.Background(), p, testStateDir(t), "start", []string{"a"}, nil, &bytes.Buffer{}); err != nil {
				t.Fatal(err)
			}
			if p.started != "a" {
				t.Errorf("started = %q, want a", p.started)
			}
			if !slices.Equal(p.marked, tt.wantMarked) {
				t.Errorf("installed agent-manager in %q, want %q", p.marked, tt.wantMarked)
			}
		})
	}
}
