package incus

import (
	"io"
	"os/exec"
	"regexp"
	"strings"
	"testing"

	"rye/internal/compute"
)

func TestGuestSetup(t *testing.T) {
	if placeholder := regexp.MustCompile(`@[A-Z_]+@`).FindString(guestSetup); placeholder != "" {
		t.Errorf("guestSetup has placeholder %s left:\n%s", placeholder, guestSetup)
	}
	cmd := exec.Command("sh", "-n")
	cmd.Stdin = strings.NewReader(guestSetup)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Errorf("sh -n guestSetup: %v\n%s", err, out)
	}
}

func TestVMAt(t *testing.T) {
	// The leases have no hostname, and vmAt matches them to instances by hardware address.
	leases := []lease{{Address: "198.18.0.2", Hwaddr: "aa"}, {Address: "198.18.0.3", Hwaddr: "bb"}}
	instances := []instance{
		{Name: "one", Config: map[string]string{hwaddrKey: "aa"}},
		{Name: "two", Config: map[string]string{hwaddrKey: "bb"}},
	}
	tests := []struct {
		name    string
		address string
		want    string
		wantErr bool
	}{
		{name: "first lease", address: "198.18.0.2", want: "one"},
		{name: "second lease", address: "198.18.0.3", want: "two"},
		{name: "unknown address", address: "198.18.0.9", wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := vmAt(tt.address, leases, instances)
			if (err != nil) != tt.wantErr {
				t.Fatalf("vmAt(%q) error = %v, want error %v", tt.address, err, tt.wantErr)
			}
			if got != tt.want {
				t.Errorf("vmAt(%q) = %q, want %q", tt.address, got, tt.want)
			}
		})
	}
}

func TestStatus(t *testing.T) {
	tests := []struct {
		name  string
		code  int
		ready bool
		want  compute.Status
	}{
		{"running before first boot", statusRunning, false, compute.StatusCreating},
		{"stopped before first boot", statusStopped, false, compute.StatusCreating},
		{"running", statusRunning, true, compute.StatusRunning},
		{"stopped", statusStopped, true, compute.StatusStopped},
		{"other code", 110, true, compute.StatusUnknown},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := status(tt.code, tt.ready); got != tt.want {
				t.Errorf("status(%v, %v) = %q, want %q", tt.code, tt.ready, got, tt.want)
			}
		})
	}
}

func TestBootLog_Update(t *testing.T) {
	tests := []struct {
		name  string
		reads []string
		want  string
	}{
		{"complete lines", []string{"one\ntwo\n"}, "  one\n  two\n"},
		{"partial line", []string{"one\ntw"}, "  one\n"},
		{"growing log", []string{"one\ntw", "one\ntwo\nthree\n"}, "  one\n  two\n  three\n"},
		{"unchanged log", []string{"one\ntwo\nthree\n", "one\ntwo\nthree\n"}, "  one\n  two\n  three\n"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var out strings.Builder
			log := &bootLog{out: &out}
			for _, read := range tt.reads {
				log.update([]byte(read))
			}
			if got := out.String(); got != tt.want {
				t.Errorf("out = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestBootLog_Tail(t *testing.T) {
	log := &bootLog{out: io.Discard}
	for i := range tailLines + 10 {
		log.update([]byte(strings.Repeat("x\n", i+1)))
	}
	if got := strings.Count(log.tail(), "\n") + 1; got != tailLines {
		t.Errorf("tail() has %d lines, want %d", got, tailLines)
	}
}
