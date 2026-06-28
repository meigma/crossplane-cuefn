package render

import (
	"archive/zip"
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"cuelang.org/go/mod/modconfig"
	"cuelang.org/go/mod/modregistry"
	"cuelang.org/go/mod/module"

	"github.com/opencontainers/go-digest"
)

// ociCacheSubdir is the directory, under the resolved cache root, that holds the
// loader's digest-keyed extractions and ref->digest pointers. It lives beside
// CUE's own version-keyed modcache so the two never collide.
const ociCacheSubdir = "cuefn-oci"

// OCIConfig configures an [OCILoader].
type OCIConfig struct {
	// Env provides environment variables (CUE_REGISTRY, CUE_CACHE_DIR, ...) used
	// to build the registry clients. When nil, the process environment
	// (os.Environ) is used. Injecting Env keeps resolution explicit and avoids
	// relying on process-global state under parallel tests.
	Env []string

	// CacheDir is the writable directory used for both CUE's module cache and the
	// loader's digest-keyed extraction cache. When set it overrides CUE_CACHE_DIR
	// in Env, which makes caching work for a nonroot or read-only-root container.
	// When empty, CUE_CACHE_DIR (or the OS user cache) is used.
	CacheDir string

	// Expect maps a module ref ("path@version") to the manifest digest the loader
	// must observe after fetch. A mismatch fails the load. Refs absent from the
	// map are not digest-checked. This is the runtime half of the schema<->runtime
	// digest lock-step: CUE references modules by semver, not digest, so the
	// expected digest is verified after fetch rather than referenced directly.
	//
	// Only the root module ref passed to Load is verified. Transitive dependency
	// refs in this map are ignored: deps are resolved immutably by version through
	// CUE's module cache, so a per-dep digest lock is out of scope here.
	Expect map[string]string
}

// OCILoader loads CUE modules from an OCI registry using the CUE module registry
// protocol. It honors CUE_REGISTRY (including the "+insecure" suffix used for
// plain-HTTP localhost registries).
//
// The root module is handled digest-aware: every Load re-resolves the tag to its
// current manifest digest and keys a loader-owned extraction cache by that
// digest, so changed content under the same tag yields a new digest, a cache
// miss, and a re-render. A ref->digest pointer lets a fresh process serve the
// last-known digest from cache when the registry is unreachable. Transitive
// dependencies resolve through CUE's own version-keyed module cache (correct,
// since versions are immutable) via the injected registry.
type OCILoader struct {
	client    *modregistry.Client
	registry  modconfig.Registry
	cacheRoot string
	expect    map[string]digest.Digest
}

// NewOCILoader builds an OCILoader from cfg. It resolves a writable cache dir,
// forces CUE_CACHE_DIR to it, and derives both a digest-aware registry client
// (for the root fetch) and a CUE registry (for transitive deps and the shared
// modcache) from the same configuration.
func NewOCILoader(cfg OCIConfig) (*OCILoader, error) {
	env := cfg.Env
	if env == nil {
		env = os.Environ()
	}

	cacheDir, err := resolveCacheDir(cfg.CacheDir, env)
	if err != nil {
		return nil, err
	}
	// Force CUE_CACHE_DIR so the dep modcache and the loader's extraction cache
	// share one writable root, regardless of what the caller's environment said.
	env = setEnv(env, "CUE_CACHE_DIR", cacheDir)

	mcfg := &modconfig.Config{Env: env}
	resolver, err := modconfig.NewResolver(mcfg)
	if err != nil {
		return nil, fmt.Errorf("cannot build CUE registry resolver from environment: %w", err)
	}
	registry, err := modconfig.NewRegistry(mcfg)
	if err != nil {
		return nil, fmt.Errorf("cannot build CUE registry from environment: %w", err)
	}

	expect := make(map[string]digest.Digest, len(cfg.Expect))
	for ref, d := range cfg.Expect {
		parsed, err := digest.Parse(d)
		if err != nil {
			return nil, fmt.Errorf("invalid expected digest for module %q: %w", ref, err)
		}
		expect[ref] = parsed
	}

	return &OCILoader{
		client:    modregistry.NewClientWithResolver(resolver),
		registry:  registry,
		cacheRoot: filepath.Join(cacheDir, ociCacheSubdir),
		expect:    expect,
	}, nil
}

