// Package integration stores integrations and the VMs attached to them.
package integration

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"slices"

	"rye/internal/atomicfile"
)

// reserved are the names of the key endpoint and the docs site.
var reserved = []string{"keys", "docs"}

var validName = regexp.MustCompile(`^[a-z0-9]([a-z0-9-]{0,61}[a-z0-9])?$`)

// Integration is an HTTP service and the header the proxy adds to requests for it.
type Integration struct {
	URL    string `json:"url"`
	Header string `json:"header"`
	Value  string `json:"value"`
}

// Store holds integrations by name, and the integrations attached to each VM.
type Store struct {
	Integrations map[string]Integration `json:"integrations"`
	Attachments  map[string][]string    `json:"attachments"`
}

// Load reads the file at path. A missing file is an empty store.
func Load(path string) (*Store, error) {
	store := &Store{Integrations: map[string]Integration{}, Attachments: map[string][]string{}}
	data, err := os.ReadFile(filepath.Clean(path))
	if errors.Is(err, os.ErrNotExist) {
		return store, nil
	}
	if err != nil {
		return nil, err
	}
	if err := json.Unmarshal(data, store); err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	return store, nil
}

// Save replaces the file at path in one step. os.CreateTemp creates it with mode 0600.
func (s *Store) Save(path string) error {
	data, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return err
	}
	return atomicfile.Write(path, data)
}

// Add stores an integration, replacing any with the same name.
func (s *Store) Add(name string, integration Integration) error {
	if !validName.MatchString(name) || slices.Contains(reserved, name) {
		return fmt.Errorf("invalid integration name %q", name)
	}
	s.Integrations[name] = integration
	return nil
}

// Remove deletes an integration and its attachments.
func (s *Store) Remove(name string) error {
	if _, ok := s.Integrations[name]; !ok {
		return fmt.Errorf("no integration %q", name)
	}
	delete(s.Integrations, name)
	for vm := range s.Attachments {
		s.detach(name, vm)
	}
	return nil
}

// RemoveTargeting deletes each integration whose URL host is host, with its attachments, and returns their names
// in order. A VM's name can be reused, and the next VM with it must not receive the credential.
func (s *Store) RemoveTargeting(host string) []string {
	var removed []string
	for name, integration := range s.Integrations {
		if u, err := url.Parse(integration.URL); err == nil && u.Hostname() == host {
			removed = append(removed, name)
		}
	}
	slices.Sort(removed)
	for _, name := range removed {
		_ = s.Remove(name)
	}
	return removed
}

// Attach lets a VM use an integration.
func (s *Store) Attach(name, vm string) error {
	if _, ok := s.Integrations[name]; !ok {
		return fmt.Errorf("no integration %q", name)
	}
	if !s.Attached(name, vm) {
		s.Attachments[vm] = append(s.Attachments[vm], name)
	}
	return nil
}

// Detach stops a VM using an integration. The VM need not be attached.
func (s *Store) Detach(name, vm string) error {
	if _, ok := s.Integrations[name]; !ok {
		return fmt.Errorf("no integration %q", name)
	}
	s.detach(name, vm)
	return nil
}

func (s *Store) detach(name, vm string) {
	s.Attachments[vm] = slices.DeleteFunc(s.Attachments[vm], func(attached string) bool { return attached == name })
	if len(s.Attachments[vm]) == 0 {
		delete(s.Attachments, vm)
	}
}

// Attached reports whether a VM may use an integration.
func (s *Store) Attached(name, vm string) bool {
	return slices.Contains(s.Attachments[vm], name)
}

// Replace sets a VM's attachments to names. A new VM calls this, because a reused name must not inherit access.
func (s *Store) Replace(vm string, names []string) error {
	delete(s.Attachments, vm)
	for _, name := range names {
		if err := s.Attach(name, vm); err != nil {
			return err
		}
	}
	return nil
}
