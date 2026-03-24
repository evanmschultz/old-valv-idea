package domain

import "testing"

func TestParseOutputFormat(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		input   string
		want    OutputFormat
		wantErr bool
	}{
		{name: "auto", input: "auto", want: OutputFormatAuto},
		{name: "human trimmed", input: " Human ", want: OutputFormatHuman},
		{name: "plain", input: "plain", want: OutputFormatPlain},
		{name: "json", input: "json", want: OutputFormatJSON},
		{name: "invalid", input: "yaml", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got, err := ParseOutputFormat(tt.input)
			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error")
				}
				return
			}
			if err != nil {
				t.Fatalf("ParseOutputFormat() error = %v", err)
			}
			if got != tt.want {
				t.Fatalf("ParseOutputFormat() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestParseOutputStyle(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		input   string
		want    OutputStyle
		wantErr bool
	}{
		{name: "auto", input: "auto", want: OutputStyleAuto},
		{name: "always", input: "always", want: OutputStyleAlways},
		{name: "never", input: " never ", want: OutputStyleNever},
		{name: "invalid", input: "sometimes", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got, err := ParseOutputStyle(tt.input)
			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error")
				}
				return
			}
			if err != nil {
				t.Fatalf("ParseOutputStyle() error = %v", err)
			}
			if got != tt.want {
				t.Fatalf("ParseOutputStyle() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestParseProvider(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		input   string
		want    Provider
		wantErr bool
	}{
		{name: "codex", input: "codex", want: ProviderCodex},
		{name: "trimmed", input: " Codex ", want: ProviderCodex},
		{name: "invalid", input: "claude", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got, err := ParseProvider(tt.input)
			if tt.wantErr {
				if err == nil {
					t.Fatal("ParseProvider() error = nil, want failure")
				}
				return
			}
			if err != nil {
				t.Fatalf("ParseProvider() error = %v", err)
			}
			if got != tt.want {
				t.Fatalf("ParseProvider() = %q, want %q", got, tt.want)
			}
		})
	}
}
