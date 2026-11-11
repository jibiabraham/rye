// Package incus implements compute.Provider on Incus.
package incus

import (
	"bytes"
	"context"
	_ "embed" // vendor-data files
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"rye/internal/compute"
	"rye/internal/vmconfig"
)

const (
	imageServer     = "https://images.linuxcontainers.org"
	imageAlias      = "ubuntu/26.04/cloud"
	readyKey        = "user.rye.ready"
	agentManagerKey = "user.rye.agent-manager"
	hwaddrKey       = "volatile.eth0.hwaddr"
	// waitTimeout bounds first boot because a dropped SSH session doesn't stop rye.
	waitTimeout = 30 * time.Minute
	// agentTimeout bounds the wait for the guest agent after a start.
	agentTimeout = 5 * time.Minute
	// cloudInitLog is the guest's first-boot output, and tailLines how much of it a failure shows.
	cloudInitLog = "/var/log/cloud-init-output.log"
	tailLines    = 20
	// stateTimeout is the seconds a graceful stop or restart may take.
	stateTimeout = 60
)

// guestUsers is the vendor-data, a cloud-config that creates dev.
//
//go:embed guest/users.yaml
var guestUsers string

//go:embed guest/setup.sh
var guestSetupTemplate string

// guestSSHD allows key logins for dev, with the keys from keys.int.rye.
var guestSSHD = vmconfig.SSHD("dev", "AuthorizedKeysCommand /usr/local/sbin/rye-keys", "AuthorizedKeysCommandUser nobody")

// guestSetup is the setup script with the shared files in place.
var guestSetup = strings.NewReplacer(
	"@SSHD@", strings.TrimSuffix(guestSSHD, "\n"),
	"@AUTO_UPGRADES_PATH@", vmconfig.AutoUpgradesPath,
	"@AUTO_UPGRADES@", strings.TrimSuffix(vmconfig.AutoUpgrades, "\n"),
).Replace(guestSetupTemplate)

// Config locates the Incus server, the project that holds VMs, and their storage pool and bridge.
type Config struct {
	URL     string `json:"url"`
	Project string `json:"project"`
	Pool    string `json:"pool"`
	Bridge  string `json:"bridge"`
}

// Provider runs VMs on an Incus server.
type Provider struct {
	api    *client
	pool   string
	bridge string
}

// Connect prepares a connection using the Config in the file at path.
func Connect(path string) (*Provider, error) {
	data, err := os.ReadFile(filepath.Clean(path))
	if err != nil {
		return nil, err
	}
	var cfg Config
	if err := json.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	api, err := newClient(cfg)
	if err != nil {
		return nil, err
	}
	return &Provider{api: api, pool: cfg.Pool, bridge: cfg.Bridge}, nil
}

// Create builds and starts a VM, and returns once cloud-init has finished and rye's setup has run.
func (p *Provider) Create(ctx context.Context, spec compute.Spec, out io.Writer) (compute.VM, error) {
	progress := func(step string) { _, _ = fmt.Fprintf(out, "%s...\n", step) }

	progress("Creating VM")
	// A change to a cloud-init.* key gives the VM a new instance-id, and cloud-init then provisions it again on its
	// next boot, with new SSH host keys. rye sets these keys only here.
	config := map[string]string{
		"limits.cpu":             strconv.Itoa(spec.CPU),
		"limits.memory":          spec.Memory,
		"cloud-init.vendor-data": guestUsers,
	}
	if spec.CloudInit != "" {
		config["cloud-init.user-data"] = spec.CloudInit
	}
	err := p.api.request(ctx, http.MethodPost, "/1.0/instances", map[string]any{
		"name":   spec.Name,
		"type":   "virtual-machine",
		"start":  true,
		"source": imageSource(),
		"config": config,
		"devices": map[string]map[string]string{
			"root": {"type": "disk", "path": "/", "pool": p.pool, "size": spec.Disk},
		},
	}, nil)
	if err != nil {
		return compute.VM{}, err
	}

	progress("Waiting for first boot")
	booted := p.WaitForCloudInit(ctx, spec.Name, out)
	// The setup runs after failed user-data too, because without it the VM takes no SSH logins.
	progress("Setting up")
	if err := errors.Join(booted, p.setup(ctx, spec.Name)); err != nil {
		return compute.VM{}, err
	}
	err = p.api.request(ctx, http.MethodPatch, instancePath(spec.Name), map[string]any{"config": map[string]string{readyKey: "true"}}, nil)
	if err != nil {
		return compute.VM{}, err
	}
	return p.Get(ctx, spec.Name)
}

