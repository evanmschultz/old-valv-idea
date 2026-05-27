package domain

import (
	"testing"
	"time"
)

func TestAccountEnvEntriesToMap(t *testing.T) {
	now := time.Now()

	tests := []struct {
		name     string
		entries  []AccountEnvEntry
		expected map[string]string
	}{
		{
			name:     "nil slice returns nil map",
			entries:  nil,
			expected: nil,
		},
		{
			name:     "empty slice returns nil map",
			entries:  []AccountEnvEntry{},
			expected: nil,
		},
		{
			name: "single entry returns single-key map",
			entries: []AccountEnvEntry{
				{
					ProfileID: "profile-1",
					EnvKey:    "API_KEY",
					EnvValue:  "secret-123",
					CreatedAt: now,
					UpdatedAt: now,
				},
			},
			expected: map[string]string{
				"API_KEY": "secret-123",
			},
		},
		{
			name: "two entries returns two-key map",
			entries: []AccountEnvEntry{
				{
					ProfileID: "profile-1",
					EnvKey:    "API_KEY",
					EnvValue:  "secret-123",
					CreatedAt: now,
					UpdatedAt: now,
				},
				{
					ProfileID: "profile-1",
					EnvKey:    "TOKEN",
					EnvValue:  "token-456",
					CreatedAt: now,
					UpdatedAt: now,
				},
			},
			expected: map[string]string{
				"API_KEY": "secret-123",
				"TOKEN":   "token-456",
			},
		},
		{
			name: "duplicate keys last-wins",
			entries: []AccountEnvEntry{
				{
					ProfileID: "profile-1",
					EnvKey:    "KEY",
					EnvValue:  "first-value",
					CreatedAt: now,
					UpdatedAt: now,
				},
				{
					ProfileID: "profile-1",
					EnvKey:    "KEY",
					EnvValue:  "second-value",
					CreatedAt: now,
					UpdatedAt: now.Add(time.Second),
				},
			},
			expected: map[string]string{
				"KEY": "second-value",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := AccountEnvEntriesToMap(tt.entries)

			// Handle nil vs empty map comparison.
			if tt.expected == nil {
				if got != nil {
					t.Errorf("expected nil map, got %v", got)
				}
				return
			}

			// Compare as maps.
			if len(got) != len(tt.expected) {
				t.Errorf("expected %d keys, got %d keys", len(tt.expected), len(got))
			}

			for k, v := range tt.expected {
				if gotVal, ok := got[k]; !ok {
					t.Errorf("expected key %q not found in map", k)
				} else if gotVal != v {
					t.Errorf("key %q: expected %q, got %q", k, v, gotVal)
				}
			}
		})
	}
}
