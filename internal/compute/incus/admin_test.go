package incus

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/hex"
	"encoding/pem"
	"io"
	"math/big"
	"net/http"
	"slices"
	"strings"
	"testing"
	"time"
)

// recordAPI answers GETs from found, 404s the rest, and records every request.
func recordAPI(t *testing.T, found map[string]string) (*Admin, *[]string) {
	t.Helper()
	var calls []string
	c := testClient(t, func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		calls = append(calls, strings.TrimSpace(r.Method+" "+r.URL.Path+" "+string(body)))
		if r.Method == http.MethodGet {
			if reply, ok := found[r.URL.Path]; ok {
				_, _ = io.WriteString(w, `{"type":"sync","metadata":`+reply+`}`)
				return
			}
			w.WriteHeader(http.StatusNotFound)
			_, _ = io.WriteString(w, `{"type":"error","error":"not found"}`)
			return
		}
		_, _ = io.WriteString(w, `{"type":"sync","metadata":{}}`)
	})
	return &Admin{api: c}, &calls
}

func TestAdmin_EnsureNetwork(t *testing.T) {
	tests := []struct {
		name   string
		found  map[string]string
		create map[string]string
		want   []string
	}{
		{
			name:   "missing bridge",
			create: map[string]string{"ipv4.address": "x"},
			want: []string{
				"GET /1.0/networks/br",
				`POST /1.0/networks {"config":{"ipv4.address":"x"},"name":"br","type":"bridge"}`,
				`PATCH /1.0/networks/br {"config":{"dns.domain":"rye"}}`,
			},
		},
		{
			name:  "existing bridge",
			found: map[string]string{"/1.0/networks/br": `{"config":{}}`},
			want: []string{
				"GET /1.0/networks/br",
				`PATCH /1.0/networks/br {"config":{"dns.domain":"rye"}}`,
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			a, calls := recordAPI(t, tt.found)
			if err := a.EnsureNetwork(context.Background(), "br", tt.create, map[string]string{"dns.domain": "rye"}); err != nil {
				t.Fatalf("EnsureNetwork() error = %v", err)
			}
			if !slices.Equal(*calls, tt.want) {
				t.Errorf("calls = %q, want %q", *calls, tt.want)
			}
		})
	}
}

func TestAdmin_TrustRestricted(t *testing.T) {
	certPEM, _ := testCertFor(t)
	sum := sha256.Sum256(decodePEM(t, certPEM))
	fingerprint := hex.EncodeToString(sum[:])
	tests := []struct {
		name      string
		certs     string
		wantCalls int
		wantSent  []string
	}{
		{name: "trusted certificate", certs: `[{"name":"rye","fingerprint":"` + fingerprint + `"}]`, wantCalls: 1},
		{name: "new certificate", certs: `[]`, wantCalls: 2, wantSent: []string{`"restricted":true`, `"projects":["rye"]`}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			a, calls := recordAPI(t, map[string]string{"/1.0/certificates": tt.certs})
			if err := a.TrustRestricted(context.Background(), "rye", certPEM, []string{"rye"}); err != nil {
				t.Fatalf("TrustRestricted() error = %v", err)
			}
			if len(*calls) != tt.wantCalls {
				t.Fatalf("got %d calls %q, want %d", len(*calls), *calls, tt.wantCalls)
			}
			last := (*calls)[len(*calls)-1]
			for _, want := range tt.wantSent {
				if !strings.Contains(last, want) {
					t.Errorf("last call = %q, want it to contain %q", last, want)
				}
			}
		})
	}
}

func TestAdmin_DeleteInstance(t *testing.T) {
	tests := []struct {
		name  string
		found map[string]string
		want  []string
	}{
		{
			name:  "running VM",
			found: map[string]string{"/1.0/instances/s": `{"status":"Running"}`},
			want: []string{
				"GET /1.0/instances/s",
				`PUT /1.0/instances/s/state {"action":"stop","force":true}`,
				"DELETE /1.0/instances/s",
			},
		},
		{
			name:  "stopped VM",
			found: map[string]string{"/1.0/instances/s": `{"status":"Stopped"}`},
			want:  []string{"GET /1.0/instances/s", "DELETE /1.0/instances/s"},
		},
		{
			name: "missing VM",
			want: []string{"GET /1.0/instances/s"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			a, calls := recordAPI(t, tt.found)
			if err := a.DeleteInstance(context.Background(), "s"); err != nil {
				t.Fatalf("DeleteInstance() error = %v", err)
			}
			if !slices.Equal(*calls, tt.want) {
				t.Errorf("calls = %q, want %q", *calls, tt.want)
			}
		})
	}
}

// testCertFor returns a self-signed PEM certificate for names, and its key.
func testCertFor(t *testing.T, names ...string) ([]byte, *ecdsa.PrivateKey) {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	template := &x509.Certificate{SerialNumber: big.NewInt(1), Subject: pkix.Name{CommonName: "t"}, DNSNames: names,
		NotBefore: time.Now(), NotAfter: time.Now().Add(time.Hour)}
	der, err := x509.CreateCertificate(rand.Reader, template, template, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	return pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}), key
}
