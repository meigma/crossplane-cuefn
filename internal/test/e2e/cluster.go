//go:build e2e

package e2e

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// crossplaneChartVersion pins the Crossplane helm chart the harness installs,
// matching the mise-pinned crossplane CLI (2.3.3) so the cluster and the
// author-side tooling speak the same Crossplane API.
const crossplaneChartVersion = "2.3.3"

// kindConfig sets containerd's registry config_path to the certs.d hosts.toml
// tree the harness later populates, so the nodes can pull the Function runtime
// image from the CA-trusted TLS registry.
const kindConfig = `kind: Cluster
apiVersion: kind.x-k8s.io/v1alpha4
containerdConfigPatches:
- |-
  [plugins."io.containerd.grpc.v1.cri".registry]
    config_path = "/etc/containerd/certs.d"
nodes:
- role: control-plane
`

// Cluster is a throwaway kind cluster handle whose Kubeconfig drives kubectl,
// helm, and chainsaw. The caller defers Delete.
type Cluster struct {
	name       string
	kubeconfig string
}

// NewCluster creates a kind cluster named `name`, writing its kubeconfig to a
// dedicated file (so it never disturbs the developer's default context), and
// waits for the control plane to be ready.
func NewCluster(ctx context.Context, name string) (*Cluster, error) {
	dir, err := os.MkdirTemp("", "cuefn-e2e-kube-")
	if err != nil {
		return nil, fmt.Errorf("cannot create kubeconfig dir: %w", err)
	}
	kubeconfig := filepath.Join(dir, "kubeconfig")

	cfgFile := filepath.Join(dir, "kind.yaml")
	if err = os.WriteFile(cfgFile, []byte(kindConfig), 0o600); err != nil {
		return nil, fmt.Errorf("cannot write kind config: %w", err)
	}

	// Delete any leftover cluster of the same name first (idempotent reruns).
	_, _ = toolOutput(ctx, "kind", "delete", "cluster", "--name", name)

	out, err := toolOutput(ctx, "kind", "create", "cluster",
		"--name", name,
		"--kubeconfig", kubeconfig,
		"--config", cfgFile,
		"--wait", "120s",
	)
	if err != nil {
		return nil, fmt.Errorf("cannot create kind cluster %s: %w\n%s", name, err, out)
	}
	return &Cluster{name: name, kubeconfig: kubeconfig}, nil
}

// Kubeconfig returns the path to the cluster's kubeconfig file.
func (c *Cluster) Kubeconfig() string { return c.kubeconfig }

// Delete tears the cluster down.
func (c *Cluster) Delete(ctx context.Context) error {
	out, err := toolOutput(ctx, "kind", "delete", "cluster", "--name", c.name)
	if err != nil {
		return fmt.Errorf("cannot delete kind cluster %s: %w\n%s", c.name, err, out)
	}
	return nil
}

// Kubectl runs kubectl against this cluster, returning its combined output.
func (c *Cluster) Kubectl(ctx context.Context, args ...string) ([]byte, error) {
	return c.kube(ctx, "kubectl", args...)
}

// Apply applies the given manifest bytes to the cluster via `kubectl apply -f -`.
func (c *Cluster) Apply(ctx context.Context, manifest []byte) ([]byte, error) {
	cmd := exec.CommandContext(ctx, "kubectl", "apply", "-f", "-")
	cmd.Env = append(os.Environ(), "KUBECONFIG="+c.kubeconfig)
	cmd.Stdin = bytes.NewReader(manifest)
	return cmd.CombinedOutput()
}

// nodes lists the kind node container names for this cluster.
func (c *Cluster) nodes(ctx context.Context) ([]string, error) {
	out, err := toolOutput(ctx, "kind", "get", "nodes", "--name", c.name)
	if err != nil {
		return nil, fmt.Errorf("cannot list kind nodes: %w\n%s", err, out)
	}
	var nodes []string
	for line := range strings.FieldsSeq(string(out)) {
		if line != "" {
			nodes = append(nodes, line)
		}
	}
	return nodes, nil
}

