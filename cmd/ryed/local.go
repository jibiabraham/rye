package main

import (
	"context"
	"io"
	"net"

	"rye/internal/compute"
	"rye/internal/vmconfig"
)

// localAgentManager is agent-manager on the machine ryed -local runs on.
const localAgentManager = "127.0.0.1:" + vmconfig.AgentManagerPort

// localProvider presents the machine ryed -local runs on as the only VM, running and with agent-manager.
type localProvider struct{ name string }

func (p localProvider) vm() compute.VM {
	return compute.VM{Name: p.name, Status: compute.StatusRunning, AgentManager: true}
}

func (p localProvider) List(context.Context) ([]compute.VM, error) { return []compute.VM{p.vm()}, nil }

func (p localProvider) Get(_ context.Context, name string) (compute.VM, error) {
	if name != p.name {
		return compute.VM{}, compute.ErrNotFound
	}
	return p.vm(), nil
}

// dialLocal connects to the local agent-manager over TCP.
func dialLocal(ctx context.Context, _ string) (io.ReadWriteCloser, error) {
	var d net.Dialer
	return d.DialContext(ctx, "tcp", localAgentManager)
}
