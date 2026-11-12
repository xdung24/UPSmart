package service

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDeploymentDoesNotOverwriteAnUnprotectedExistingExecutable(t *testing.T) {
	path := filepath.Join(t.TempDir(), "monitor.exe")
	if err := os.WriteFile(path, []byte("existing"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := writeProtected(path, strings.NewReader("replacement"), readOnlyACL, true); err == nil {
		t.Fatal("deployment accepted an executable outside the protected owner/ACL policy")
	}
	contents, err := os.ReadFile(path)
	if err != nil || string(contents) != "existing" {
		t.Fatal("unprotected existing file was modified")
	}
	if _, err := os.Stat(path + ".new"); !os.IsNotExist(err) {
		t.Fatalf("unexpected staging file after rejected deployment: %v", err)
	}
}
