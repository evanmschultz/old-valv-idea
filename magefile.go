//go:build mage

package main

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"github.com/evanmschultz/laslig"
	"github.com/evanmschultz/laslig/gotestout"
	"github.com/magefile/mage/mg"
)

const (
	// TODO: restore to 70.0 after raising internal/adapters/docker coverage (see main/REFINEMENTS.md).
	coverageThreshold = 60.0
	localBuildVCSFlag = "-buildvcs=false"
	devHomeFile       = ".tmp/dev-home.path"
	devImageRepo      = "valv-codex-dev"
	devImageTag       = "dev"
	devImage          = devImageRepo + ":" + devImageTag
)

var coverageLinePattern = regexp.MustCompile(`^(?:ok\s+)?(\S+)(?:\s+\S+)?\s+coverage:\s+([0-9.]+)% of statements(?: in ./\.\.\.)?$`)

type coverageRow struct {
	Package string
	Percent float64
}

type coverageReport struct {
	Rows []coverageRow
}

type goTestRunOptions struct {
	CaptureCoverage  bool
	DisabledSections []gotestout.Section
}

// Aliases preserves the familiar hyphenated task names while keeping the visible target list small.
var Aliases = map[string]interface{}{
	"check":         CI,
	"fmt":           Format,
	"fmt-check":     FormatCheck,
	"format-check":  FormatCheck,
	"format-file":   FormatFile,
	"test-func":     TestFunc,
	"test-pkg":      TestPkg,
	"race-pkg":      RacePkg,
	"vet-pkg":       VetPkg,
	"golden-update": GoldenUpdate,
}

// Build compiles the local valv binary.
func Build() error {
	printer := newMagePrinter(os.Stdout)
	if err := printer.StatusLine(laslig.StatusLine{
		Level:  laslig.NoticeInfoLevel,
		Text:   "Building valv",
		Detail: "./cmd/valv",
	}); err != nil {
		return fmt.Errorf("write build start: %w", err)
	}
	if err := runGo("build", "-o", "./valv", "./cmd/valv"); err != nil {
		return err
	}
	if err := printer.StatusLine(laslig.StatusLine{
		Level:  laslig.NoticeSuccessLevel,
		Text:   "Built valv",
		Detail: "./valv",
	}); err != nil {
		return fmt.Errorf("write build success: %w", err)
	}
	return nil
}

// BuildProxy builds the valv-proxy sidecar Docker image and loads it into the
// local daemon as valv-proxy:dev. The build context is the repository root so
// the Dockerfile at internal/cmd/valv-proxy/Dockerfile can COPY the compiled
// binary from the multi-stage builder stage.
func BuildProxy() error {
	return run("docker", "buildx", "build", "--load",
		"-f", "internal/cmd/valv-proxy/Dockerfile",
		".", "-t", "valv-proxy:dev")
}

// Install installs the valv binary to $GOBIN (or $GOPATH/bin).
func Install() error {
	printer := newMagePrinter(os.Stdout)
	if err := printer.StatusLine(laslig.StatusLine{
		Level:  laslig.NoticeInfoLevel,
		Text:   "Installing valv",
		Detail: "./cmd/valv",
	}); err != nil {
		return fmt.Errorf("write install start: %w", err)
	}
	if err := runGo("install", "./cmd/valv"); err != nil {
		return err
	}
	if err := printer.StatusLine(laslig.StatusLine{
		Level:  laslig.NoticeSuccessLevel,
		Text:   "Installed valv",
		Detail: "$GOBIN",
	}); err != nil {
		return fmt.Errorf("write install success: %w", err)
	}
	return nil
}

