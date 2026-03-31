package progress

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"github.com/evanmschultz/laslig"
)

func TestStartWithPrinterPlainFallback(t *testing.T) {
	t.Parallel()

	var buf bytes.Buffer
	spin, err := StartWithPrinter(laslig.New(&buf, laslig.Policy{Format: laslig.FormatPlain}), "Checking image")
	if err != nil {
		t.Fatalf("StartWithPrinter() error = %v", err)
	}
	if err := spin.Stop("Image ready", laslig.NoticeInfoLevel); err != nil {
		t.Fatalf("Stop() error = %v", err)
	}

	want := "\n[RUNNING] Checking image\n[INFO] Image ready\n"
	if got := buf.String(); got != want {
		t.Fatalf("spinner output = %q, want %q", got, want)
	}
}

func TestStartWithPrinterHumanNoStyleFallback(t *testing.T) {
	t.Parallel()

	var buf bytes.Buffer
	spin, err := StartWithPrinter(laslig.New(&buf, laslig.Policy{
		Format: laslig.FormatHuman,
		Style:  laslig.StyleNever,
	}), "Waiting for test output")
	if err != nil {
		t.Fatalf("StartWithPrinter() error = %v", err)
	}
	if err := spin.Stop("Streaming tests", laslig.NoticeInfoLevel); err != nil {
		t.Fatalf("Stop() error = %v", err)
	}

	got := buf.String()
	want := "\n[RUNNING] Waiting for test output\n[INFO] Streaming tests\n"
	if got != want {
		t.Fatalf("spinner output = %q, want %q", got, want)
	}
	if strings.Contains(got, "\x1b[") {
		t.Fatalf("spinner output = %q, want no ANSI", got)
	}
}

func TestStartWithPrinterJSONIsStable(t *testing.T) {
	t.Parallel()

	var buf bytes.Buffer
	spin, err := StartWithPrinter(laslig.New(&buf, laslig.Policy{Format: laslig.FormatJSON}), "Waiting for test output")
	if err != nil {
		t.Fatalf("StartWithPrinter() error = %v", err)
	}
	if err := spin.Stop("Streaming tests", laslig.NoticeInfoLevel); err != nil {
		t.Fatalf("Stop() error = %v", err)
	}

	if strings.Contains(buf.String(), "\r") {
		t.Fatalf("spinner output = %q, want no transient carriage returns", buf.String())
	}
	decoder := json.NewDecoder(strings.NewReader(buf.String()))
	var payloads []map[string]any
	for decoder.More() {
		var payload map[string]any
		if err := decoder.Decode(&payload); err != nil {
			t.Fatalf("Decode() error = %v\noutput:\n%s", err, buf.String())
		}
		payloads = append(payloads, payload)
	}
	if got, want := len(payloads), 2; got != want {
		t.Fatalf("json payload count = %d, want %d\noutput:\n%s", got, want, buf.String())
	}
	for _, payload := range payloads {
		if got := payload["type"]; got != "status_line" {
			t.Fatalf("type = %v, want status_line", got)
		}
	}
}

func TestStartUsesPolicyAndUpdateTracksLatestText(t *testing.T) {
	t.Parallel()

	var buf bytes.Buffer
	spin, err := Start(&buf, laslig.Policy{Format: laslig.FormatPlain}, "Checking image")
	if err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	if err := spin.Update("Checking image metadata"); err != nil {
		t.Fatalf("Update() error = %v", err)
	}
	if err := spin.Stop("", laslig.NoticeInfoLevel); err != nil {
		t.Fatalf("Stop() error = %v", err)
	}

	want := "\n[RUNNING] Checking image\n[INFO] Checking image metadata\n"
	if got := buf.String(); got != want {
		t.Fatalf("spinner output = %q, want %q", got, want)
	}
}

func TestSpinnerNilMethodsAreSafe(t *testing.T) {
	t.Parallel()

	var spin *Spinner
	if err := spin.Update("ignored"); err != nil {
		t.Fatalf("Update() error = %v, want nil", err)
	}
	if err := spin.Stop("ignored", laslig.NoticeInfoLevel); err != nil {
		t.Fatalf("Stop() error = %v, want nil", err)
	}
}
