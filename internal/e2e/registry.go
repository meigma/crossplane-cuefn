//go:build e2e

package e2e

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"fmt"
	"math/big"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"time"
)

const (
	// registryImage pins the OCI registry image both e2e registries run, matching
	// the pin the render/function integration tests use so the harness is
	// reproducible.
	registryImage = "registry:2.8.3"

	// registryReadyTimeout / registryPollInterval / httpClientTimeout bound the
	// /v2/ readiness poll after a registry container starts.
	registryReadyTimeout = 60 * time.Second
	registryPollInterval = 500 * time.Millisecond
	httpClientTimeout    = 2 * time.Second

	// certValidity is how long the throwaway CA and server certs are valid; the
	// cluster is torn down within minutes, so a day is ample.
	certValidity = 24 * time.Hour
)

// Registry is a throwaway OCI registry running as a host Docker container joined
// to kind's network, so both the host (via the published loopback port) and the
// in-cluster workloads (via the container name on the kind network) reach the
// same content. A TLS registry serves the Crossplane xpkgs Crossplane pulls over
// HTTPS; a plain-HTTP one serves the CUE modules the function fetches +insecure.
type Registry struct {
	name        string // docker container name (for docker ops)
	clusterHost string // in-cluster DNS name (a network alias)
	hostPort    int    // published loopback port on the host
	clusterID   string // "<clusterHost>:5000" reference used inside the cluster
	tls         bool
	caPEM       []byte // CA bundle (TLS registries only)
	certDir     string // host dir holding ca.crt/server.crt/server.key (TLS only)
}

// StartTLSRegistry runs an HTTPS registry as host container `name`, published on
// 127.0.0.1:hostPort, reachable inside the cluster as `clusterHost` (a Docker
// network alias). clusterHost MUST be a dotted hostname (e.g. foo.bar) because
// Crossplane's package-ref validation rejects a registry host without a dot. A
// freshly minted self-signed CA's server cert covers clusterHost, localhost, and
// 127.0.0.1. The CA bundle is handed to Crossplane (helm registryCaBundleConfig)
// and to the kind nodes' containerd so both trust the registry. The caller must
// Connect it to the kind network after the cluster exists and defer Close.
func StartTLSRegistry(ctx context.Context, name, clusterHost string, hostPort int) (*Registry, error) {
	certDir, err := os.MkdirTemp("", "cuefn-e2e-certs-")
	if err != nil {
		return nil, fmt.Errorf("cannot create cert dir: %w", err)
	}
	caPEM, err := writeTLSMaterial(certDir, clusterHost)
	if err != nil {
		return nil, fmt.Errorf("cannot mint TLS material: %w", err)
	}

	if err := removeContainer(ctx, name); err != nil {
		return nil, err
	}
	args := []string{
		"run", "-d", "--name", name,
		"-p", fmt.Sprintf("127.0.0.1:%d:5000", hostPort),
		"-v", certDir + ":/certs:ro",
		"-e", "REGISTRY_HTTP_TLS_CERTIFICATE=/certs/server.crt",
		"-e", "REGISTRY_HTTP_TLS_KEY=/certs/server.key",
		registryImage,
	}
	if out, err := dockerOutput(ctx, args...); err != nil {
		return nil, fmt.Errorf("cannot start TLS registry %s: %w\n%s", name, err, out)
	}

	r := &Registry{
		name:        name,
		clusterHost: clusterHost,
		hostPort:    hostPort,
		clusterID:   clusterHost + ":5000",
		tls:         true,
		caPEM:       caPEM,
		certDir:     certDir,
	}
	if err := r.waitReady(ctx); err != nil {
		_ = r.Close(ctx)
		return nil, err
	}
	return r, nil
}

// StartHTTPRegistry runs a plain-HTTP registry as host container `name`,
// published on 127.0.0.1:hostPort. It serves the CUE modules the function fetches
// with CUE_REGISTRY=<name>:5000+insecure; Crossplane never touches it.
func StartHTTPRegistry(ctx context.Context, name string, hostPort int) (*Registry, error) {
	if err := removeContainer(ctx, name); err != nil {
		return nil, err
	}
	args := []string{
		"run", "-d", "--name", name,
		"-p", fmt.Sprintf("127.0.0.1:%d:5000", hostPort),
		registryImage,
	}
	if out, err := dockerOutput(ctx, args...); err != nil {
		return nil, fmt.Errorf("cannot start HTTP registry %s: %w\n%s", name, err, out)
	}

	r := &Registry{
		name:        name,
		clusterHost: name,
		hostPort:    hostPort,
		clusterID:   name + ":5000",
		tls:         false,
	}
	if err := r.waitReady(ctx); err != nil {
		_ = r.Close(ctx)
		return nil, err
	}
	return r, nil
}

