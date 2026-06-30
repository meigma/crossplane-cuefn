package render

import (
	"os"
	"path/filepath"
	"testing"
)

// writableUserCache returns a userCache func reporting a creatable directory.
func writableUserCache(dir string) func() (string, error) {
	return func() (string, error) { return dir, nil }
}

func TestResolveCacheDir_ExplicitWins(t *testing.T) {
	got, err := resolveCacheDir("/explicit/path", []string{"CUE_CACHE_DIR=/env/path"}, writableUserCache(t.TempDir()))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "/explicit/path" {
		t.Fatalf("got %q, want /explicit/path", got)
	}
}

func TestResolveCacheDir_EnvWinsOverOSCache(t *testing.T) {
	got, err := resolveCacheDir("", []string{"CUE_CACHE_DIR=/env/path"}, writableUserCache(t.TempDir()))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "/env/path" {
		t.Fatalf("got %q, want /env/path", got)
	}
}

func TestResolveCacheDir_OSCacheWhenCreatable(t *testing.T) {
	base := t.TempDir()
	got, err := resolveCacheDir("", nil, writableUserCache(base))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := filepath.Join(base, "cue")
	if got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
	if _, statErr := os.Stat(got); statErr != nil {
		t.Fatalf("cache dir not created: %v", statErr)
	}
}

func TestResolveCacheDir_FallsBackWhenOSCacheUncreatable(t *testing.T) {
	// Point the user cache under a regular file so MkdirAll fails with ENOTDIR,
	// the portable stand-in for the nonroot container's EACCES on "/.cache".
	fileParent := filepath.Join(t.TempDir(), "not-a-dir")
	if err := os.WriteFile(fileParent, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	userCache := writableUserCache(filepath.Join(fileParent, "cache"))

	got, err := resolveCacheDir("", nil, userCache)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if filepath.Base(got) != "cuefn-cache" {
		t.Fatalf("got %q, want a .../cuefn-cache temp fallback", got)
	}
	if _, statErr := os.Stat(got); statErr != nil {
		t.Fatalf("fallback dir not created: %v", statErr)
	}
}

func TestResolveCacheDir_FallsBackWhenOSCacheUnknown(t *testing.T) {
	userCache := func() (string, error) { return "", os.ErrNotExist }

	got, err := resolveCacheDir("", nil, userCache)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if filepath.Base(got) != "cuefn-cache" {
		t.Fatalf("got %q, want a .../cuefn-cache temp fallback", got)
	}
}