// Load fetches the module at ref from the OCI registry, verifies its manifest
// digest, extracts it (once) into a digest-keyed cache directory, and returns
// that directory together with the CUE registry for resolving transitive deps.
//
// When the registry is reachable, the tag is always re-resolved to its current
// digest so republished content is re-fetched. When the registry is unreachable,
// the loader falls back to the last-known digest recorded for ref and serves it
// from cache. A non-existent ref (404/NAME_UNKNOWN/MANIFEST_UNKNOWN) propagates
// rather than falling back.
func (o *OCILoader) Load(ctx context.Context, ref string) (Loaded, error) {
	mv, err := module.ParseVersion(ref)
	if err != nil {
		return Loaded{}, fmt.Errorf("invalid module reference %q (want path@version): %w", ref, err)
	}

	m, err := o.client.GetModule(ctx, mv)
	if err != nil {
		return o.loadOffline(ref, err)
	}

	dg := m.ManifestDigest()
	if err := o.verifyDigest(ref, dg); err != nil {
		return Loaded{}, err
	}

	dir := o.digestDir(dg)
	if err := o.ensureExtracted(ctx, ref, m, dir); err != nil {
		return Loaded{}, err
	}
	if err := o.writePointer(ref, dg); err != nil {
		return Loaded{}, err
	}

	return o.loaded(dir), nil
}

// ResolveDigest resolves ref ("path@version") to its current OCI manifest digest
// ("sha256:...") by querying the same registry the loader fetches modules from.
// It is the publish-time half of the schema<->runtime digest lock-step: the
// author records this real, resolved digest in the Composition's cuefn input so
// the runtime loader (OCIConfig.Expect) can verify the module it later fetches
// has not drifted. A malformed ref, a non-existent module, or an unreachable
// registry surfaces as a clear error naming ref; no digest cache fallback is
// used because publish must observe the live digest.
func (o *OCILoader) ResolveDigest(ctx context.Context, ref string) (string, error) {
	mv, err := module.ParseVersion(ref)
	if err != nil {
		return "", fmt.Errorf("invalid module reference %q (want path@version): %w", ref, err)
	}

	m, err := o.client.GetModule(ctx, mv)
	if err != nil {
		if errors.Is(err, modregistry.ErrNotFound) {
			return "", fmt.Errorf("module %q not found in registry: %w", ref, err)
		}
		return "", fmt.Errorf("cannot resolve digest for module %q: %w", ref, err)
	}

	return m.ManifestDigest().String(), nil
}

// loadOffline handles a failed online resolve. A non-existent ref is propagated;
// a transport/dial failure falls back to the last-known digest recorded for ref,
// if any, and otherwise surfaces an unreachable-registry error.
func (o *OCILoader) loadOffline(ref string, cause error) (Loaded, error) {
	if errors.Is(cause, modregistry.ErrNotFound) {
		return Loaded{}, fmt.Errorf("module %q not found in registry: %w", ref, cause)
	}

	dg, ok := o.readPointer(ref)
	if !ok {
		return Loaded{}, fmt.Errorf("cannot reach registry for module %q: %w", ref, cause)
	}
	if err := o.verifyDigest(ref, dg); err != nil {
		return Loaded{}, err
	}
	dir := o.digestDir(dg)
	if !dirExists(dir) {
		return Loaded{}, fmt.Errorf(
			"cannot reach registry for module %q and no cached copy is available: %w",
			ref,
			cause,
		)
	}
	return o.loaded(dir), nil
}

// loaded returns a Loaded for dir. The registry is shared so transitive deps
// resolve through CUE's modcache; cleanup is a no-op because the digest-keyed
// cache is persistent and shared across loads and processes.
func (o *OCILoader) loaded(dir string) Loaded {
	return Loaded{Dir: dir, Registry: o.registry, Cleanup: func() {}}
}

// verifyDigest checks the observed manifest digest against an expected value for
// ref, when one is configured. It is a no-op for refs without an expectation.
func (o *OCILoader) verifyDigest(ref string, got digest.Digest) error {
	want, ok := o.expect[ref]
	if !ok {
		return nil
	}
	if got != want {
		return fmt.Errorf("module %q manifest digest mismatch: expected %s, got %s", ref, want, got)
	}
	return nil
}

// digestDir is the cache directory that holds the extraction for a digest.
func (o *OCILoader) digestDir(dg digest.Digest) string {
	return filepath.Join(o.cacheRoot, dg.Algorithm().String(), dg.Encoded())
}

// ensureExtracted extracts the module zip into dir exactly once. Extraction goes
// to a sibling temp dir and is atomically renamed into place, so a concurrent
// loader (or a crashed one) never observes a half-written module.
func (o *OCILoader) ensureExtracted(ctx context.Context, ref string, m *modregistry.Module, dir string) error {
	if dirExists(dir) {
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(dir), 0o750); err != nil {
		return fmt.Errorf("cannot create cache directory for module %q: %w", ref, err)
	}

	tmp, err := os.MkdirTemp(filepath.Dir(dir), ".partial-")
	if err != nil {
		return fmt.Errorf("cannot create temp dir for module %q: %w", ref, err)
	}
	defer os.RemoveAll(tmp)

	zr, err := m.GetZip(ctx)
	if err != nil {
		return fmt.Errorf("cannot fetch module zip for %q: %w", ref, err)
	}
	defer zr.Close()

	if err := unzipModule(zr, tmp); err != nil {
		return fmt.Errorf("cannot extract module %q: %w", ref, err)
	}

	if err := os.Rename(tmp, dir); err != nil {
		// A concurrent loader may have won the race and populated dir first.
		if dirExists(dir) {
			return nil
		}
		return fmt.Errorf("cannot finalize cache for module %q: %w", ref, err)
	}
	return nil
}