// Connect joins the registry container to kind's Docker network under its cluster
// host alias, so the cluster's nodes and pods resolve it by that name. kind names
// its network "kind".
func (r *Registry) Connect(ctx context.Context) error {
	if out, err := dockerOutput(ctx, "network", "connect", "--alias", r.clusterHost, "kind", r.name); err != nil {
		return fmt.Errorf("cannot connect registry %s to kind network: %w\n%s", r.name, err, out)
	}
	return nil
}

// HostRef is the registry address reachable from the host (the test process),
// e.g. "localhost:5000". Packages and modules are pushed here.
func (r *Registry) HostRef() string { return fmt.Sprintf("localhost:%d", r.hostPort) }

// ClusterRef is the registry address reachable from inside the cluster, e.g.
// "cuefn-e2e-registry:5000". Package and module refs installed into the cluster
// use this name.
func (r *Registry) ClusterRef() string { return r.clusterID }

// CABundle returns the PEM CA bundle a TLS registry's clients must trust; nil for
// a plain-HTTP registry.
func (r *Registry) CABundle() []byte { return r.caPEM }

// CACertPath returns the on-disk ca.crt path for a TLS registry, for mounting
// into containerd; empty for a plain-HTTP registry.
func (r *Registry) CACertPath() string {
	if !r.tls {
		return ""
	}
	return filepath.Join(r.certDir, "ca.crt")
}

// Transport returns an [http.RoundTripper] that trusts the registry's CA, for
// pushing to a TLS registry from the host with go-containerregistry. It returns
// [http.DefaultTransport] for a plain-HTTP registry.
func (r *Registry) Transport() http.RoundTripper {
	if !r.tls {
		return http.DefaultTransport
	}
	pool := x509.NewCertPool()
	pool.AppendCertsFromPEM(r.caPEM)
	base, _ := http.DefaultTransport.(*http.Transport)
	tr := base.Clone()
	tr.TLSClientConfig = &tls.Config{RootCAs: pool, MinVersion: tls.VersionTLS12}
	return tr
}

// Close removes the registry container and any temp cert dir. It is safe to call
// more than once.
func (r *Registry) Close(ctx context.Context) error {
	err := removeContainer(ctx, r.name)
	if r.certDir != "" {
		_ = os.RemoveAll(r.certDir)
	}
	return err
}

// waitReady polls the registry's /v2/ endpoint over the published loopback port
// until it answers, so pushes do not race the container's startup.
func (r *Registry) waitReady(ctx context.Context) error {
	scheme := "http"
	client := &http.Client{Timeout: httpClientTimeout}
	if r.tls {
		scheme = "https"
		client.Transport = r.Transport()
	}
	url := fmt.Sprintf("%s://%s/v2/", scheme, r.HostRef())

	deadline := time.Now().Add(registryReadyTimeout)
	for {
		req, _ := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
		resp, err := client.Do(req)
		if err == nil {
			_ = resp.Body.Close()
			return nil
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("registry %s did not become ready: %w", r.name, err)
		}
		time.Sleep(registryPollInterval)
	}
}

// writeTLSMaterial mints a self-signed CA and a server certificate signed by it
// (SANs: cn, localhost, 127.0.0.1), writes ca.crt/server.crt/server.key into dir,
// and returns the CA bundle PEM.
func writeTLSMaterial(dir, cn string) ([]byte, error) {
	caKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return nil, err
	}
	caTmpl := &x509.Certificate{
		SerialNumber:          big.NewInt(1),
		Subject:               pkix.Name{CommonName: "cuefn-e2e-ca"},
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              time.Now().Add(certValidity),
		IsCA:                  true,
		KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageDigitalSignature,
		BasicConstraintsValid: true,
	}
	caDER, err := x509.CreateCertificate(rand.Reader, caTmpl, caTmpl, &caKey.PublicKey, caKey)
	if err != nil {
		return nil, err
	}
	caCert, err := x509.ParseCertificate(caDER)
	if err != nil {
		return nil, err
	}
	caPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: caDER})

	srvKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return nil, err
	}
	srvTmpl := &x509.Certificate{
		SerialNumber: big.NewInt(2), //nolint:mnd // a distinct serial from the CA's.
		Subject:      pkix.Name{CommonName: cn},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().Add(certValidity),
		KeyUsage:     x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		DNSNames:     []string{cn, "localhost"},
		IPAddresses:  []net.IP{net.ParseIP("127.0.0.1"), net.IPv6loopback},
	}
	srvDER, err := x509.CreateCertificate(rand.Reader, srvTmpl, caCert, &srvKey.PublicKey, caKey)
	if err != nil {
		return nil, err
	}
	srvKeyDER, err := x509.MarshalPKCS8PrivateKey(srvKey)
	if err != nil {
		return nil, err
	}

	files := map[string][]byte{
		"ca.crt":     caPEM,
		"server.crt": pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: srvDER}),
		"server.key": pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: srvKeyDER}),
	}
	for name, data := range files {
		if err := os.WriteFile(filepath.Join(dir, name), data, 0o600); err != nil {
			return nil, err
		}
	}
	return caPEM, nil
}
