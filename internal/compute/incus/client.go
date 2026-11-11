package incus

import (
	"bytes"
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"rye/internal/vmconfig"

	"rye/internal/compute"
)

// Incus status codes.
const (
	statusSuccess = 200
	statusRunning = 103
	statusStopped = 102
)

// instance holds the fields read from an Incus instance.
type instance struct {
	Name       string                       `json:"name"`
	StatusCode int                          `json:"status_code"`
	Config     map[string]string            `json:"config"`
	Devices    map[string]map[string]string `json:"devices"`
}

// lease is a DHCP lease on the bridge.
type lease struct {
	Address string `json:"address"`
	Hwaddr  string `json:"hwaddr"`
}

// client calls the Incus REST API.
type client struct {
	http    *http.Client
	baseURL string
	project string
}

// response is the envelope around every Incus reply.
type response struct {
	Type      string          `json:"type"`
	Error     string          `json:"error"`
	Operation string          `json:"operation"`
	Metadata  json.RawMessage `json:"metadata"`
}

func newClient(cfg Config) (*client, error) {
	pair, err := tls.LoadX509KeyPair(vmconfig.ClientCertPath, vmconfig.ClientKeyPath)
	if err != nil {
		return nil, err
	}
	server, err := readCertificate(vmconfig.ServerCertPath)
	if err != nil {
		return nil, err
	}
	config, err := serverTLS(server, pair)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", vmconfig.ServerCertPath, err)
	}
	return &client{http: &http.Client{Transport: &http.Transport{TLSClientConfig: config}}, baseURL: cfg.URL,
		project: cfg.Project}, nil
}

// serverTLS trusts only the Incus server's own certificate. The certificate doesn't name the bridge address that
// ryed dials. ryed checks it under the first DNS name it lists.
func serverTLS(server *x509.Certificate, client tls.Certificate) (*tls.Config, error) {
	if len(server.DNSNames) == 0 {
		return nil, errors.New("the Incus server certificate lists no DNS name to check it under")
	}
	roots := x509.NewCertPool()
	roots.AddCert(server)
	return &tls.Config{
		MinVersion:   tls.VersionTLS13,
		Certificates: []tls.Certificate{client},
		RootCAs:      roots,
		ServerName:   server.DNSNames[0],
	}, nil
}

// SocketPath is the Incus API's unix socket. Members of incus-admin can use it.
const SocketPath = "/var/lib/incus/unix.socket"

// newSocketClient calls the API through the unix socket, in project.
func newSocketClient(socket, project string) *client {
	return &client{
		http: &http.Client{Transport: &http.Transport{
			DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
				return (&net.Dialer{}).DialContext(ctx, "unix", socket)
			},
		}},
		baseURL: "http://incus",
		project: project,
	}
}

func readCertificate(path string) (*x509.Certificate, error) {
	data, err := os.ReadFile(filepath.Clean(path))
	if err != nil {
		return nil, err
	}
	block, _ := pem.Decode(data)
	if block == nil {
		return nil, fmt.Errorf("%s holds no certificate", path)
	}
	return x509.ParseCertificate(block.Bytes)
}

// request sends a call, waits for it if Incus runs it in the background, and decodes the metadata into out.
func (c *client) request(ctx context.Context, method, path string, body, out any) error {
	reply, err := c.send(ctx, method, path, body)
	if err != nil {
		return err
	}
	if reply.Type == "async" {
		return c.wait(ctx, reply.Operation, out)
	}
	return decode(reply.Metadata, out)
}

func (c *client) wait(ctx context.Context, operation string, out any) error {
	reply, err := c.send(ctx, http.MethodGet, operation+"/wait?timeout=-1", nil)
	if err != nil {
		return err
	}
	var result struct {
		StatusCode int             `json:"status_code"`
		Err        string          `json:"err"`
		Metadata   json.RawMessage `json:"metadata"`
	}
	if err := json.Unmarshal(reply.Metadata, &result); err != nil {
		return err
	}
	if result.StatusCode != statusSuccess {
		return errors.New(result.Err)
	}
	return decode(result.Metadata, out)
}

func (c *client) send(ctx context.Context, method, path string, body any) (*response, error) {
	var payload io.Reader
	if body != nil {
		data, err := json.Marshal(body)
		if err != nil {
			return nil, err
		}
		payload = bytes.NewReader(data)
	}
	req, err := http.NewRequestWithContext(ctx, method, c.url(path), payload)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	res, err := c.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer func() { _ = res.Body.Close() }()
	var reply response
	if err := json.NewDecoder(res.Body).Decode(&reply); err != nil {
		return nil, fmt.Errorf("%s %s: %w", method, path, err)
	}
	if res.StatusCode == http.StatusNotFound {
		return nil, fmt.Errorf("%s: %w", path, compute.ErrNotFound)
	}
	if reply.Type == "error" {
		return nil, errors.New(reply.Error)
	}
	return &reply, nil
}

// file returns a file from an instance, which Incus reads through the guest agent.
func (c *client) file(ctx context.Context, instance, path string) ([]byte, error) {
	data, err := c.raw(ctx, "/1.0/instances/"+url.PathEscape(instance)+"/files?path="+url.QueryEscape(path))
	if err != nil {
		return nil, fmt.Errorf("reading %s in %s: %w", path, instance, err)
	}
	return data, nil
}

// raw returns the body of a GET that Incus answers with the bytes themselves, such as a file or an exec log.
func (c *client) raw(ctx context.Context, path string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.url(path), nil)
	if err != nil {
		return nil, err
	}
	res, err := c.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer func() { _ = res.Body.Close() }()
	if res.StatusCode != http.StatusOK {
		return nil, errors.New(res.Status)
	}
	return io.ReadAll(res.Body)
}

// push writes file into an instance through the guest agent. The body streams from file.Body.
func (c *client) push(ctx context.Context, instance string, file compute.File) error {
	path := "/1.0/instances/" + url.PathEscape(instance) + "/files?path=" + url.QueryEscape(file.Path)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.url(path), file.Body)
	if err != nil {
		return err
	}
	req.ContentLength = file.Size
	// Incus reads a form-encoded body as a form, and the file comes out empty.
	req.Header.Set("Content-Type", "application/octet-stream")
	req.Header.Set("X-Incus-type", "file")
	req.Header.Set("X-Incus-write", "overwrite")
	req.Header.Set("X-Incus-uid", strconv.Itoa(file.UID))
	req.Header.Set("X-Incus-gid", strconv.Itoa(file.GID))
	req.Header.Set("X-Incus-mode", fmt.Sprintf("%04o", file.Mode.Perm()))
	res, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer func() { _ = res.Body.Close() }()
	var reply response
	if err := json.NewDecoder(res.Body).Decode(&reply); err != nil {
		return fmt.Errorf("writing %s in %s: %s", file.Path, instance, res.Status)
	}
	if reply.Type == "error" {
		return fmt.Errorf("writing %s in %s: %s", file.Path, instance, reply.Error)
	}
	return nil
}

func (c *client) url(path string) string {
	separator := "?"
	if strings.Contains(path, "?") {
		separator = "&"
	}
	return c.baseURL + path + separator + "project=" + url.QueryEscape(c.project)
}

func decode(data json.RawMessage, out any) error {
	if out == nil || len(data) == 0 {
		return nil
	}
	return json.Unmarshal(data, out)
}
