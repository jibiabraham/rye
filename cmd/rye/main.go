// Command rye runs platform commands on the service VM.
package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"maps"
	"net/netip"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"text/tabwriter"
	"time"

	"rye/internal/agentmanager"
	"rye/internal/compute"
	"rye/internal/compute/incus"
	"rye/internal/hostkeys"
	"rye/internal/integration"
	"rye/internal/vmconfig"
)

// storeIn returns the integration store's path in stateDir.
func storeIn(stateDir string) string { return filepath.Join(stateDir, vmconfig.IntegrationsFile) }

const usage = `usage: rye <command>

  new <vm> [--cpu n] [--memory m] [--disk s] [--cloud-init < user-data] [--integration a,b] [--verbose]
  ls [--json]
  show <vm> [--json]
  start <vm>
  stop <vm>
  restart <vm>
  rm <vm> --yes
  resize <vm> [--cpu n] [--memory m] [--disk s]
  integration add <name> --url u [--header h] < header-value
  integration ls
  integration rm <name>
  integration attach <name> <vm>
  integration detach <name> <vm>
  agent-manager install <vm>|--all
  ssh-config
  help
`

func main() {
	if err := run(context.Background(), commandArgs(os.Args[1:], os.Getenv("SSH_ORIGINAL_COMMAND")), os.Stdin, os.Stdout); err != nil {
		fmt.Fprintln(os.Stderr, "rye:", err)
		os.Exit(1)
	}
}

// commandArgs returns the arguments from an SSH command when ForceCommand runs rye.
func commandArgs(args []string, sshCommand string) []string {
	if sshCommand != "" {
		return strings.Fields(sshCommand)
	}
	return args
}

func run(ctx context.Context, args []string, stdin io.Reader, stdout io.Writer) error {
	switch {
	case len(args) == 0, args[0] == "help", args[0] == "-h", args[0] == "--help":
		_, err := io.WriteString(stdout, usage)
		return err
	}
	if args[0] == "ssh-config" {
		return sshConfig(os.Getenv("SSH_CONNECTION"), stdout)
	}
	provider, err := incus.Connect(vmconfig.IncusConfigPath)
	if err != nil {
		return err
	}
	return dispatch(ctx, provider, vmconfig.StateDir, args[0], args[1:], stdin, stdout)
}

// sshConfig prints client config for this service VM. The server address is the third field of SSH_CONNECTION.
func sshConfig(connection string, stdout io.Writer) error {
	fields := strings.Fields(connection)
	if len(fields) != 4 {
		return errors.New("ssh-config runs over SSH")
	}
	service, err := netip.ParseAddr(fields[2])
	if err != nil || !service.Is4() {
		return fmt.Errorf("ssh-config needs an IPv4 server address, got %q", fields[2])
	}
	_, err = io.WriteString(stdout, vmconfig.SSHConfig(service))
	return err
}

// getVM returns the named VM, with a hint when there is none.
func getVM(ctx context.Context, p compute.Provider, name string) (compute.VM, error) {
	vm, err := p.Get(ctx, name)
	return vm, noVM(name, err)
}

// noVM gives a not-found error for the VM name a hint, and returns other errors as they are.
func noVM(name string, err error) error {
	if errors.Is(err, compute.ErrNotFound) {
		return fmt.Errorf("no VM %s; ssh rye ls lists them", name)
	}
	return err
}

// parse parses args into flags, which may come before or after the positional arguments, and returns those
// arguments.
func parse(flags *flag.FlagSet, args []string) ([]string, error) {
	var positional []string
	for {
		if err := flags.Parse(args); err != nil {
			return nil, err
		}
		args = flags.Args()
		if len(args) == 0 {
			return positional, nil
		}
		positional = append(positional, args[0])
		args = args[1:]
	}
}

// dispatch runs a command.
func dispatch(ctx context.Context, p compute.Provider, stateDir, command string, args []string, stdin io.Reader, stdout io.Writer) error {
	switch command {
	case "new":
		return newVM(ctx, p, stateDir, args, stdin, stdout)
	case "ls":
		return list(ctx, p, args, stdout)
	case "show":
		return show(ctx, p, args, stdout)
	case "start":
		return boot(ctx, p, "start", args, p.Start, stdout)
	case "stop":
		return withName(ctx, "stop", args, p.Stop)
	case "restart":
		return boot(ctx, p, "restart", args, p.Restart, stdout)
	case "rm":
		return remove(ctx, p, stateDir, args, stdout)
	case "resize":
		return resize(ctx, p, args)
	case "integration":
		return integrationCommand(ctx, p, stateDir, args, stdin, stdout)
	case "agent-manager":
		return agentManagerCommand(ctx, p, args, stdout)
	default:
		return fmt.Errorf("unknown command %q\n%s", command, usage)
	}
}