// List returns every VM in the project.
func (p *Provider) List(ctx context.Context) ([]compute.VM, error) {
	var instances []instance
	if err := p.api.request(ctx, http.MethodGet, "/1.0/instances?recursion=1", nil, &instances); err != nil {
		return nil, err
	}
	vms := make([]compute.VM, 0, len(instances))
	for i := range instances {
		vms = append(vms, toVM(&instances[i]))
	}
	return vms, nil
}

// Get returns one VM.
func (p *Provider) Get(ctx context.Context, name string) (compute.VM, error) {
	var found instance
	if err := p.api.request(ctx, http.MethodGet, instancePath(name), nil, &found); err != nil {
		return compute.VM{}, err
	}
	return toVM(&found), nil
}

// Delete removes a VM. The VM may already be gone after a failed create.
func (p *Provider) Delete(ctx context.Context, name string) error {
	var found instance
	err := p.api.request(ctx, http.MethodGet, instancePath(name), nil, &found)
	if errors.Is(err, compute.ErrNotFound) {
		return nil
	}
	if err != nil {
		return err
	}
	if found.StatusCode != statusStopped {
		if err := p.setState(ctx, name, "stop", true); err != nil {
			return err
		}
	}
	return p.api.request(ctx, http.MethodDelete, instancePath(name), nil, nil)
}

// Start boots a stopped VM, waits for its guest agent, and runs rye's setup in it.
func (p *Provider) Start(ctx context.Context, name string) error {
	if err := p.setState(ctx, name, "start", false); err != nil {
		return err
	}
	return p.setupAfterBoot(ctx, name)
}

// setupAfterBoot waits for a booting VM's guest agent, then runs the setup.
func (p *Provider) setupAfterBoot(ctx context.Context, name string) error {
	wait, cancel := context.WithTimeout(ctx, agentTimeout)
	defer cancel()
	for {
		_, err := p.Run(wait, name, compute.Command{Args: []string{"true"}})
		if err == nil {
			break
		}
		select {
		case <-wait.Done():
			return fmt.Errorf("waiting for the guest agent in %s: %w", name, errors.Join(wait.Err(), err))
		case <-time.After(time.Second):
		}
	}
	return p.setup(ctx, name)
}

// setup runs the setup script as root in a running VM.
func (p *Provider) setup(ctx context.Context, name string) error {
	result, err := p.Run(ctx, name, compute.Command{Args: []string{"sh", "-c", guestSetup}})
	if err != nil {
		return err
	}
	if result.Code != 0 {
		return fmt.Errorf("setup in %s exited %d: %s", name, result.Code, strings.TrimSpace(result.Stderr))
	}
	return nil
}

// Stop shuts a VM down.
func (p *Provider) Stop(ctx context.Context, name string) error {
	return p.setState(ctx, name, "stop", false)
}

// Restart reboots a VM, waits for its guest agent, and runs rye's setup in it.
func (p *Provider) Restart(ctx context.Context, name string) error {
	if err := p.setState(ctx, name, "restart", false); err != nil {
		return err
	}
	return p.setupAfterBoot(ctx, name)
}

