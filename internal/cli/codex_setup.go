package cli

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/evanmschultz/laslig"
	"github.com/spf13/cobra"

	"github.com/evanmschultz/valv/internal/config"
	"github.com/evanmschultz/valv/internal/domain"
	"github.com/evanmschultz/valv/internal/output"
	projectdetect "github.com/evanmschultz/valv/internal/project"
	manageservice "github.com/evanmschultz/valv/internal/services/manage"
)

var errCodexSetupCanceled = errors.New("codex setup canceled")

func ensureCodexBindingReady(cmd *cobra.Command, paths config.Paths, workingDir string) error {
	service, closeStore, err := openManageService(cmd, paths)
	if err != nil {
		return fmt.Errorf("initialize manage service: %w", err)
	}
	defer closeStore()

	if _, err := service.Status(cmd.Context(), workingDir); err == nil {
		return nil
	} else if !errors.Is(err, domain.ErrUnboundProject) && !strings.Contains(err.Error(), domain.ErrUnboundProject.Error()) {
		return fmt.Errorf("validate binding: %w", err)
	}

	project, err := projectdetect.DetectFrom(workingDir)
	if err != nil {
		return fmt.Errorf("validate binding: detect project from %q: %w", workingDir, err)
	}
	if !commandHasTTY(cmd.InOrStdin()) || !commandHasTTY(cmd.OutOrStdout()) {
		return fmt.Errorf("validate binding: project %q: %w; run `valv manage account add codex` for the default host-backed account or `valv manage account add codex account-name` for an isolated account", project.Root, domain.ErrUnboundProject)
	}
	if err := runCodexFirstRunSetup(cmd, service, project.Root); err != nil {
		return fmt.Errorf("validate binding: %w", err)
	}
	return nil
}

func runCodexFirstRunSetup(cmd *cobra.Command, service manageservice.Service, projectRoot string) error {
	reader := bufio.NewReader(cmd.InOrStdin())
	for {
		if err := writeCodexSetupIntro(cmd.ErrOrStderr(), projectRoot); err != nil {
			return fmt.Errorf("write setup intro: %w", err)
		}
		selection, err := readPrompt(reader, cmd.ErrOrStderr(), "Select [1-4]: ")
		if err != nil {
			if errors.Is(err, io.EOF) {
				return errCodexSetupCanceled
			}
			return fmt.Errorf("read setup selection: %w", err)
		}
		switch selection {
		case "1", "":
			profile, err := service.CreateDefaultHostProfile(cmd.Context(), domain.ProviderCodex)
			if err != nil {
				return fmt.Errorf("prepare default host-backed account: %w", err)
			}
			return loginBindAndReportCodexSetup(cmd, service, projectRoot, profile)
		case "2":
			profiles, err := service.ListProfiles(cmd.Context(), domain.ProviderCodex)
			if err != nil {
				return fmt.Errorf("list existing accounts: %w", err)
			}
			selected, err := pickProfile(cmd, domain.ProviderCodex, profiles.Profiles)
			if err != nil {
				if errors.Is(err, errSelectionCanceled) {
					return errCodexSetupCanceled
				}
				if strings.Contains(err.Error(), "no codex accounts found") {
					_ = writeCLINotice(
						cmd.ErrOrStderr(),
						laslig.NoticeWarningLevel,
						"No existing Codex accounts are available yet",
						"Create one or use the default account flow first.",
					)
					continue
				}
				return fmt.Errorf("select existing account: %w", err)
			}
			account, err := service.ProfileByName(cmd.Context(), domain.ProviderCodex, selected)
			if err != nil {
				return fmt.Errorf("resolve existing account %q: %w", selected, err)
			}
			return loginBindAndReportCodexSetup(cmd, service, projectRoot, account)
		case "3":
			name, err := readPrompt(reader, cmd.ErrOrStderr(), "New isolated account name: ")
			if err != nil {
				if errors.Is(err, io.EOF) {
					return errCodexSetupCanceled
				}
				return fmt.Errorf("read isolated account name: %w", err)
			}
			name = strings.TrimSpace(name)
			if name == "" {
				_ = writeCLINotice(
					cmd.ErrOrStderr(),
					laslig.NoticeErrorLevel,
					"Account name cannot be empty",
					"Enter a non-empty isolated account name.",
				)
				continue
			}
			profile, err := service.CreateProfile(cmd.Context(), domain.ProviderCodex, name, "")
			if err != nil {
				return fmt.Errorf("create isolated account %q: %w", name, err)
			}
			return loginBindAndReportCodexSetup(cmd, service, projectRoot, profile)
		case "4", "q", "quit", "cancel", "esc":
			return errCodexSetupCanceled
		default:
			_ = writeCLINotice(
				cmd.ErrOrStderr(),
				laslig.NoticeWarningLevel,
				"Unknown selection",
				fmt.Sprintf("Choose one of 1-4, got %q.", selection),
			)
		}
	}
}

func loginBindAndReportCodexSetup(cmd *cobra.Command, service manageservice.Service, projectRoot string, profile domain.Profile) error {
	if err := ensureManagedAccountReady(cmd, profile.Provider, profile, accountAuthOptions{}); err != nil {
		return fmt.Errorf("prepare account %q: %w", profile.Name, err)
	}
	result, err := service.BindProject(cmd.Context(), profile.Provider, profile.Name, projectRoot)
	if err != nil {
		return fmt.Errorf("bind project to account %q: %w", profile.Name, err)
	}
	return writeCodexSetupResult(cmd.ErrOrStderr(), "Account ready and project bound", result.Project.Root, result.Profile)
}

func writeCodexSetupIntro(out io.Writer, projectRoot string) error {
	return writeCLINotice(
		out,
		laslig.NoticeInfoLevel,
		"Valv setup needed",
		fmt.Sprintf("project: %s", projectRoot),
		"1. Use the default Codex account for this environment and bind it",
		"2. Choose an existing Valv account",
		"3. Create a new isolated Valv account",
		"4. Cancel",
	)
}

func writeCodexSetupResult(out io.Writer, heading string, projectRoot string, profile domain.Profile) error {
	mode := output.ResolveMode(out, output.Policy{Format: domain.OutputFormatAuto, Style: domain.OutputStyleAuto})
	return output.WriteRecord(out, mode, heading, []output.Field{
		{Label: "project", Value: projectRoot, Identifier: true},
		{Label: "provider", Value: string(profile.Provider), Muted: true},
		{Label: "account", Value: profile.Name, Identifier: true},
		{Label: "home", Value: profile.HomePath},
	})
}

func readPrompt(reader *bufio.Reader, out io.Writer, prompt string) (string, error) {
	if _, err := fmt.Fprint(out, prompt); err != nil {
		return "", fmt.Errorf("write prompt: %w", err)
	}
	line, err := reader.ReadString('\n')
	if err != nil && !errors.Is(err, io.EOF) {
		return "", err
	}
	if errors.Is(err, io.EOF) && strings.TrimSpace(line) == "" {
		return "", io.EOF
	}
	return strings.TrimSpace(line), nil
}
