package vmconfig

import (
	"fmt"
	"os/exec"
	"strings"
	"testing"
)

func TestNodeSetup(t *testing.T) {
	if repo := fmt.Sprintf("https://deb.nodesource.com/node_%d.x", NodeMajor); !strings.Contains(NodeSetup, repo) {
		t.Errorf("NodeSetup = %q, want it to add %s", NodeSetup, repo)
	}
	cmd := exec.Command("sh", "-n")
	cmd.Stdin = strings.NewReader(NodeSetup)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Errorf("sh -n NodeSetup: %v\n%s", err, out)
	}
}
