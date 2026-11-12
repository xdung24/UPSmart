package ups

import (
	"os"
	"path/filepath"
	"testing"
)

func TestSameNamedManagementCopyIsNotMistakenForLegacy(t *testing.T) {
	root := t.TempDir()
	first, second := filepath.Join(root, "portable.exe"), filepath.Join(root, "installed.exe")
	for _, path := range []string{first, second} {
		if err := os.WriteFile(path, []byte("same replacement image"), 0600); err != nil {
			t.Fatal(err)
		}
	}
	if same, err := sameImage(first, second); err != nil || !same {
		t.Fatalf("identical portable/installed images misclassified: %t %v", same, err)
	}
	if err := os.WriteFile(second, []byte("different legacy image"), 0600); err != nil {
		t.Fatal(err)
	}
	if same, err := sameImage(first, second); err != nil || same {
		t.Fatalf("different image was excluded from guard: %t %v", same, err)
	}
	if _, err := sameImage(first, filepath.Join(root, "missing.exe")); err == nil {
		t.Fatal("inaccessible image was silently assumed safe")
	}
}