// CI is the canonical local verification gate: bootstrap check, FormatCheck,
// Vet, Tests (race + coverage + per-package threshold), Tidy. Renamed from the
// prior `Test` gate per the canonical 12-target shape (2026-05-30); the `check`
// alias is preserved. Vet + Tidy are NEW stages (valv had neither before) —
// they may surface preexisting issues on first run; see R_SHIP_HANDOFF.md.
func CI() error {
	printer := newMagePrinter(os.Stdout)
	runStage := func(title string, fn func(*laslig.Printer) error) error {
		if err := printer.Section(title); err != nil {
			return fmt.Errorf("render %s stage: %w", title, err)
		}
		return fn(printer)
	}

	if err := verifyBootstrap(); err != nil {
		return err
	}
	if err := runStage("Format", func(*laslig.Printer) error { return checkRepoFormatting() }); err != nil {
		return err
	}
	if err := runStage("Vet", func(*laslig.Printer) error { return runGo("vet", "./...") }); err != nil {
		return err
	}
	if err := runStage("Tests", runRepoTests); err != nil {
		return err
	}
	if err := runStage("Tidy", func(*laslig.Printer) error { return tidyCheck() }); err != nil {
		return err
	}
	return nil
}

// Test runs `go test -count=1 ./...` over every package (no race, no coverage).
// Closeout/orchestrator surface — fastest all-package gate.
func Test() error {
	return runGoTest("-count=1", "./...")
}

// TestPkg runs `go test -count=1 <pkg>` for ONE package pattern (no race).
// Plan-QA read-only surface — verifies a code claim against a single package.
func TestPkg(pkg string) error {
	pkg = strings.TrimSpace(pkg)
	if pkg == "" {
		return errors.New("testPkg requires one package pattern, for example: mage testPkg ./internal/output")
	}
	return runGoTest("-count=1", pkg)
}

// TestFunc runs `go test -run "^<testName>$" -race -count=1 <pkg>` — ONE named
// test function in ONE package with race detection. Builder + build-QA surface.
func TestFunc(pkg, testName string) error {
	pkg = strings.TrimSpace(pkg)
	testName = strings.TrimSpace(testName)
	if pkg == "" {
		return errors.New("testFunc requires a package pattern, for example: mage testFunc ./internal/output TestMyThing")
	}
	if testName == "" {
		return errors.New("testFunc requires a test function name, for example: mage testFunc ./internal/output TestMyThing")
	}
	return runGoTest("-run", "^"+testName+"$", "-race", "-count=1", pkg)
}

// Race runs `go test -race -count=1 ./...` over every package.
// Closeout/orchestrator surface. Use RacePkg for one package.
func Race() error {
	return runGoTest("-race", "-count=1", "./...")
}

// RacePkg runs `go test -race -count=1 <pkg>` for ONE package. Build-QA surface.
func RacePkg(pkg string) error {
	pkg = strings.TrimSpace(pkg)
	if pkg == "" {
		return errors.New("racePkg requires one package pattern, for example: mage racePkg ./internal/output")
	}
	return runGoTest("-race", "-count=1", pkg)
}

// Cover runs the full suite with race + coverage and enforces the per-package
// threshold. This is the CI gate's Tests stage exposed as a standalone target.
func Cover() error {
	printer := newMagePrinter(os.Stdout)
	if err := printer.Section("Tests"); err != nil {
		return fmt.Errorf("render cover stage: %w", err)
	}
	return runRepoTests(printer)
}

// Format rewrites tracked Go files in place with `go tool gofumpt -w`.
// Closeout/orch surface. Use FormatFile for a single file or directory.
func Format() error {
	files, err := goFiles(".")
	if err != nil {
		return err
	}
	if len(files) == 0 {
		return nil
	}
	args := append([]string{"tool", "gofumpt", "-w"}, files...)
	return runGo(args...)
}

// FormatFile rewrites ONE file (or directory) with `go tool gofumpt -w`.
// Builder + build-QA surface — formats only the file(s) just edited.
func FormatFile(path string) error {
	path = strings.TrimSpace(path)
	if path == "" {
		return errors.New("formatFile requires a path, for example: mage formatFile internal/output/foo.go")
	}
	if _, err := os.Stat(path); err != nil {
		return fmt.Errorf("formatFile %q: %w", path, err)
	}
	return runGo("tool", "gofumpt", "-w", path)
}

// FormatCheck fails if any tracked Go file is not gofumpt-clean.
func FormatCheck() error {
	return checkRepoFormatting()
}

// Vet runs `go vet ./...`.
func Vet() error {
	return runGo("vet", "./...")
}

