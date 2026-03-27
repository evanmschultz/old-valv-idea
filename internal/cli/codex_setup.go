package cli

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"strings"

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
		return fmt.Errorf("validate binding: project %q: %w; run `valv manage profile add codex` for the default host-backed profile or `valv manage profile add codex profile-name` for an isolated profile", project.Root, domain.ErrUnboundProject)
	}
	if err := runCodexFirstRunSetup(cmd, service, project.Root); err != nil {
		return fmt.Errorf("validate binding: %w", err)
	}
	return nil
}

func runCodexFirstRunSetup(cmd *cobra.Command, service manageservice.Service, projectRoot string) error {
	reader := bufio.NewReader(cmd.InOrStdin())
	for {
		writeCodexSetupIntro(cmd.ErrOrStderr(), projectRoot)
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
				return fmt.Errorf("prepare default host-backed profile: %w", err)
			}
			return bindAndReportCodexSetup(cmd, service, projectRoot, profile)
		case "2":
			profiles, err := service.ListProfiles(cmd.Context(), domain.ProviderCodex)
			if err != nil {
				return fmt.Errorf("list existing profiles: %w", err)
			}
			selected, err := pickProfile(cmd, domain.ProviderCodex, profiles.Profiles)
			if err != nil {
				if errors.Is(err, errSelectionCanceled) {
					return errCodexSetupCanceled
				}
				if strings.Contains(err.Error(), "no codex profiles found") {
					_, _ = fmt.Fprintln(cmd.ErrOrStderr(), "No existing Codex profiles are available yet.")
					continue
				}
				return fmt.Errorf("select existing profile: %w", err)
			}
			result, bindErr := service.BindProject(cmd.Context(), domain.ProviderCodex, selected, projectRoot)
			if bindErr != nil {
				return fmt.Errorf("bind existing profile %q: %w", selected, bindErr)
			}
			return writeCodexSetupResult(cmd.ErrOrStderr(), "Project binding updated", result.Project.Root, result.Profile)
		case "3":
			name, err := readPrompt(reader, cmd.ErrOrStderr(), "New isolated profile name: ")
			if err != nil {
				if errors.Is(err, io.EOF) {
					return errCodexSetupCanceled
				}
				return fmt.Errorf("read isolated profile name: %w", err)
			}
			name = strings.TrimSpace(name)
			if name == "" {
				_, _ = fmt.Fprintln(cmd.ErrOrStderr(), "Profile name cannot be empty.")
				continue
			}
			profile, err := service.CreateProfile(cmd.Context(), domain.ProviderCodex, name, "")
			if err != nil {
				return fmt.Errorf("create isolated profile %q: %w", name, err)
			}
			return bindAndReportCodexSetup(cmd, service, projectRoot, profile)
		case "4", "q", "quit", "cancel", "esc":
			return errCodexSetupCanceled
		default:
			_, _ = fmt.Fprintf(cmd.ErrOrStderr(), "Unknown selection %q.\n", selection)
		}
	}
}

func bindAndReportCodexSetup(cmd *cobra.Command, service manageservice.Service, projectRoot string, profile domain.Profile) error {
	result, err := service.BindProject(cmd.Context(), profile.Provider, profile.Name, projectRoot)
	if err != nil {
		return fmt.Errorf("bind project to profile %q: %w", profile.Name, err)
	}
	return writeCodexSetupResult(cmd.ErrOrStderr(), "Profile ready and project bound", result.Project.Root, result.Profile)
}

func writeCodexSetupIntro(out io.Writer, projectRoot string) {
	_, _ = fmt.Fprintln(out, "Valv setup needed")
	_, _ = fmt.Fprintf(out, "  project: %s\n", projectRoot)
	_, _ = fmt.Fprintln(out, "  1. Use the default Codex home for this environment and bind it")
	_, _ = fmt.Fprintln(out, "  2. Choose an existing Valv profile")
	_, _ = fmt.Fprintln(out, "  3. Create a new isolated Valv profile")
	_, _ = fmt.Fprintln(out, "  4. Cancel")
}

func writeCodexSetupResult(out io.Writer, heading string, projectRoot string, profile domain.Profile) error {
	mode := output.ResolveMode(out, output.Policy{Format: domain.OutputFormatAuto, Style: domain.OutputStyleAuto})
	return output.WriteRecord(out, mode, heading, []output.Field{
		{Label: "project", Value: projectRoot, Identifier: true},
		{Label: "provider", Value: string(profile.Provider), Muted: true},
		{Label: "profile", Value: profile.Name, Identifier: true},
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
