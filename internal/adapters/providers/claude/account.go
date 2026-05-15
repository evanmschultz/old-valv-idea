package claude

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// AccountIdentity holds identity metadata for a Claude account. Email, Name,
// and AuthMode remain zero-valued for v1 — Claude credentials are not parsed
// from disk. LoggedIn is set when .credentials.json is present.
type AccountIdentity struct {
	Email    string
	Name     string
	AuthMode string
	LoggedIn bool
}

// ReadAccountIdentity checks whether a Claude credentials file is present in
// homePath. It performs a presence-only check on .credentials.json and does
// NOT parse or decode the file contents.
func ReadAccountIdentity(homePath string) (AccountIdentity, error) {
	credPath := filepath.Join(strings.TrimSpace(homePath), ".credentials.json")
	info, err := os.Stat(credPath)
	if err != nil {
		if os.IsNotExist(err) {
			return AccountIdentity{}, nil
		}
		return AccountIdentity{}, fmt.Errorf("read claude credentials %q: %w", credPath, err)
	}
	if info.IsDir() {
		return AccountIdentity{}, nil
	}
	return AccountIdentity{LoggedIn: true}, nil
}
