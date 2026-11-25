package session

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestInstallAndRemoveHookPreserveOtherEntries(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config", "hooks.json")
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	initial := `{"unrelated-tool":{"Stop":[{"type":"command","command":"echo keep"}]},"other":{"Stop":[{"type":"command","command":"echo keep"}]}}`
	if err := os.WriteFile(path, []byte(initial), 0o600); err != nil {
		t.Fatal(err)
	}
	command := `'/opt/agy tools/agy-tools' hook`
	changed, err := InstallHook(path, command)
	if err != nil || !changed {
		t.Fatalf("InstallHook() = (%v, %v), want changed", changed, err)
	}
	if !HookConfigured(path, command) {
		t.Fatal("installed hook was not recognized")
	}
	var content map[string]any
	data, err := os.ReadFile(path)
	if err != nil || json.Unmarshal(data, &content) != nil {
		t.Fatalf("read hooks config: %v", err)
	}
	if _, ok := content["other"]; !ok {
		t.Fatal("InstallHook removed an unrelated hook")
	}
	if changed, err = InstallHook(path, command); err != nil || changed {
		t.Fatalf("idempotent InstallHook() = (%v, %v)", changed, err)
	}
	if _, err := InstallHook(path, `'/different/agy-tools' hook`); err == nil {
		t.Fatal("InstallHook overwrote a conflicting agym hook")
	}
	changed, err = UninstallHook(path)
	if err != nil || !changed {
		t.Fatalf("UninstallHook() = (%v, %v), want changed", changed, err)
	}
	data, err = os.ReadFile(path)
	if err != nil || json.Unmarshal(data, &content) != nil {
		t.Fatalf("read hooks config after uninstall: %v", err)
	}
	if _, ok := content["other"]; !ok {
		t.Fatal("UninstallHook removed an unrelated hook")
	}
}

func TestHookInstallRejectsMalformedConfig(t *testing.T) {
	path := filepath.Join(t.TempDir(), "hooks.json")
	if err := os.WriteFile(path, []byte("not-json"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := InstallHook(path, "agym hook"); err == nil {
		t.Fatal("InstallHook overwrote malformed user config")
	}
}