// VetPkg runs `go vet <pkg>` over ONE package. Builder + build-QA surface.
func VetPkg(pkg string) error {
	pkg = strings.TrimSpace(pkg)
	if pkg == "" {
		return errors.New("vetPkg requires one package pattern, for example: mage vetPkg ./internal/output")
	}
	return runGo("vet", pkg)
}

// Tidy runs `go mod tidy` and fails if go.mod or go.sum changed.
func Tidy() error {
	return tidyCheck()
}

// Integration runs the Docker-backed integration and external golden tests.
func Integration() error {
	return runGoTest("-tags=integration", "-count=1", "./internal/cli", "./internal/services/images")
}

// Golden runs the full golden suite.
func Golden() error {
	printer := newMagePrinter(os.Stdout)
	runStage := func(title string, args ...string) error {
		if err := printer.Section(title); err != nil {
			return fmt.Errorf("render %s golden stage: %w", title, err)
		}
		return runGoTest(args...)
	}

	if err := runStage("Tracked goldens", "-count=1", "./internal/output", "./internal/tui/manage"); err != nil {
		return err
	}
	return runStage("External transcript", "-tags=integration", "-count=1", "./internal/cli", "-run", "TestCodexInteractiveMCPGolden")
}

// Run builds the binary and runs it with one quoted argument string.
func Run(args string) error {
	if err := Build(); err != nil {
		return err
	}
	return runValv(nil, args)
}

// GoldenUpdate refreshes all tracked golden fixtures.
func GoldenUpdate() error {
	printer := newMagePrinter(os.Stdout)
	runStage := func(title string, args ...string) error {
		if err := printer.Section(title); err != nil {
			return fmt.Errorf("render %s golden update stage: %w", title, err)
		}
		return runGoTest(args...)
	}

	if err := runStage("Tracked goldens", "-count=1", "./internal/output", "./internal/tui/manage", "-args", "-update"); err != nil {
		return err
	}
	return runStage("External transcript", "-tags=integration", "-count=1", "./internal/cli", "-run", "TestCodexInteractiveMCPGolden", "-args", "-update")
}

func runRepoTests(printer *laslig.Printer) error {
	report, err := runGoTestWithOptions(goTestRunOptions{
		CaptureCoverage: true,
		DisabledSections: []gotestout.Section{
			gotestout.SectionSkippedTests,
		},
	}, "-count=1", "-race", "-cover", "./...")
	if err != nil {
		return err
	}
	return renderCoverage(printer, report, coverageThreshold)
}

// Dev groups disposable dev-home automation.
type Dev mg.Namespace

// Home prints the current disposable dev-home path and usage hints.
func (Dev) Home() error {
	devHome, err := ensureDevHome()
	if err != nil {
		return err
	}
	return printDevHomeMessage("Disposable dev home", devHome)
}

// Reset recreates the disposable dev-home path.
func (Dev) Reset() error {
	devHome, err := resetDevHome()
	if err != nil {
		return err
	}
	return printDevHomeMessage("Disposable dev home reset", devHome)
}

// Clean removes the disposable dev-home path and dev-tagged images.
func (Dev) Clean() error {
	if err := removeDevHome(); err != nil {
		return err
	}
	images, err := listDevImages()
	if err != nil {
		return err
	}
	if len(images) == 0 {
		return nil
	}
	args := append([]string{"image", "rm", "--force"}, images...)
	return run("docker", args...)
}

// Run builds the binary and runs it with one quoted argument string inside the disposable dev-home environment.
func (Dev) Run(args string) error {
	if err := Build(); err != nil {
		return err
	}
	devHome, err := ensureDevHome()
	if err != nil {
		return err
	}
	hostHome, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("resolve host home: %w", err)
	}
	return runValv([]string{
		"HOME=" + devHome,
		"DOCKER_CONFIG=" + filepath.Join(hostHome, ".docker"),
		"VALV_REAL_HOME=" + hostHome,
		"VALV_CODEX_IMAGE=" + devImage,
	}, args)
}

func verifyBootstrap() error {
	required := []string{
		"CONTRIBUTING.md",
		"README.md",
		"go.mod",
		"magefile.go",
		".github/workflows/ci.yml",
	}
	for _, path := range required {
		if _, err := os.Stat(path); err != nil {
			return fmt.Errorf("verify bootstrap %q: %w", path, err)
		}
	}
	return nil
}