// pointerPath is the file recording the last-known manifest digest for ref. The
// ref is path-escaped so it forms a single safe filename.
func (o *OCILoader) pointerPath(ref string) string {
	return filepath.Join(o.cacheRoot, "refs", url.PathEscape(ref))
}

// writePointer records ref's current digest so a later offline load can find it.
// The write is atomic (temp file + rename) and best-effort consistent.
func (o *OCILoader) writePointer(ref string, dg digest.Digest) error {
	path := o.pointerPath(ref)
	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		return fmt.Errorf("cannot create pointer directory for module %q: %w", ref, err)
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), ".ptr-")
	if err != nil {
		return fmt.Errorf("cannot write digest pointer for module %q: %w", ref, err)
	}
	tmpName := tmp.Name()
	if _, err := tmp.WriteString(dg.String()); err != nil {
		_ = tmp.Close()
		_ = os.Remove(tmpName)
		return fmt.Errorf("cannot write digest pointer for module %q: %w", ref, err)
	}
	if err := tmp.Close(); err != nil {
		_ = os.Remove(tmpName)
		return fmt.Errorf("cannot write digest pointer for module %q: %w", ref, err)
	}
	if err := os.Rename(tmpName, path); err != nil {
		_ = os.Remove(tmpName)
		return fmt.Errorf("cannot write digest pointer for module %q: %w", ref, err)
	}
	return nil
}

// readPointer returns the last-known digest recorded for ref, if any.
func (o *OCILoader) readPointer(ref string) (digest.Digest, bool) {
	data, err := os.ReadFile(o.pointerPath(ref))
	if err != nil {
		return "", false
	}
	dg, err := digest.Parse(strings.TrimSpace(string(data)))
	if err != nil {
		return "", false
	}
	return dg, true
}

// resolveCacheDir picks the writable cache root: an explicit dir wins, then
// CUE_CACHE_DIR from env, then the OS user cache dir (matching CUE's own
// fallback in internal/cueconfig.CacheDir).
func resolveCacheDir(explicit string, env []string) (string, error) {
	if explicit != "" {
		return explicit, nil
	}
	if dir := getEnv(env, "CUE_CACHE_DIR"); dir != "" {
		return dir, nil
	}
	dir, err := os.UserCacheDir()
	if err != nil {
		return "", fmt.Errorf("cannot determine cache directory (set CUE_CACHE_DIR): %w", err)
	}
	return filepath.Join(dir, "cue"), nil
}

// getEnv reads key from an env slice ("KEY=value"), preferring the last match to
// mirror process-environment override semantics.
func getEnv(env []string, key string) string {
	prefix := key + "="
	for _, e := range slices.Backward(env) {
		if strings.HasPrefix(e, prefix) {
			return e[len(prefix):]
		}
	}
	return ""
}

// setEnv returns env with key set to val, replacing any existing entries.
func setEnv(env []string, key, val string) []string {
	prefix := key + "="
	out := make([]string, 0, len(env)+1)
	set := false
	for _, e := range env {
		if strings.HasPrefix(e, prefix) {
			if !set {
				out = append(out, prefix+val)
				set = true
			}
			continue
		}
		out = append(out, e)
	}
	if !set {
		out = append(out, prefix+val)
	}
	return out
}

// dirExists reports whether path is an existing directory.
func dirExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.IsDir()
}

// unzipModule extracts a CUE module zip into dir. The zip returned by the CUE
// registry (modregistry GetZip) has module-root-relative entries (e.g.
// "cue.mod/module.cue", "api.cue"), so each entry maps directly under dir.
func unzipModule(r io.Reader, dir string) error {
	buf, err := io.ReadAll(r)
	if err != nil {
		return err
	}
	zr, err := zip.NewReader(bytes.NewReader(buf), int64(len(buf)))
	if err != nil {
		return err
	}
	for _, f := range zr.File {
		if f.Name == "" || strings.HasSuffix(f.Name, "/") {
			continue
		}
		if err := writeZipEntry(f, filepath.Join(dir, filepath.FromSlash(f.Name))); err != nil {
			return err
		}
	}
	return nil
}

// writeZipEntry writes one zip entry to dest, creating parent directories.
func writeZipEntry(f *zip.File, dest string) error {
	if err := os.MkdirAll(filepath.Dir(dest), 0o750); err != nil {
		return err
	}
	rc, err := f.Open()
	if err != nil {
		return err
	}
	defer rc.Close()
	out, err := os.Create(dest)
	if err != nil {
		return err
	}
	defer out.Close()
	_, err = io.Copy(out, rc) //nolint:gosec // module contents are size-bounded by the registry
	return err
}