func newVM(ctx context.Context, p compute.Provider, stateDir string, args []string, stdin io.Reader, stdout io.Writer) error {
	var spec compute.Spec
	flags := flag.NewFlagSet("new", flag.ContinueOnError)
	flags.IntVar(&spec.CPU, "cpu", 2, "CPU count")
	flags.StringVar(&spec.Memory, "memory", "4GiB", "memory")
	flags.StringVar(&spec.Disk, "disk", "30GiB", "disk size")
	cloudInit := flags.Bool("cloud-init", false, "read user-data from stdin")
	verbose := flags.Bool("verbose", false, "show every line of the guest's first-boot output")
	integrations := flags.String("integration", "", "integrations to attach, comma separated")
	names, err := parse(flags, args)
	if err != nil {
		return err
	}
	if len(names) != 1 {
		return errors.New("new needs a VM name")
	}
	spec.Name = names[0]
	if *cloudInit {
		data, err := io.ReadAll(stdin)
		if err != nil {
			return err
		}
		spec.CloudInit = string(data)
	}
	switch _, err := p.Get(ctx, spec.Name); {
	case err == nil:
		return fmt.Errorf("%s already exists", spec.Name)
	case !errors.Is(err, compute.ErrNotFound):
		return err
	}
	if err := replaceAttachments(stateDir, spec.Name, *integrations); err != nil {
		return err
	}
	started := time.Now()
	out := bootOutput(stdout, *verbose)
	vm, err := p.Create(ctx, spec, out)
	out.Flush()
	if err != nil {
		return err
	}
	_, err = fmt.Fprintf(stdout, "Ready: %s in %s\n", vm.Name, elapsed(started))
	return err
}

func list(ctx context.Context, p compute.Provider, args []string, stdout io.Writer) error {
	asJSON, _, err := parseOutput("ls", args)
	if err != nil {
		return err
	}
	vms, err := p.List(ctx)
	if err != nil {
		return err
	}
	if asJSON {
		return json.NewEncoder(stdout).Encode(vms)
	}
	var rows strings.Builder
	rows.WriteString("NAME\tSTATUS\tCPU\tMEMORY\tDISK\n")
	for _, vm := range vms {
		fmt.Fprintf(&rows, "%s\t%s\t%s\t%s\t%s\n", vm.Name, vm.Status, vm.CPU, vm.Memory, vm.Disk)
	}
	return writeTable(stdout, rows.String())
}

func writeTable(stdout io.Writer, rows string) error {
	table := tabwriter.NewWriter(stdout, 0, 0, 2, ' ', 0)
	if _, err := io.WriteString(table, rows); err != nil {
		return err
	}
	return table.Flush()
}

func show(ctx context.Context, p compute.Provider, args []string, stdout io.Writer) error {
	asJSON, rest, err := parseOutput("show", args)
	if err != nil {
		return err
	}
	if len(rest) != 1 {
		return errors.New("show needs a VM name")
	}
	vm, err := getVM(ctx, p, rest[0])
	if err != nil {
		return err
	}
	if asJSON {
		return json.NewEncoder(stdout).Encode(vm)
	}
	_, err = fmt.Fprintf(stdout, "name: %s\nstatus: %s\ncpu: %s\nmemory: %s\ndisk: %s\n",
		vm.Name, vm.Status, vm.CPU, vm.Memory, vm.Disk)
	return err
}

func parseOutput(command string, args []string) (bool, []string, error) {
	flags := flag.NewFlagSet(command, flag.ContinueOnError)
	asJSON := flags.Bool("json", false, "JSON output")
	rest, err := parse(flags, args)
	return *asJSON, rest, err
}

// boot starts or restarts a VM with action, which runs rye's setup in it, and installs agent-manager in a VM that
// has it.
func boot(ctx context.Context, p compute.Provider, command string, args []string,
	action func(context.Context, string) error, stdout io.Writer) error {
	if len(args) != 1 {
		return fmt.Errorf("%s needs a VM name", command)
	}
	vm, err := getVM(ctx, p, args[0])
	if err != nil {
		return err
	}
	if err := action(ctx, vm.Name); err != nil {
		return err
	}
	if !vm.AgentManager {
		return nil
	}
	if err := installAgentManager(ctx, p, vm.Name, stdout); err != nil {
		return fmt.Errorf("%s is running, and installing agent-manager failed: %w", vm.Name, err)
	}
	return nil
}

// withName runs action on the VM that args name.
func withName(ctx context.Context, command string, args []string, action func(context.Context, string) error) error {
	if len(args) != 1 {
		return fmt.Errorf("%s needs a VM name", command)
	}
	return noVM(args[0], action(ctx, args[0]))
}