func checkRepoFormatting() error {
	files, err := goFiles(".")
	if err != nil {
		return err
	}
	return checkGofumpt(files)
}

func checkGofumpt(files []string) error {
	if len(files) == 0 {
		return nil
	}
	args := append([]string{"tool", "gofumpt", "-l"}, files...)
	out, err := outputWithEnv(goEnv(), "go", args...)
	if err != nil {
		return err
	}
	if strings.TrimSpace(out) != "" {
		return fmt.Errorf("gofumpt required for:\n%s", strings.TrimSpace(out))
	}
	return nil
}

func goFiles(root string) ([]string, error) {
	var files []string
	err := filepath.WalkDir(root, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			switch entry.Name() {
			case ".git", ".tmp", ".worklog":
				return filepath.SkipDir
			}
			return nil
		}
		if filepath.Ext(path) == ".go" {
			files = append(files, path)
		}
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("list go files: %w", err)
	}
	return files, nil
}

func goEnv() []string {
	env := os.Environ()
	flags := strings.TrimSpace(os.Getenv("GOFLAGS"))
	switch {
	case flags == "":
		flags = localBuildVCSFlag
	case strings.Contains(flags, localBuildVCSFlag):
	default:
		flags += " " + localBuildVCSFlag
	}
	return append(env, "GOFLAGS="+flags)
}

func runGo(args ...string) error {
	return runWithEnv(goEnv(), "go", args...)
}

// tidyCheck runs `go mod tidy` and fails if go.mod or go.sum drifted. Shared by
// the Tidy target and the CI gate's Tidy stage.
func tidyCheck() error {
	before, err := snapshotFiles("go.mod", "go.sum")
	if err != nil {
		return err
	}
	if err := runGo("mod", "tidy"); err != nil {
		return err
	}
	after, err := snapshotFiles("go.mod", "go.sum")
	if err != nil {
		return err
	}
	if before != after {
		return errors.New("go.mod or go.sum changed; commit the tidy result")
	}
	return nil
}

// snapshotFiles concatenates the given files' contents for drift comparison.
// Missing files (e.g. go.sum on a freshly-tidied module) are treated as empty.
func snapshotFiles(paths ...string) (string, error) {
	var b strings.Builder
	for _, p := range paths {
		data, err := os.ReadFile(p)
		if err != nil {
			if errors.Is(err, os.ErrNotExist) {
				continue
			}
			return "", fmt.Errorf("snapshot %s: %w", p, err)
		}
		b.WriteString(p)
		b.WriteByte('\n')
		b.Write(data)
		b.WriteByte('\n')
	}
	return b.String(), nil
}

func runValv(extraEnv []string, argsLine string) error {
	args := splitArgs(argsLine)
	return runWithEnv(extraEnv, "./valv", args...)
}

func splitArgs(line string) []string {
	line = strings.TrimSpace(line)
	if line == "" {
		return nil
	}
	return strings.Fields(line)
}

func run(command string, args ...string) error {
	return runWithEnv(nil, command, args...)
}

func runWithEnv(extraEnv []string, command string, args ...string) error {
	cmd := exec.Command(command, args...)
	cmd.Env = append(os.Environ(), extraEnv...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Stdin = os.Stdin
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("run %s %s: %w", command, strings.Join(args, " "), err)
	}
	return nil
}

func output(command string, args ...string) (string, error) {
	return outputWithEnv(nil, command, args...)
}

func outputWithEnv(extraEnv []string, command string, args ...string) (string, error) {
	cmd := exec.Command(command, args...)
	cmd.Env = append(os.Environ(), extraEnv...)
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
	if err != nil {
		text := strings.TrimSpace(stderr.String())
		if text == "" {
			text = strings.TrimSpace(stdout.String())
		}
		if text == "" {
			return "", fmt.Errorf("run %s %s: %w", command, strings.Join(args, " "), err)
		}
		return "", fmt.Errorf("run %s %s: %w\n%s", command, strings.Join(args, " "), err, text)
	}
	return stdout.String(), nil
}

