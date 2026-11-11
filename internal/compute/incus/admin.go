package incus

import (
	"context"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/hex"
	"encoding/pem"
	"errors"
	"fmt"
	"net/http"
	"net/url"

	"rye/internal/compute"
)

// Admin calls the Incus API through the unix socket, for what rye sets up outside project rye. It manages networks,
// network ACLs, projects, profiles, VMs in the default project and trusted certificates.
type Admin struct {
	api *client
}

// ConnectAdmin returns an Admin on the unix socket, after one call that checks access.
func ConnectAdmin(ctx context.Context, socket string) (*Admin, error) {
	a := &Admin{api: newSocketClient(socket, "default")}
	if _, err := a.Server(ctx); err != nil {
		return nil, fmt.Errorf("cannot reach Incus at %s (%w); join incus-admin, then log in again", socket, err)
	}
	return a, nil
}

// in returns a client for project.
func (a *Admin) in(project string) *client {
	return &client{http: a.api.http, baseURL: a.api.baseURL, project: project}
}

// ServerInfo is the part of GET /1.0 that rye reads.
type ServerInfo struct {
	Config      map[string]string `json:"config"`
	Environment struct {
		Certificate string `json:"certificate"`
	} `json:"environment"`
}

// Server returns the server's config and certificate.
func (a *Admin) Server(ctx context.Context) (ServerInfo, error) {
	var info ServerInfo
	err := a.api.request(ctx, http.MethodGet, "/1.0", nil, &info)
	return info, err
}

// SetServerConfig sets one server config key.
func (a *Admin) SetServerConfig(ctx context.Context, key, value string) error {
	return a.api.request(ctx, http.MethodPatch, "/1.0", map[string]any{"config": map[string]string{key: value}}, nil)
}

// PoolExists reports whether a storage pool exists.
func (a *Admin) PoolExists(ctx context.Context, pool string) (bool, error) {
	return exists(a.api.request(ctx, http.MethodGet, "/1.0/storage-pools/"+url.PathEscape(pool), nil, nil))
}

// Network returns a network's config, or nil when it doesn't exist.
func (a *Admin) Network(ctx context.Context, name string) (map[string]string, error) {
	var network struct {
		Config map[string]string `json:"config"`
	}
	err := a.api.request(ctx, http.MethodGet, "/1.0/networks/"+url.PathEscape(name), nil, &network)
	if errors.Is(err, compute.ErrNotFound) {
		return nil, nil
	}
	if network.Config == nil {
		network.Config = map[string]string{}
	}
	return network.Config, err
}

// EnsureNetwork creates a bridge with create when it's missing, then sets config on it.
func (a *Admin) EnsureNetwork(ctx context.Context, name string, create, config map[string]string) error {
	current, err := a.Network(ctx, name)
	if err != nil {
		return err
	}
	if current == nil {
		if err := a.api.request(ctx, http.MethodPost, "/1.0/networks", map[string]any{"name": name, "type": "bridge", "config": create}, nil); err != nil {
			return err
		}
	}
	return a.api.request(ctx, http.MethodPatch, "/1.0/networks/"+url.PathEscape(name), map[string]any{"config": config}, nil)
}

// Rule is one network ACL rule.
type Rule struct {
	Action          string `json:"action"`
	Source          string `json:"source,omitempty"`
	Destination     string `json:"destination,omitempty"`
	Protocol        string `json:"protocol,omitempty"`
	DestinationPort string `json:"destination_port,omitempty"`
	State           string `json:"state"`
}

// ACL is a network ACL in the default project.
type ACL struct {
	Description string `json:"description"`
	Egress      []Rule `json:"egress"`
	Ingress     []Rule `json:"ingress"`
}

// GetACL returns an ACL, or nil when it doesn't exist.
func (a *Admin) GetACL(ctx context.Context, name string) (*ACL, error) {
	var acl ACL
	err := a.api.request(ctx, http.MethodGet, "/1.0/network-acls/"+url.PathEscape(name), nil, &acl)
	if errors.Is(err, compute.ErrNotFound) {
		return nil, nil
	}
	return &acl, err
}

// PutACL creates an ACL when it's missing and replaces its rules.
func (a *Admin) PutACL(ctx context.Context, name string, acl ACL) error {
	current, err := a.GetACL(ctx, name)
	if err != nil {
		return err
	}
	if current == nil {
		if err := a.api.request(ctx, http.MethodPost, "/1.0/network-acls", map[string]any{"name": name}, nil); err != nil {
			return err
		}
	}
	return a.api.request(ctx, http.MethodPut, "/1.0/network-acls/"+url.PathEscape(name), acl, nil)
}

// DeleteACL deletes an ACL. A missing one is no error.
func (a *Admin) DeleteACL(ctx context.Context, name string) error {
	return missingOK(a.api.request(ctx, http.MethodDelete, "/1.0/network-acls/"+url.PathEscape(name), nil, nil))
}

