package agentmanager

import (
	"errors"
	"io"
	"os/exec"
	"testing"
)

func TestSSHConnClose(t *testing.T) {
	tests := []struct {
		name string
		cmd  func() *exec.Cmd
		read bool
	}{
		{name: "running", cmd: func() *exec.Cmd { return exec.Command("sleep", "60") }},
		{name: "ended", cmd: func() *exec.Cmd { return exec.Command("true") }, read: true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			c, err := startConn(tc.cmd())
			if err != nil {
				t.Fatal(err)
			}
			if tc.read {
				if _, err := c.Read(make([]byte, 1)); !errors.Is(err, io.EOF) {
					t.Fatalf("Read = %v, want EOF", err)
				}
			}
			if err := c.Close(); err != nil {
				t.Fatal(err)
			}
			if c.cmd.ProcessState == nil {
				t.Error("Close left the process unreaped")
			}
		})
	}
}
