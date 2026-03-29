package codex

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

type AccountIdentity struct {
	Email    string
	Name     string
	AuthMode string
	LoggedIn bool
}

type authFile struct {
	AuthMode string `json:"auth_mode"`
	APIKey   string `json:"OPENAI_API_KEY"`
	Tokens   struct {
		IDToken string `json:"id_token"`
	} `json:"tokens"`
}

type idTokenClaims struct {
	Email string `json:"email"`
	Name  string `json:"name"`
}

func ReadAccountIdentity(homePath string) (AccountIdentity, error) {
	authPath := filepath.Join(strings.TrimSpace(homePath), "auth.json")
	payload, err := os.ReadFile(authPath)
	if err != nil {
		if os.IsNotExist(err) {
			return AccountIdentity{}, nil
		}
		return AccountIdentity{}, fmt.Errorf("read codex auth file %q: %w", authPath, err)
	}

	var auth authFile
	if err := json.Unmarshal(payload, &auth); err != nil {
		return AccountIdentity{}, fmt.Errorf("decode codex auth file %q: %w", authPath, err)
	}

	identity := AccountIdentity{
		AuthMode: strings.TrimSpace(auth.AuthMode),
		LoggedIn: strings.TrimSpace(auth.APIKey) != "" || strings.TrimSpace(auth.Tokens.IDToken) != "",
	}
	if strings.TrimSpace(auth.Tokens.IDToken) == "" {
		return identity, nil
	}

	claims, err := decodeIDTokenClaims(auth.Tokens.IDToken)
	if err != nil {
		return AccountIdentity{}, fmt.Errorf("decode codex id token from %q: %w", authPath, err)
	}
	identity.Email = strings.TrimSpace(claims.Email)
	identity.Name = strings.TrimSpace(claims.Name)
	return identity, nil
}

func decodeIDTokenClaims(token string) (idTokenClaims, error) {
	parts := strings.Split(strings.TrimSpace(token), ".")
	if len(parts) < 2 {
		return idTokenClaims{}, fmt.Errorf("invalid JWT format")
	}
	decoded, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return idTokenClaims{}, fmt.Errorf("decode payload: %w", err)
	}
	var claims idTokenClaims
	if err := json.Unmarshal(decoded, &claims); err != nil {
		return idTokenClaims{}, fmt.Errorf("decode claims json: %w", err)
	}
	return claims, nil
}
