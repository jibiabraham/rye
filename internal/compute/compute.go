// Package compute defines the VM lifecycle that platform commands use.
package compute

import (
	"context"
	"errors"
	"io"
	"os"
)

// ErrNotFound reports a VM that does not exist.
var ErrNotFound = errors.New("not found")

// Status is the lifecycle state of a VM.
type Status string

// VM statuses.
const (
	StatusCreating Status = "creating"
	StatusRunning  Status = "running"
	StatusStopped  Status = "stopped"
	StatusUnknown  Status = "unknown"
)

// Spec describes a VM to create.
type Spec struct {
	Name      string
	CPU       int
	Memory    string
	Disk      string
	CloudInit string
}

// Size holds new sizes for a VM. Zero values stay as they are.
type Size struct {
	CPU    int
	Memory string
	Disk   string
}

// VM is a VM as the platform reports it.
type VM struct {
	Name   string `json:"name"`
	Status Status `json:"status"`
	CPU    string `json:"cpu"`
	Memory string `json:"memory"`
	Disk   string `json:"disk"`
	// AgentManager reports that rye agent-manager install has run on the VM.
	AgentManager bool `json:"agentManager"`
}

// File is a file to write into a VM.
type File struct {
	Path string
	// Body holds Size bytes, which the provider streams into the VM.
	Body io.Reader
	Size int64
	Mode os.FileMode
	UID  int
	GID  int
}

// Command is a command to run in a VM, as UID and GID in Dir with Env.
type Command struct {
	Args []string
	UID  int
	GID  int
	Dir  string
	Env  map[string]string
}

// Result is a finished command's exit code and output.
type Result struct {
	Code   int
	Stdout string
	Stderr string
}

// Provider runs VMs.
type Provider interface {
	// Create builds and starts a VM and returns once it is ready. It writes each step, and the guest's first-boot
	// output, to out.
	Create(ctx context.Context, spec Spec, out io.Writer) (VM, error)
	List(ctx context.Context) ([]VM, error)
	Get(ctx context.Context, name string) (VM, error)
	Delete(ctx context.Context, name string) error
	// Start boots a stopped VM and runs rye's setup in it.
	Start(ctx context.Context, name string) error
	Stop(ctx context.Context, name string) error
	// Restart reboots a VM and runs rye's setup in it.
	Restart(ctx context.Context, name string) error
	// Resize changes a stopped VM's sizes. Disks only grow.
	Resize(ctx context.Context, name string, size Size) error
	// MarkAgentManager records that agent-manager is installed on a VM. The mark lasts until the VM is removed.
	MarkAgentManager(ctx context.Context, name string) error
	// FindByAddress returns the name of the VM at an address, from data the guest can't change.
	FindByAddress(ctx context.Context, address string) (string, error)
	// Push writes a file into a running VM, replacing any file at its path.
	Push(ctx context.Context, name string, file File) error
	// Run runs a command in a running VM and returns once it ends.
	Run(ctx context.Context, name string, command Command) (Result, error)
}
