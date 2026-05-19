package cli

import (
	"reflect"
	"testing"
)

func TestStripAccountFlag(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name            string
		args            []string
		wantAccountName string
		wantRemaining   []string
	}{
		{
			name:            "no flag present",
			args:            []string{"resume", "--last"},
			wantAccountName: "",
			wantRemaining:   []string{"resume", "--last"},
		},
		{
			name:            "nil args",
			args:            nil,
			wantAccountName: "",
			wantRemaining:   nil,
		},
		{
			name:            "empty args",
			args:            []string{},
			wantAccountName: "",
			wantRemaining:   []string{},
		},
		{
			name:            "--account space-separated",
			args:            []string{"--account", "work"},
			wantAccountName: "work",
			wantRemaining:   []string{},
		},
		{
			name:            "--account equals form",
			args:            []string{"--account=work"},
			wantAccountName: "work",
			wantRemaining:   []string{},
		},
		{
			name:            "--account as last token (malformed)",
			args:            []string{"--account"},
			wantAccountName: "",
			wantRemaining:   []string{"--account"},
		},
		{
			name:            "--account= empty value (malformed)",
			args:            []string{"--account="},
			wantAccountName: "",
			wantRemaining:   []string{"--account="},
		},
		{
			name:            "--account in the middle of other args",
			args:            []string{"resume", "--account", "work", "--last"},
			wantAccountName: "work",
			wantRemaining:   []string{"resume", "--last"},
		},
		{
			name:            "--account=work in the middle of other args",
			args:            []string{"resume", "--account=work", "--last"},
			wantAccountName: "work",
			wantRemaining:   []string{"resume", "--last"},
		},
		{
			name:            "multiple --account flags (first wins, second preserved)",
			args:            []string{"--account", "first", "--account", "second"},
			wantAccountName: "first",
			wantRemaining:   []string{"--account", "second"},
		},
		{
			name:            "multiple --account= flags (first wins, second preserved)",
			args:            []string{"--account=first", "--account=second"},
			wantAccountName: "first",
			wantRemaining:   []string{"--account=second"},
		},
		{
			name:            "-- terminator stops scan (account after -- preserved)",
			args:            []string{"--", "--account", "work"},
			wantAccountName: "",
			wantRemaining:   []string{"--", "--account", "work"},
		},
		{
			name:            "-- terminator stops scan (account= after -- preserved)",
			args:            []string{"--", "--account=work"},
			wantAccountName: "",
			wantRemaining:   []string{"--", "--account=work"},
		},
		{
			name:            "--account before -- is extracted",
			args:            []string{"--account", "work", "--", "passthrough"},
			wantAccountName: "work",
			wantRemaining:   []string{"--", "passthrough"},
		},
		{
			name:            "leading and trailing unrelated args preserved in order",
			args:            []string{"exec", "--model", "o4", "--account", "hylla", "--quiet"},
			wantAccountName: "hylla",
			wantRemaining:   []string{"exec", "--model", "o4", "--quiet"},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			gotName, gotRemaining := stripAccountFlag(tc.args)
			if gotName != tc.wantAccountName {
				t.Errorf("stripAccountFlag() accountName = %q, want %q", gotName, tc.wantAccountName)
			}
			if !reflect.DeepEqual(gotRemaining, tc.wantRemaining) {
				t.Errorf("stripAccountFlag() remaining = %v, want %v", gotRemaining, tc.wantRemaining)
			}
		})
	}
}

// TestStripAccountFlagDoesNotMutateInputSlice verifies that the input args
// slice is not modified when account extraction occurs.
func TestStripAccountFlagDoesNotMutateInputSlice(t *testing.T) {
	t.Parallel()

	original := []string{"--account", "work", "--last"}
	snapshot := append([]string(nil), original...)

	stripAccountFlag(original)

	if !reflect.DeepEqual(original, snapshot) {
		t.Errorf("stripAccountFlag() mutated input: got %v, want %v", original, snapshot)
	}
}