func newMagePrinter(out io.Writer) *laslig.Printer {
	return laslig.New(out, laslig.Policy{
		Format: laslig.FormatAuto,
		Style:  laslig.StyleAuto,
	})
}

func runGoTest(args ...string) error {
	_, err := runGoTestWithOptions(goTestRunOptions{}, args...)
	return err
}

func runGoTestWithOptions(options goTestRunOptions, args ...string) (coverageReport, error) {
	goArgs := append([]string{"test", "-json"}, args...)
	printer := newMagePrinter(os.Stdout)
	cmd := exec.Command("go", goArgs...)
	cmd.Env = goEnv()
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return coverageReport{}, fmt.Errorf("create go test stdout pipe: %w", err)
	}
	cmd.Stderr = os.Stderr
	cmd.Stdin = os.Stdin

	if err := cmd.Start(); err != nil {
		return coverageReport{}, fmt.Errorf("start go test: %w", err)
	}
	if err := printer.StatusLine(laslig.StatusLine{
		Level:  laslig.NoticeInfoLevel,
		Text:   "Started go test -json",
		Detail: strings.Join(args, " "),
	}); err != nil {
		return coverageReport{}, fmt.Errorf("write go test start status: %w", err)
	}

	reader := io.Reader(stdout)
	var raw bytes.Buffer
	if options.CaptureCoverage {
		reader = io.TeeReader(reader, &raw)
	}

	summary, renderErr := gotestout.Render(os.Stdout, reader, gotestout.Options{
		DisabledSections: options.DisabledSections,
		Activity: gotestout.ActivityOptions{
			Mode: gotestout.ActivityAuto,
		},
	})
	waitErr := cmd.Wait()

	if renderErr != nil {
		return coverageReport{}, fmt.Errorf("render go test output: %w", renderErr)
	}
	if waitErr != nil {
		return coverageReport{}, fmt.Errorf("go test %s: %w", strings.Join(args, " "), waitErr)
	}
	if summary.HasFailures() {
		return coverageReport{}, fmt.Errorf("go test %s: test summary reported failures", strings.Join(args, " "))
	}

	report := coverageReport{}
	if options.CaptureCoverage {
		report, err = parseCoverageReport(raw.Bytes())
		if err != nil {
			return coverageReport{}, err
		}
	}
	return report, nil
}

func parseCoverageReport(raw []byte) (coverageReport, error) {
	events, err := gotestout.Parse(bytes.NewReader(raw))
	if err != nil {
		return coverageReport{}, fmt.Errorf("parse go test event stream: %w", err)
	}

	seen := make(map[string]int)
	rows := make([]coverageRow, 0)
	for _, event := range events {
		if event.Action != gotestout.ActionOutput || event.Test != "" {
			continue
		}
		pkg, percent, ok, err := parseCoverageLine(event.Package, event.Output)
		if err != nil {
			return coverageReport{}, err
		}
		if !ok {
			continue
		}
		if index, exists := seen[pkg]; exists {
			rows[index].Percent = percent
			continue
		}
		seen[pkg] = len(rows)
		rows = append(rows, coverageRow{
			Package: pkg,
			Percent: percent,
		})
	}
	if len(rows) == 0 {
		return coverageReport{}, errors.New("no coverage rows were parsed from go test output")
	}
	return coverageReport{Rows: rows}, nil
}

func parseCoverageLine(defaultPackage, line string) (string, float64, bool, error) {
	line = strings.TrimSpace(line)
	if line == "" {
		return "", 0, false, nil
	}

	match := coverageLinePattern.FindStringSubmatch(line)
	if match == nil {
		return "", 0, false, nil
	}

	percent, err := strconv.ParseFloat(match[2], 64)
	if err != nil {
		return "", 0, false, fmt.Errorf("parse coverage for %q: %w", match[1], err)
	}

	pkg := strings.TrimSpace(match[1])
	if pkg == "coverage:" || pkg == "ok" || pkg == "" {
		pkg = strings.TrimSpace(defaultPackage)
	}
	if pkg == "" {
		return "", 0, false, nil
	}
	return pkg, percent, true, nil
}

