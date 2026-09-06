package servertray

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestWriteBootstrapError(t *testing.T) {
	directory := t.TempDir()
	if err := WriteBootstrapError(directory, errors.New("first line\nsecond line")); err != nil {
		t.Fatalf("WriteBootstrapError: %v", err)
	}
	content, err := os.ReadFile(filepath.Join(directory, "bootstrap-error.log"))
	if err != nil {
		t.Fatalf("read bootstrap log: %v", err)
	}
	if !strings.Contains(string(content), "first line second line") {
		t.Fatalf("unexpected bootstrap log: %q", content)
	}
}
