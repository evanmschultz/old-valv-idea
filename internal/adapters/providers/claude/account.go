package claude

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// AccountIdentity holds identity metadata for a Claude account. LoggedIn is
// set when .credentials.json is present in the account home. Email is extracted
// from the Claude config file (.config.json or .claude.json) when available;
// it is left empty when the config file is absent or does not contain an email.
type AccountIdentity struct {
	Email    string
	Name     string
	AuthMode string
	LoggedIn bool
}

// claudeConfigFile mirrors the subset of the Claude CLI config file
// (~/.claude/.claude.json or ~/.claude/.config.json) that Valv reads. The
// OAuth account section is written by the CLI after a successful auth login and
// contains account identity metadata including the user's email address.
type claudeConfigFile struct {
	OAuthAccount *claudeOAuthAccount `json:"oauthAccount,omitempty"`
}

type claudeOAuthAccount struct {
	EmailAddress string `json:"emailAddress,omitempty"`
}

// ReadAccountIdentity checks whether a Claude credentials file is present in
// homePath and extracts identity metadata from the companion config file when
// available. LoggedIn is set iff .credentials.json exists and is not a
// directory. Email is sourced from oauthAccount.emailAddress in .config.json
// (preferred) or .claude.json; if neither is present or parseable the field
// is left empty and LoggedIn is still accurate.
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

	identity := AccountIdentity{LoggedIn: true}
	identity.Email = readClaudeConfigEmail(strings.TrimSpace(homePath))
	return identity, nil
}

// readClaudeConfigEmail attempts to read the email address from the Claude CLI
// config file stored alongside .credentials.json. It checks .config.json first
// (matching the CLI's own precedence) then falls back to .claude.json. Any
// read or parse failure returns an empty string — the caller preserves LoggedIn.
func readClaudeConfigEmail(homePath string) string {
	for _, name := range []string{".config.json", ".claude.json"} {
		configPath := filepath.Join(homePath, name)
		data, err := os.ReadFile(configPath)
		if err != nil {
			if os.IsNotExist(err) {
				continue
			}
			// Unreadable for some other reason — skip gracefully.
			return ""
		}
		var cfg claudeConfigFile
		if err := json.Unmarshal(data, &cfg); err != nil {
			// Malformed config — skip gracefully.
			return ""
		}
		if cfg.OAuthAccount != nil {
			if email := strings.TrimSpace(cfg.OAuthAccount.EmailAddress); email != "" {
				return email
			}
		}
	}
	return ""
}