// TrustRegistry installs reg's CA into each node's containerd certs.d tree so
// containerd pulls the Function runtime image from the TLS registry over a
// trusted connection. It is a no-op for a plain-HTTP registry.
func (c *Cluster) TrustRegistry(ctx context.Context, reg *Registry) error {
	caPath := reg.CACertPath()
	if caPath == "" {
		return nil
	}
	nodes, err := c.nodes(ctx)
	if err != nil {
		return err
	}
	dir := "/etc/containerd/certs.d/" + reg.ClusterRef()
	hostsTOML := fmt.Sprintf("server = \"https://%s\"\n\n[host.\"https://%s\"]\n  ca = \"%s/ca.crt\"\n",
		reg.ClusterRef(), reg.ClusterRef(), dir)

	for _, node := range nodes {
		if out, err := dockerOutput(ctx, "exec", node, "mkdir", "-p", dir); err != nil {
			return fmt.Errorf("cannot create certs.d on node %s: %w\n%s", node, err, out)
		}
		if out, err := dockerOutput(ctx, "cp", caPath, node+":"+dir+"/ca.crt"); err != nil {
			return fmt.Errorf("cannot copy CA to node %s: %w\n%s", node, err, out)
		}
		// Write hosts.toml via `tee` so the heredoc content is controlled by us.
		cmd := exec.CommandContext(ctx, "docker", "exec", "-i", node,
			"tee", dir+"/hosts.toml")
		cmd.Stdin = strings.NewReader(hostsTOML)
		if out, err := cmd.CombinedOutput(); err != nil {
			return fmt.Errorf("cannot write hosts.toml on node %s: %w\n%s", node, err, out)
		}
	}
	return nil
}

// InstallCrossplane installs Crossplane into the cluster via helm, wiring the TLS
// registry's CA bundle into the package manager (registryCaBundleConfig) so
// Crossplane trusts the registry it pulls the Function and Configuration xpkgs
// from. It waits for the Crossplane deployment to become available.
func InstallCrossplane(ctx context.Context, c *Cluster, caBundle []byte) error {
	const ns = "crossplane-system"
	if out, err := c.kube(ctx, "kubectl", "create", "namespace", ns); err != nil &&
		!strings.Contains(string(out), "AlreadyExists") {
		return fmt.Errorf("cannot create %s namespace: %w\n%s", ns, err, out)
	}

	if len(caBundle) > 0 {
		caFile, err := os.CreateTemp("", "cuefn-ca-*.crt")
		if err != nil {
			return err
		}
		defer func() { _ = os.Remove(caFile.Name()) }()
		if _, err := caFile.Write(caBundle); err != nil {
			return err
		}
		_ = caFile.Close()
		// The package manager mounts this ConfigMap and is pointed at it via
		// --ca-bundle-path (helm registryCaBundleConfig), so it trusts the
		// self-signed registry CA.
		_, _ = c.kube(ctx, "kubectl", "-n", ns, "delete", "configmap", "cuefn-registry-ca")
		if out, err := c.kube(ctx, "kubectl", "-n", ns, "create", "configmap",
			"cuefn-registry-ca", "--from-file=ca.crt="+caFile.Name()); err != nil {
			return fmt.Errorf("cannot create CA configmap: %w\n%s", err, out)
		}
	}

	if out, err := toolOutput(ctx, "helm", "repo", "add", "crossplane-stable",
		"https://charts.crossplane.io/stable"); err != nil &&
		!strings.Contains(string(out), "already exists") {
		return fmt.Errorf("cannot add crossplane helm repo: %w\n%s", err, out)
	}
	if out, err := toolOutput(ctx, "helm", "repo", "update", "crossplane-stable"); err != nil {
		return fmt.Errorf("cannot update crossplane helm repo: %w\n%s", err, out)
	}

	args := []string{
		"install", "crossplane", "crossplane-stable/crossplane",
		"--namespace", ns,
		"--version", crossplaneChartVersion,
		"--set", "registryCaBundleConfig.name=cuefn-registry-ca",
		"--set", "registryCaBundleConfig.key=ca.crt",
		"--wait", "--timeout", "5m",
	}
	if out, err := c.kube(ctx, "helm", args...); err != nil {
		return fmt.Errorf("cannot install crossplane: %w\n%s", err, out)
	}
	return nil
}

// kube runs a tool with KUBECONFIG pointed at this cluster.
func (c *Cluster) kube(ctx context.Context, name string, args ...string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Env = append(os.Environ(), "KUBECONFIG="+c.kubeconfig)
	return cmd.CombinedOutput()
}

// toolOutput runs a tool and returns its combined output.
func toolOutput(ctx context.Context, name string, args ...string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, name, args...)
	return cmd.CombinedOutput()
}

// dockerOutput runs docker and returns its combined output.
func dockerOutput(ctx context.Context, args ...string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, "docker", args...)
	return cmd.CombinedOutput()
}

// removeContainer force-removes a docker container by name, ignoring a missing
// container (idempotent setup/teardown).
func removeContainer(ctx context.Context, name string) error {
	out, err := dockerOutput(ctx, "rm", "-f", name)
	if err != nil && !strings.Contains(string(out), "No such container") {
		return fmt.Errorf("cannot remove container %s: %w\n%s", name, err, out)
	}
	return nil
}
