package session

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const HookName = "agy-tools"

func DefaultHooksPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".gemini", "config", "hooks.json"), nil
}

func HookEntry(command string) map[string]any {
	return map[string]any{
		"Stop": []any{map[string]any{
			"type":    "command",
			"command": command,
			"timeout": 10,
		}},
	}
}

func HookConfigured(path, command string) bool {
	data, err := readHookConfig(path)
	if err != nil {
		return false
	}
	current, ok := data[HookName]
	if !ok {
		return false
	}
	currentBytes, _ := json.Marshal(current)
	wantBytes, _ := json.Marshal(HookEntry(command))
	return string(currentBytes) == string(wantBytes)
}

// InstallHook adds only the agym Stop hook, preserving unrelated user
// settings. It refuses to overwrite an existing entry with the same name.
func InstallHook(path, command string) (bool, error) {
	data, err := readHookConfig(path)
	if err != nil {
		return false, err
	}
	entry := HookEntry(command)
	if old, exists := data[HookName]; exists {
		oldBytes, _ := json.Marshal(old)
		newBytes, _ := json.Marshal(entry)
		if string(oldBytes) == string(newBytes) {
			return false, nil
		}
		return false, errors.New("an agym hook entry already exists with different settings; inspect hooks.json before replacing it")
	}
	data[HookName] = entry
	if err := writeHookConfig(path, data); err != nil {
		return false, err
	}
	return true, nil
}

// UninstallHook removes only the agym key and leaves all other settings intact.
func UninstallHook(path string) (bool, error) {
	data, err := readHookConfig(path)
	if err != nil {
		return false, err
	}
	if _, exists := data[HookName]; !exists {
		return false, nil
	}
	delete(data, HookName)
	if len(data) == 0 {
		if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
			return false, err
		}
		return true, nil
	}
	if err := writeHookConfig(path, data); err != nil {
		return false, err
	}
	return true, nil
}

func readHookConfig(path string) (map[string]any, error) {
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return map[string]any{}, nil
	}
	if err != nil {
		return nil, err
	}
	var root map[string]any
	if err := json.Unmarshal(data, &root); err != nil {
		return nil, fmt.Errorf("agy hooks file is not valid JSON: %w", err)
	}
	if root == nil {
		return nil, errors.New("agy hooks file must contain a JSON object")
	}
	return root, nil
}

func writeHookConfig(path string, value map[string]any) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	file, err := os.CreateTemp(filepath.Dir(path), ".hooks-*.tmp")
	if err != nil {
		return err
	}
	tmp := file.Name()
	defer os.Remove(tmp)
	if err := file.Chmod(0o600); err != nil {
		_ = file.Close()
		return err
	}
	if _, err := file.Write(data); err != nil {
		_ = file.Close()
		return err
	}
	if err := file.Sync(); err != nil {
		_ = file.Close()
		return err
	}
	if err := file.Close(); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

// ShellQuote quotes an executable path for agy's `sh -c` hook invocation.
func ShellQuote(value string) string {
	return "'" + strings.ReplaceAll(value, "'", "'\\''") + "'"
}
