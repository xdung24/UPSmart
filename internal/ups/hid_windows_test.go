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

func TestNormalizeHIDQ1RepairsOnlyValidatedFirstCharacterCorruption(t *testing.T) {
	valid := "(000.0 000.0 220.0 009 00.0 25.4 29.0 10001000\r"
	for _, damaged := range []string{
		"\x15000.0 000.0 220.0 009 00.0 25.4 29.0 10001000\r",
		"*000.0 000.0 220.0 009 00.0 25.4 29.0 10001000\r",
	} {
		if got := normalizeHIDQ1(damaged); got != valid {
			t.Fatalf("valid Q1 with damaged prefix not normalized: %q", got)
		}
	}
	for _, invalid := range []string{
		"\x15UPS No Ack",
		"*230.0 000.0 230.0 invalid",
	} {
		if got := normalizeHIDQ1(invalid); got != invalid {
			t.Fatalf("invalid HID response was rewritten: %q", got)
		}
	}
}