// stageLine matches cloud-init's stage lines, its verdict, warnings and errors, the first-boot lines shown without --verbose.
var stageLine = regexp.MustCompile(`Cloud-init v\. .* (running|finished)|^cloud-init:|(?i)\b(warn|warning|error|errors|fail|failed|failure)\b`)

// filterWriter passes rye's own lines, and indented guest lines that keep matches.
type filterWriter struct {
	out     io.Writer
	keep    func(string) bool
	pending []byte
}

func bootOutput(out io.Writer, verbose bool) *filterWriter {
	keep := func(line string) bool {
		return verbose || !strings.HasPrefix(line, "  ") || stageLine.MatchString(strings.TrimSpace(line))
	}
	return &filterWriter{out: out, keep: keep}
}

func (w *filterWriter) Write(p []byte) (int, error) {
	w.pending = append(w.pending, p...)
	for {
		end := bytes.IndexByte(w.pending, '\n')
		if end < 0 {
			return len(p), nil
		}
		line := string(w.pending[:end])
		w.pending = w.pending[end+1:]
		if w.keep(line) {
			if _, err := io.WriteString(w.out, line+"\n"); err != nil {
				return len(p), err
			}
		}
	}
}

// Flush writes a final line that has no newline.
func (w *filterWriter) Flush() {
	if len(w.pending) > 0 && w.keep(string(w.pending)) {
		_, _ = w.out.Write(append(w.pending, '\n'))
	}
	w.pending = nil
}

func elapsed(since time.Time) string {
	return time.Since(since).Round(time.Second).String()
}

func resize(ctx context.Context, p compute.Provider, args []string) error {
	var size compute.Size
	flags := flag.NewFlagSet("resize", flag.ContinueOnError)
	flags.IntVar(&size.CPU, "cpu", 0, "CPU count")
	flags.StringVar(&size.Memory, "memory", "", "memory")
	flags.StringVar(&size.Disk, "disk", "", "disk size")
	names, err := parse(flags, args)
	if err != nil {
		return err
	}
	if len(names) != 1 {
		return errors.New("resize needs a VM name")
	}
	return noVM(names[0], p.Resize(ctx, names[0], size))
}

func remove(ctx context.Context, p compute.Provider, stateDir string, args []string, stdout io.Writer) error {
	name, err := confirmedName(flag.NewFlagSet("rm", flag.ContinueOnError), "permanently deletes %s and its disk", args)
	if err != nil {
		return err
	}
	_, err = p.Get(ctx, name)
	missing := errors.Is(err, compute.ErrNotFound)
	if err != nil && !missing {
		return err
	}
	if err := p.Delete(ctx, name); err != nil {
		return err
	}
	// A new VM with the same name has a different host key.
	if err := hostkeys.Remove(filepath.Join(stateDir, vmconfig.KnownHostsFile), name+".rye"); err != nil {
		return err
	}
	store, err := integration.Load(storeIn(stateDir))
	if err != nil {
		return err
	}
	if err := store.Replace(name, nil); err != nil {
		return err
	}
	removed := store.RemoveTargeting(name + ".rye")
	if err := store.Save(storeIn(stateDir)); err != nil {
		return err
	}
	var out strings.Builder
	for _, integration := range removed {
		out.WriteString("Removed integration " + integration + ", which pointed at " + name + "\n")
	}
	if missing {
		out.WriteString("No VM " + name + "; removed its host key and integrations\n")
	} else {
		out.WriteString("Removed " + name + "\n")
	}
	_, err = io.WriteString(stdout, out.String())
	return err
}

// agentManagerCommand installs agent-manager in a VM, or with --all in every running VM that has it.
func agentManagerCommand(ctx context.Context, p compute.Provider, args []string, stdout io.Writer) error {
	usage := errors.New("agent-manager needs install and a VM name, or install --all")
	if len(args) == 0 || args[0] != "install" {
		return usage
	}
	flags := flag.NewFlagSet("agent-manager install", flag.ContinueOnError)
	all := flags.Bool("all", false, "every running VM that has agent-manager")
	names, err := parse(flags, args[1:])
	if err != nil {
		return err
	}
	if *all == (len(names) == 1) || len(names) > 1 {
		return usage
	}
	if !*all {
		vm, err := getVM(ctx, p, names[0])
		if err != nil {
			return err
		}
		if vm.Status != compute.StatusRunning {
			return fmt.Errorf("%s is %s; ssh rye start %s", vm.Name, vm.Status, vm.Name)
		}
		return installAgentManager(ctx, p, vm.Name, stdout)
	}
	vms, err := p.List(ctx)
	if err != nil {
		return err
	}
	var errs []error
	for _, vm := range vms {
		switch {
		case !vm.AgentManager:
		case vm.Status != compute.StatusRunning:
			if _, err := fmt.Fprintf(stdout, "Skipped %s, which is %s\n", vm.Name, vm.Status); err != nil {
				return err
			}
		default:
			if _, err := fmt.Fprintf(stdout, "%s: ", vm.Name); err != nil {
				return err
			}
			if err := installAgentManager(ctx, p, vm.Name, stdout); err != nil {
				// The error ends the VM's line, and the command's error lists it again.
				if _, werr := fmt.Fprintln(stdout, err); werr != nil {
					return werr
				}
				errs = append(errs, fmt.Errorf("%s: %w", vm.Name, err))
			}
		}
	}
	return errors.Join(errs...)
}

