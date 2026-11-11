// Command ryed serves guests the key endpoint and their attached integrations, and serves the GUI's API on localhost.
// With -local <name>, it serves only the GUI and API for agent-manager on its own machine, listed as the VM <name>, and
// with -idle it powers that machine off after a time without agent-manager activity. The admin VM runs it that way.
package main

import (
	"context"
	"flag"
	"log"
	"log/slog"
	"net/http"
	"os"
	"time"

	"rye/internal/agentmanager"
	"rye/internal/api"
	"rye/internal/compute/incus"
	"rye/internal/docs"
	"rye/internal/proxy"
	"rye/internal/vmconfig"
	"rye/web"
)

const (
	// controlDir is ryed's RuntimeDirectory, for SSH control sockets.
	controlDir = "/run/ryed"
	// apiAddress takes browsers through an SSH tunnel. Guests reach port 80 only.
	apiAddress = "127.0.0.1:8080"
	// tailnetAddress takes browsers through tailscale serve.
	tailnetAddress = "127.0.0.1:8081"
)

func main() {
	local := flag.String("local", "", "serve only the GUI for this machine's agent-manager, listed as the VM `name`")
	idle := flag.Duration("idle", 0, "with -local, power the machine off after this long without agent-manager activity; 0 never")
	flag.Parse()
	// journald stamps each line with the time.
	slog.SetDefault(slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{
		ReplaceAttr: func(groups []string, a slog.Attr) slog.Attr {
			if a.Key == slog.TimeKey && len(groups) == 0 {
				return slog.Attr{}
			}
			return a
		},
	})))
	if *local != "" {
		if *idle > 0 {
			go powerOffWhenIdle(context.Background(), newIdleWatch(*idle, dialLocal))
		}
		gui := &http.Server{Addr: apiAddress, Handler: api.New(localProvider{name: *local}, dialLocal, web.Files()), ReadHeaderTimeout: 5 * time.Second}
		log.Fatal(gui.ListenAndServe())
	}
	provider, err := incus.Connect(vmconfig.IncusConfigPath)
	if err != nil {
		log.Fatal(err)
	}
	knownHosts := vmconfig.KnownHostsPath
	dial := agentmanager.SSH(knownHosts, controlDir)
	guests := &http.Server{
		Addr: ":80",
		Handler: &proxy.Handler{
			KeysPath: vmconfig.RyeKeysPath, Docs: &docs.Handler{Source: vmconfig.SourcePath}, StorePath: vmconfig.IntegrationsPath,
			FindVM: provider.FindByAddress, DialVM: agentmanager.SSHPort(knownHosts, controlDir),
		},
		ReadHeaderTimeout: 5 * time.Second,
	}
	handler := api.New(provider, dial, web.Files())
	gui := &http.Server{
		Addr:              apiAddress,
		Handler:           handler,
		ReadHeaderTimeout: 5 * time.Second,
	}
	go func() { log.Fatal(gui.ListenAndServe()) }()
	// make tailscale sets both.
	if host, login := os.Getenv("RYE_TAILNET_HOST"), os.Getenv("RYE_TAILNET_LOGIN"); host != "" && login != "" {
		tailnet := &http.Server{
			Addr:              tailnetAddress,
			Handler:           handler.Tailnet(host, login),
			ReadHeaderTimeout: 5 * time.Second,
		}
		go func() { log.Fatal(tailnet.ListenAndServe()) }()
	}
	log.Fatal(guests.ListenAndServe())
}
