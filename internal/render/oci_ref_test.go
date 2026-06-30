package render

import (
	"strings"
	"testing"
)

func TestParseModuleRef_FullVersionParses(t *testing.T) {
	if _, err := parseModuleRef("cuefn.example/app@v0.1.0"); err != nil {
		t.Fatalf("unexpected error for a full version: %v", err)
	}
}

func TestParseModuleRef_MajorOnlyExplainsOCIRequirement(t *testing.T) {
	_, err := parseModuleRef("cuefn.example/app@v0")
	if err == nil {
		t.Fatal("expected an error for a major-only OCI ref")
	}
	for _, want := range []string{"full version", "--dir", "cuefn.example/app@v0"} {
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("error %q does not mention %q", err.Error(), want)
		}
	}
}
