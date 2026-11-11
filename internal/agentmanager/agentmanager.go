// Package agentmanager talks to agent-manager in a VM, one JSON message per line.
package agentmanager

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os/exec"
	"strings"
	"sync"
	"time"

	"rye/internal/vmconfig"
)

// Dialer opens a connection to agent-manager in a VM.
type Dialer func(ctx context.Context, vm string) (io.ReadWriteCloser, error)

// RequestError is an error reply from agent-manager.
type RequestError struct{ Message string }

func (e *RequestError) Error() string { return e.Message }

// PortDialer opens a connection to a TCP port on a VM's loopback interface.
type PortDialer func(ctx context.Context, vm, port string) (io.ReadWriteCloser, error)

// SSH dials agent-manager with ssh -W.
func SSH(knownHosts, controlDir string) Dialer {
	dial := SSHPort(knownHosts, controlDir)
	return func(ctx context.Context, vm string) (io.ReadWriteCloser, error) {
		return dial(ctx, vm, vmconfig.AgentManagerPort)
	}
}

// SSHPort dials 127.0.0.1:<port> in a VM with ssh -W. Connections to one VM share an SSH connection through a control
// socket.
func SSHPort(knownHosts, controlDir string) PortDialer {
	return func(ctx context.Context, vm, port string) (io.ReadWriteCloser, error) {
		// G204: ssh runs from argv, without a shell. vm is a VM name that the caller looked up. It follows the
		// user and "--" in argv.
		cmd := exec.CommandContext(ctx, "ssh", //nolint:gosec // see above
			"-o", "BatchMode=yes",
			"-o", "StrictHostKeyChecking=accept-new",
			"-o", "UserKnownHostsFile="+knownHosts,
			"-o", "HashKnownHosts=no",
			"-o", "ControlMaster=auto",
			"-o", "ControlPersist=60",
			"-o", "ControlPath="+controlDir+"/%C",
			"-W", "127.0.0.1:"+port, "--", vmconfig.GuestUser+"@"+vm+".rye")
		return startConn(cmd)
	}
}

// startConn starts cmd and connects to its stdin and stdout.
func startConn(cmd *exec.Cmd) (*sshConn, error) {
	stderr := &bytes.Buffer{}
	cmd.Stderr = stderr
	// Wait returns this long after ssh exits, even when a process ssh started still holds stderr open.
	cmd.WaitDelay = 2 * time.Second
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, err
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, err
	}
	if err := cmd.Start(); err != nil {
		return nil, err
	}
	return &sshConn{cmd: cmd, stdin: stdin, stdout: stdout, stderr: stderr}, nil
}

type sshConn struct {
	cmd    *exec.Cmd
	stdin  io.WriteCloser
	stdout io.Reader
	stderr *bytes.Buffer
	waited sync.Once
}

// wait reaps ssh once. Read calls it when ssh ends, and Close after killing it.
func (c *sshConn) wait() { c.waited.Do(func() { _ = c.cmd.Wait() }) }

func (c *sshConn) Write(p []byte) (int, error) { return c.stdin.Write(p) }

// Read reports ssh's message when the connection ends.
func (c *sshConn) Read(p []byte) (int, error) {
	n, err := c.stdout.Read(p)
	if errors.Is(err, io.EOF) && n == 0 {
		c.wait()
		if message := strings.TrimSpace(c.stderr.String()); message != "" {
			return 0, errors.New(message)
		}
	}
	return n, err
}

func (c *sshConn) Close() error {
	_ = c.stdin.Close()
	_ = c.cmd.Process.Kill()
	c.wait()
	return nil
}

// Line is one message from agent-manager. ID is set on replies.
type Line struct {
	ID    int
	Data  json.RawMessage
	Error string
}

// Conn is an open connection to agent-manager.
type Conn struct {
	rw     io.ReadWriteCloser
	reader *bufio.Reader
}

// Dial opens a connection to agent-manager in vm.
func Dial(ctx context.Context, dial Dialer, vm string) (*Conn, error) {
	rw, err := dial(ctx, vm)
	if err != nil {
		return nil, err
	}
	return &Conn{rw: rw, reader: bufio.NewReader(rw)}, nil
}

// Send writes a request with an ID.
func (c *Conn) Send(id int, request map[string]any) error {
	request["id"] = id
	line, err := json.Marshal(request)
	if err != nil {
		return err
	}
	_, err = c.rw.Write(append(line, '\n'))
	return err
}

// Next reads the next line.
func (c *Conn) Next() (Line, error) {
	data, err := c.reader.ReadBytes('\n')
	if err != nil {
		if errors.Is(err, io.EOF) {
			err = errors.New("agent-manager closed the connection")
		}
		return Line{}, err
	}
	var head struct {
		ID    int    `json:"id"`
		Error string `json:"error"`
	}
	if err := json.Unmarshal(data, &head); err != nil {
		return Line{}, fmt.Errorf("agent-manager sent %q: %w", data, err)
	}
	return Line{ID: head.ID, Data: bytes.TrimSpace(data), Error: head.Error}, nil
}

// Close closes the connection.
func (c *Conn) Close() error { return c.rw.Close() }

// Call sends one request and returns its reply, without the ID.
func Call(ctx context.Context, dial Dialer, vm string, request map[string]any) (map[string]json.RawMessage, error) {
	conn, err := Dial(ctx, dial, vm)
	if err != nil {
		return nil, err
	}
	defer func() { _ = conn.Close() }()
	if err := conn.Send(1, request); err != nil {
		return nil, err
	}
	for {
		line, err := conn.Next()
		if err != nil {
			return nil, err
		}
		if line.ID != 1 {
			continue
		}
		if line.Error != "" {
			return nil, &RequestError{line.Error}
		}
		var reply map[string]json.RawMessage
		if err := json.Unmarshal(line.Data, &reply); err != nil {
			return nil, err
		}
		delete(reply, "id")
		return reply, nil
	}
}