// installAgentManager installs agent-manager in a running VM and marks the VM.
func installAgentManager(ctx context.Context, p compute.Provider, name string, stdout io.Writer) error {
	if err := agentmanager.Install(ctx, p, name, vmconfig.GuestUser, stdout); err != nil {
		return err
	}
	return p.MarkAgentManager(ctx, name)
}

// replaceAttachments sets a VM's attachments from a comma-separated list.
func replaceAttachments(stateDir, vm, names string) error {
	store, err := integration.Load(storeIn(stateDir))
	if err != nil {
		return err
	}
	var list []string
	if names != "" {
		list = strings.Split(names, ",")
	}
	if err := store.Replace(vm, list); err != nil {
		return err
	}
	return store.Save(storeIn(stateDir))
}

func integrationCommand(ctx context.Context, p compute.Provider, stateDir string, args []string, stdin io.Reader, stdout io.Writer) error {
	if len(args) == 0 {
		return errors.New("integration needs add, ls, rm, attach or detach")
	}
	store, err := integration.Load(storeIn(stateDir))
	if err != nil {
		return err
	}
	switch args[0] {
	case "ls":
		return listIntegrations(store, stdout)
	case "add":
		err = addIntegration(store, args[1:], stdin)
	case "rm":
		if len(args) != 2 {
			return errors.New("integration rm needs a name")
		}
		err = store.Remove(args[1])
	case "attach":
		if len(args) != 3 {
			return errors.New("integration attach needs an integration and a VM")
		}
		if _, err := getVM(ctx, p, args[2]); err != nil {
			return err
		}
		err = store.Attach(args[1], args[2])
	case "detach":
		if len(args) != 3 {
			return errors.New("integration detach needs an integration and a VM")
		}
		err = store.Detach(args[1], args[2])
	default:
		return fmt.Errorf("unknown integration command %q", args[0])
	}
	if err != nil {
		return err
	}
	return store.Save(storeIn(stateDir))
}

func addIntegration(store *integration.Store, args []string, stdin io.Reader) error {
	flags := flag.NewFlagSet("integration add", flag.ContinueOnError)
	target := flags.String("url", "", "service URL")
	header := flags.String("header", "Authorization", "header to add")
	names, err := parse(flags, args)
	if err != nil {
		return err
	}
	if len(names) != 1 {
		return errors.New("integration add needs a name")
	}
	if *target == "" {
		return errors.New("integration add needs --url")
	}
	data, err := io.ReadAll(stdin)
	if err != nil {
		return err
	}
	value := strings.TrimSpace(string(data))
	if value == "" {
		return errors.New("integration add reads the header value from stdin")
	}
	return store.Add(names[0], integration.Integration{URL: *target, Header: *header, Value: value})
}

func listIntegrations(store *integration.Store, stdout io.Writer) error {
	var rows strings.Builder
	rows.WriteString("NAME\tURL\tHEADER\tVMS\n")
	for _, name := range slices.Sorted(maps.Keys(store.Integrations)) {
		var vms []string
		for _, vm := range slices.Sorted(maps.Keys(store.Attachments)) {
			if store.Attached(name, vm) {
				vms = append(vms, vm)
			}
		}
		fmt.Fprintf(&rows, "%s\t%s\t%s\t%s\n", name, store.Integrations[name].URL, store.Integrations[name].Header, strings.Join(vms, ","))
	}
	return writeTable(stdout, rows.String())
}

// confirmedName parses args with flags and a --yes flag that it adds, and returns the VM name. args must hold --yes.
func confirmedName(flags *flag.FlagSet, warning string, args []string) (string, error) {
	yes := flags.Bool("yes", false, "confirm")
	names, err := parse(flags, args)
	if err != nil {
		return "", err
	}
	if len(names) != 1 {
		return "", fmt.Errorf("%s needs a VM name", flags.Name())
	}
	if !*yes {
		return "", fmt.Errorf(flags.Name()+" "+warning+"; pass --yes to confirm", names[0])
	}
	return names[0], nil
}
