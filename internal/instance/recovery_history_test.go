package instance

import (
	"path/filepath"
	"testing"
)

func TestRecoveryReplacementRetainsEveryHistoricalIdentity(t *testing.T) {
	root := t.TempDir()
	first := filepath.Join(root, "first")
	second := filepath.Join(root, "second")
	third := filepath.Join(root, "third")
	recipient, err := ExportRecoveryFile(first)
	if err != nil {
		t.Fatal(err)
	}
	next, err := ExportRecoveryHistory(first, second, recipient)
	if err != nil {
		t.Fatal(err)
	}
	newest, err := ExportRecoveryHistory(second, third, next)
	if err != nil {
		t.Fatal(err)
	}
	for _, public := range []string{recipient, next, newest} {
		if err := VerifyRecoveryFile(third, public); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := ExportRecoveryHistory(first, third, recipient); err == nil {
		t.Fatal("existing recovery file overwritten")
	}
	if _, err := ExportRecoveryHistory(first, filepath.Join(root, "wrong"), next); err == nil {
		t.Fatal("wrong original recovery identity accepted")
	}
}
