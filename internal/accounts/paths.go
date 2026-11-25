package accounts

import (
	"os"
	"path/filepath"
)

const (
	accountsDirectory = "Library/Application Support/sparkle"
	accountsFilename  = "agy-accounts.txt"
	manualFilename    = "agy-manual-only.txt"

	liveSlotService   = "gemini"
	liveSlotAccount   = "antigravity"
	backupSlotService = "gemini-agy-backup"
)

// Paths points to the on-disk account roster files. The format intentionally
// matches the existing Python implementation so both versions can coexist.
type Paths struct {
	AccountsFile   string
	ManualOnlyFile string
}

// DefaultPaths returns the established macOS account roster paths under the
// current user's home directory.
func DefaultPaths() (Paths, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return Paths{}, err
	}
	base := filepath.Join(home, accountsDirectory)
	return Paths{
		AccountsFile:   filepath.Join(base, accountsFilename),
		ManualOnlyFile: filepath.Join(base, manualFilename),
	}, nil
}