// MarkAgentManager records that agent-manager is installed, in the VM's config.
func (p *Provider) MarkAgentManager(ctx context.Context, name string) error {
	return p.api.request(ctx, http.MethodPatch, instancePath(name), map[string]any{"config": map[string]string{agentManagerKey: "true"}}, nil)
}

// Resize changes a stopped VM's sizes.
func (p *Provider) Resize(ctx context.Context, name string, size compute.Size) error {
	var found instance
	if err := p.api.request(ctx, http.MethodGet, instancePath(name), nil, &found); err != nil {
		return err
	}
	if found.StatusCode != statusStopped {
		return fmt.Errorf("stop %s before resizing it", name)
	}
	config := map[string]string{}
	if size.CPU != 0 {
		config["limits.cpu"] = strconv.Itoa(size.CPU)
	}
	if size.Memory != "" {
		config["limits.memory"] = size.Memory
	}
	patch := map[string]any{"config": config}
	if size.Disk != "" {
		// PATCH replaces a device whole.
		found.Devices["root"]["size"] = size.Disk
		patch["devices"] = map[string]map[string]string{"root": found.Devices["root"]}
	}
	return p.api.request(ctx, http.MethodPatch, instancePath(name), patch, nil)
}

// FindByAddress returns the VM whose Incus-assigned hardware address holds the lease for address. It ignores the
// lease hostname, which the guest sets.
func (p *Provider) FindByAddress(ctx context.Context, address string) (string, error) {
	var leases []lease
	if err := p.api.request(ctx, http.MethodGet, "/1.0/networks/"+p.bridge+"/leases", nil, &leases); err != nil {
		return "", err
	}
	var instances []instance
	if err := p.api.request(ctx, http.MethodGet, "/1.0/instances?recursion=1", nil, &instances); err != nil {
		return "", err
	}
	return vmAt(address, leases, instances)
}

func vmAt(address string, leases []lease, instances []instance) (string, error) {
	for _, l := range leases {
		if l.Address != address {
			continue
		}
		for i := range instances {
			if instances[i].Config[hwaddrKey] == l.Hwaddr {
				return instances[i].Name, nil
			}
		}
	}
	return "", fmt.Errorf("no VM at %s", address)
}

func (p *Provider) setState(ctx context.Context, name, action string, force bool) error {
	return p.api.request(ctx, http.MethodPut, instancePath(name)+"/state", map[string]any{"action": action, "force": force, "timeout": stateTimeout}, nil)
}

// WaitForCloudInit retries until the guest agent answers, then waits for cloud-init to finish. Meanwhile it copies
// new lines of the guest's cloud-init output log to out, indented. A failure includes the log's last lines.
func (p *Provider) WaitForCloudInit(ctx context.Context, name string, out io.Writer) error {
	ctx, cancel := context.WithTimeout(ctx, waitTimeout)
	defer cancel()
	log := &bootLog{out: out}
	done := make(chan struct{})
	following := make(chan struct{})
	go func() {
		defer close(following)
		for {
			if data, err := p.api.file(ctx, name, cloudInitLog); err == nil {
				log.update(data)
			}
			select {
			case <-done:
				return
			case <-time.After(2 * time.Second):
			}
		}
	}()
	recoverable, err := p.waitForCloudInitStatus(ctx, name)
	close(done)
	<-following
	if data, readErr := p.api.file(ctx, name, cloudInitLog); readErr == nil {
		log.update(data)
	}
	if err != nil && log.tail() != "" {
		return fmt.Errorf("%w; last lines of %s:\n%s", err, cloudInitLog, log.tail())
	}
	if err != nil {
		return err
	}
	if recoverable {
		_, _ = fmt.Fprintf(out, "  cloud-init: done with recoverable errors, listed in %s\n", cloudInitLog)
	} else {
		_, _ = fmt.Fprintln(out, "  cloud-init: done")
	}
	return nil
}