func renderCoverage(printer *laslig.Printer, report coverageReport, threshold float64) error {
	rows := make([][]string, 0, len(report.Rows))
	var belowThreshold []string
	for _, row := range report.Rows {
		rows = append(rows, []string{row.Package, fmt.Sprintf("%.1f%%", row.Percent)})
		if row.Percent < threshold {
			belowThreshold = append(belowThreshold, fmt.Sprintf("%s=%.1f%%", row.Package, row.Percent))
		}
	}

	if err := printer.Table(laslig.Table{
		Header:  []string{"package", "cover"},
		Rows:    rows,
		Caption: fmt.Sprintf("Minimum package coverage: %.1f%%.", threshold),
	}); err != nil {
		return fmt.Errorf("write coverage table: %w", err)
	}
	if len(belowThreshold) > 0 {
		if err := printer.Notice(laslig.Notice{
			Level: laslig.NoticeErrorLevel,
			Title: "Coverage threshold not met",
			Body:  fmt.Sprintf("Each package must stay at or above %.1f%% coverage.", threshold),
			Detail: []string{
				strings.Join(belowThreshold, ", "),
			},
		}); err != nil {
			return fmt.Errorf("write coverage failure notice: %w", err)
		}
		return fmt.Errorf("coverage below %.1f%% for: %s", threshold, strings.Join(belowThreshold, ", "))
	}
	if err := printer.Notice(laslig.Notice{
		Level: laslig.NoticeSuccessLevel,
		Title: "Coverage threshold met",
		Body:  fmt.Sprintf("All packages are at or above %.1f%% coverage.", threshold),
	}); err != nil {
		return fmt.Errorf("write coverage success notice: %w", err)
	}
	return nil
}

func ensureDevHome() (string, error) {
	if err := os.MkdirAll(filepath.Dir(devHomeFile), 0o755); err != nil {
		return "", fmt.Errorf("create dev-home parent: %w", err)
	}
	if home, ok, err := readDevHome(); err != nil {
		return "", err
	} else if ok {
		if err := os.MkdirAll(home, 0o755); err != nil {
			return "", fmt.Errorf("ensure dev home %q: %w", home, err)
		}
		return home, nil
	}
	return createDevHome()
}

func resetDevHome() (string, error) {
	if err := removeDevHome(); err != nil {
		return "", err
	}
	return createDevHome()
}

func removeDevHome() error {
	home, ok, err := readDevHome()
	if err != nil {
		return err
	}
	if !ok {
		return nil
	}
	if err := os.RemoveAll(home); err != nil {
		return fmt.Errorf("remove dev home %q: %w", home, err)
	}
	if err := os.Remove(devHomeFile); err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("remove dev-home file: %w", err)
	}
	return nil
}

func createDevHome() (string, error) {
	home, err := os.MkdirTemp("", "valv-dev.*")
	if err != nil {
		return "", fmt.Errorf("create dev home: %w", err)
	}
	if err := os.WriteFile(devHomeFile, []byte(home+"\n"), 0o644); err != nil {
		return "", fmt.Errorf("write dev-home file: %w", err)
	}
	return home, nil
}

func readDevHome() (string, bool, error) {
	data, err := os.ReadFile(devHomeFile)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return "", false, nil
		}
		return "", false, fmt.Errorf("read dev-home file: %w", err)
	}
	home := strings.TrimSpace(string(data))
	if home == "" {
		return "", false, nil
	}
	return home, true, nil
}

func listDevImages() ([]string, error) {
	out, err := output("docker", "image", "ls", devImageRepo, "--format", "{{.Repository}}:{{.Tag}}")
	if err != nil {
		return nil, err
	}
	lines := strings.Split(strings.TrimSpace(out), "\n")
	images := make([]string, 0, len(lines))
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		images = append(images, line)
	}
	return images, nil
}

func printDevHomeMessage(title, home string) error {
	printer := newMagePrinter(os.Stdout)
	return printer.Record(laslig.Record{
		Title: title,
		Fields: []laslig.Field{
			{Label: "home", Value: home, Identifier: true},
			{Label: "use", Value: `mage dev:run "..."`, Identifier: true},
			{Label: "cleanup", Value: "mage dev:reset or mage dev:clean", Muted: true},
			{Label: "bootstrap", Value: `mage dev:run "image update"`, Identifier: true},
		},
	})
}
