// Package vmexec runs commands and writes files in one VM through a provider. A command that exits nonzero is an
// error that carries its stderr.
package vmexec

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"rye/internal/compute"
)

// Runner writes files into VMs and runs commands in them. compute.Provider is one.
type Runner interface {
	Push(ctx context.Context, name string, file compute.File) error
	Run(ctx context.Context, name string, command compute.Command) (compute.Result, error)
}

// VM is one VM, reached through a Runner.
type VM struct {
	runner Runner
	Name   string
}

// New returns the VM name, reached through r.
func New(r Runner, name string) VM { return VM{runner: r, Name: name} }

// rootEnv is the environment of commands run as root.
var rootEnv = map[string]string{
	"DEBIAN_FRONTEND": "noninteractive",
	"PATH":            "/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin",
}

// ExitError is a command that ran and exited nonzero.
type ExitError struct {
	Code   int
	Stderr string
}

func (e *ExitError) Error() string {
	return fmt.Sprintf("exit %d: %s", e.Code, strings.TrimSpace(e.Stderr))
}

// Exec runs a command and returns its stdout. A nonzero exit is an *ExitError.
func (v VM) Exec(ctx context.Context, command compute.Command) (string, error) {
	result, err := v.runner.Run(ctx, v.Name, command)
	if err != nil {
		return "", err
	}
	if result.Code != 0 {
		return result.Stdout, &ExitError{Code: result.Code, Stderr: result.Stderr}
	}
	return result.Stdout, nil
}

// Root runs a command as root in / and returns its stdout. Its error names the command and the VM.
func (v VM) Root(ctx context.Context, args ...string) (string, error) {
	out, err := v.Exec(ctx, compute.Command{Args: args, Dir: "/", Env: rootEnv})
	if err != nil {
		return out, fmt.Errorf("%s in %s: %w", strings.Join(args, " "), v.Name, err)
	}
	return out, nil
}

// OK reports whether a command run as root exits 0. A nonzero exit is false, and a failure to run it is an error.
func (v VM) OK(ctx context.Context, args ...string) (bool, error) {
	_, err := v.Root(ctx, args...)
	var exit *ExitError
	if errors.As(err, &exit) {
		return false, nil
	}
	return err == nil, err
}

// Read returns a file's contents.
func (v VM) Read(ctx context.Context, path string) (string, error) {
	return v.Root(ctx, "cat", path)
}

// Write replaces a file with data.
func (v VM) Write(ctx context.Context, path, data string, mode os.FileMode, uid, gid int) error {
	return v.Push(ctx, compute.File{Path: path, Body: strings.NewReader(data), Size: int64(len(data)), Mode: mode,
		UID: uid, GID: gid})
}

// Copy replaces a file with a local file's contents, streamed.
func (v VM) Copy(ctx context.Context, path, local string, mode os.FileMode, uid, gid int) error {
	file, err := os.Open(filepath.Clean(local))
	if err != nil {
		return err
	}
	defer func() { _ = file.Close() }()
	info, err := file.Stat()
	if err != nil {
		return err
	}
	return v.Push(ctx, compute.File{Path: path, Body: file, Size: info.Size(), Mode: mode, UID: uid, GID: gid})
}

// Push writes a file next to its path, then renames it into place. A push that fails partway leaves the old file,
// and a running program keeps the inode it opened.
func (v VM) Push(ctx context.Context, file compute.File) error {
	path := file.Path
	file.Path = path + ".tmp"
	if err := v.runner.Push(ctx, v.Name, file); err != nil {
		return fmt.Errorf("writing %s in %s: %w", path, v.Name, err)
	}
	_, err := v.Root(ctx, "mv", file.Path, path)
	return err
}

// IDs returns a user's numeric user and group IDs.
func (v VM) IDs(ctx context.Context, user string) (uid, gid int, err error) {
	if uid, err = v.id(ctx, "-u", user); err != nil {
		return 0, 0, err
	}
	gid, err = v.id(ctx, "-g", user)
	return uid, gid, err
}

func (v VM) id(ctx context.Context, flag, user string) (int, error) {
	out, err := v.Root(ctx, "id", flag, user)
	if err != nil {
		return 0, err
	}
	n, err := strconv.Atoi(strings.TrimSpace(out))
	if err != nil {
		return 0, fmt.Errorf("id %s %s in %s printed %q", flag, user, v.Name, out)
	}
	return n, nil
}