func (p *Provider) waitForCloudInitStatus(ctx context.Context, name string) (recoverable bool, err error) {
	for {
		result, err := p.Run(ctx, name, compute.Command{Args: []string{"cloud-init", "status", "--wait"}})
		if err == nil {
			if result.Code == 1 {
				return false, errors.New("cloud-init failed")
			}
			// Exit code 2 reports recoverable errors on a VM that booted.
			return result.Code == 2, nil
		}
		select {
		case <-ctx.Done():
			return false, fmt.Errorf("waiting for %s: %w", name, errors.Join(ctx.Err(), err))
		case <-time.After(time.Second):
		}
	}
}

// bootLog copies complete lines that a growing log gains between reads.
type bootLog struct {
	out     io.Writer
	printed int
	lines   []string
}

func (l *bootLog) update(data []byte) {
	if len(data) < l.printed {
		l.printed = 0
	}
	end := bytes.LastIndexByte(data, '\n') + 1
	if end <= l.printed {
		return
	}
	for _, line := range strings.Split(string(data[l.printed:end-1]), "\n") {
		_, _ = fmt.Fprintf(l.out, "  %s\n", line)
		l.lines = append(l.lines, line)
	}
	if extra := len(l.lines) - tailLines; extra > 0 {
		l.lines = l.lines[extra:]
	}
	l.printed = end
}

func (l *bootLog) tail() string {
	return strings.Join(l.lines, "\n")
}

// Push writes a file into a VM through the Incus guest agent.
func (p *Provider) Push(ctx context.Context, name string, file compute.File) error {
	return p.api.push(ctx, name, file)
}

// Run runs a command in a VM through the Incus guest agent. Incus records its output in log files, which Run reads
// and then deletes.
func (p *Provider) Run(ctx context.Context, name string, command compute.Command) (compute.Result, error) {
	var result struct {
		Return *int              `json:"return"`
		Output map[string]string `json:"output"`
	}
	err := p.api.request(ctx, http.MethodPost, instancePath(name)+"/exec", map[string]any{
		"command":            command.Args,
		"user":               command.UID,
		"group":              command.GID,
		"cwd":                command.Dir,
		"environment":        command.Env,
		"interactive":        false,
		"wait-for-websocket": false,
		"record-output":      true,
	}, &result)
	if err != nil {
		return compute.Result{}, err
	}
	if result.Return == nil {
		return compute.Result{}, errors.New("exec returned no exit code")
	}
	run := compute.Result{Code: *result.Return}
	for stream, log := range result.Output {
		data, err := p.api.raw(ctx, log)
		if err != nil {
			return run, fmt.Errorf("reading exec output %s: %w", log, err)
		}
		switch stream {
		case "1":
			run.Stdout = string(data)
		case "2":
			run.Stderr = string(data)
		}
		if err := p.api.request(ctx, http.MethodDelete, log, nil, nil); err != nil {
			return run, fmt.Errorf("deleting exec output %s: %w", log, err)
		}
	}
	return run, nil
}

// toVM describes an instance.
func toVM(found *instance) compute.VM {
	return compute.VM{
		Name:         found.Name,
		Status:       status(found.StatusCode, found.Config[readyKey] == "true"),
		AgentManager: found.Config[agentManagerKey] == "true",
		CPU:          found.Config["limits.cpu"],
		Memory:       found.Config["limits.memory"],
		Disk:         found.Devices["root"]["size"],
	}
}

func imageSource() map[string]string {
	return map[string]string{"type": "image", "alias": imageAlias, "server": imageServer, "protocol": "simplestreams"}
}

func instancePath(name string) string { return "/1.0/instances/" + url.PathEscape(name) }

func status(code int, ready bool) compute.Status {
	switch {
	case !ready:
		return compute.StatusCreating
	case code == statusRunning:
		return compute.StatusRunning
	case code == statusStopped:
		return compute.StatusStopped
	default:
		return compute.StatusUnknown
	}
}