// EnsureProject creates a project when it's missing, then sets config on it.
func (a *Admin) EnsureProject(ctx context.Context, name string, config map[string]string) error {
	found, err := exists(a.api.request(ctx, http.MethodGet, "/1.0/projects/"+url.PathEscape(name), nil, nil))
	if err != nil {
		return err
	}
	if !found {
		if err := a.api.request(ctx, http.MethodPost, "/1.0/projects", map[string]any{"name": name}, nil); err != nil {
			return err
		}
	}
	return a.api.request(ctx, http.MethodPatch, "/1.0/projects/"+url.PathEscape(name), map[string]any{"config": config}, nil)
}

// PutProfile replaces a profile in project.
func (a *Admin) PutProfile(ctx context.Context, project, name, description string, devices map[string]map[string]string) error {
	return a.in(project).request(ctx, http.MethodPut, "/1.0/profiles/"+url.PathEscape(name),
		map[string]any{"description": description, "config": map[string]string{}, "devices": devices}, nil)
}

// Instance is a VM in the default project as Admin reads it.
type Instance struct {
	Status  string                       `json:"status"`
	Devices map[string]map[string]string `json:"devices"`
}

// Instance returns a VM in the default project, or nil when it doesn't exist.
func (a *Admin) Instance(ctx context.Context, name string) (*Instance, error) {
	var found Instance
	err := a.api.request(ctx, http.MethodGet, instancePath(url.PathEscape(name)), nil, &found)
	if errors.Is(err, compute.ErrNotFound) {
		return nil, nil
	}
	return &found, err
}

// CreateInstance creates a stopped VM in the default project from rye's image.
func (a *Admin) CreateInstance(ctx context.Context, name string, config map[string]string, devices map[string]map[string]string) error {
	return a.api.request(ctx, http.MethodPost, "/1.0/instances", map[string]any{
		"name": name, "type": "virtual-machine", "source": imageSource(), "config": config, "devices": devices,
	}, nil)
}

// Start starts a VM in the default project.
func (a *Admin) Start(ctx context.Context, name string) error {
	return a.api.request(ctx, http.MethodPut, instancePath(url.PathEscape(name))+"/state", map[string]any{"action": "start"}, nil)
}

// DeleteInstance stops a VM in the default project and deletes it. A missing one is no error.
func (a *Admin) DeleteInstance(ctx context.Context, name string) error {
	found, err := a.Instance(ctx, name)
	if err != nil || found == nil {
		return err
	}
	if found.Status != "Stopped" {
		err := a.api.request(ctx, http.MethodPut, instancePath(url.PathEscape(name))+"/state", map[string]any{"action": "stop", "force": true}, nil)
		if err != nil {
			return err
		}
	}
	return a.api.request(ctx, http.MethodDelete, instancePath(url.PathEscape(name)), nil, nil)
}

// VM returns a provider for files and commands in a VM of the default project.
func (a *Admin) VM() *Provider {
	return &Provider{api: a.api}
}

// Certificate is a trusted client certificate.
type Certificate struct {
	Name        string `json:"name"`
	Fingerprint string `json:"fingerprint"`
}

// Certificates lists the trusted certificates.
func (a *Admin) Certificates(ctx context.Context) ([]Certificate, error) {
	var certs []Certificate
	err := a.api.request(ctx, http.MethodGet, "/1.0/certificates?recursion=1", nil, &certs)
	return certs, err
}

// TrustRestricted trusts a PEM client certificate under name, limited to projects. A trusted one is left as it is.
func (a *Admin) TrustRestricted(ctx context.Context, name string, certPEM []byte, projects []string) error {
	block, _ := pem.Decode(certPEM)
	if block == nil {
		return errors.New("the client certificate isn't PEM")
	}
	if _, err := x509.ParseCertificate(block.Bytes); err != nil {
		return err
	}
	sum := sha256.Sum256(block.Bytes)
	fingerprint := hex.EncodeToString(sum[:])
	certs, err := a.Certificates(ctx)
	if err != nil {
		return err
	}
	for _, c := range certs {
		if c.Fingerprint == fingerprint {
			return nil
		}
	}
	return a.api.request(ctx, http.MethodPost, "/1.0/certificates", map[string]any{
		"type": "client", "name": name, "certificate": base64.StdEncoding.EncodeToString(block.Bytes),
		"restricted": true, "projects": projects,
	}, nil)
}

// Distrust removes every trusted certificate named name.
func (a *Admin) Distrust(ctx context.Context, name string) error {
	certs, err := a.Certificates(ctx)
	if err != nil {
		return err
	}
	for _, c := range certs {
		if c.Name == name {
			if err := a.api.request(ctx, http.MethodDelete, "/1.0/certificates/"+c.Fingerprint, nil, nil); err != nil {
				return err
			}
		}
	}
	return nil
}

func exists(err error) (bool, error) {
	if errors.Is(err, compute.ErrNotFound) {
		return false, nil
	}
	return err == nil, err
}

func missingOK(err error) error {
	if errors.Is(err, compute.ErrNotFound) {
		return nil
	}
	return err
}
