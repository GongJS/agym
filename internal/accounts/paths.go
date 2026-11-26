package accounts

import "path/filepath"

const (
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

// DefaultPaths returns the platform account roster paths.
func DefaultPaths() (Paths, error) {
	base, err := defaultBaseDir()
	if err != nil {
		return Paths{}, err
	}
	return Paths{
		AccountsFile:   filepath.Join(base, accountsFilename),
		ManualOnlyFile: filepath.Join(base, manualFilename),
	}, nil
}
