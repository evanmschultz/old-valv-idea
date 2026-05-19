package cli

import (
	"errors"
	"fmt"
	"strings"

	"github.com/evanmschultz/laslig"
	"github.com/spf13/cobra"

	"github.com/evanmschultz/valv/internal/config"
	"github.com/evanmschultz/valv/internal/domain"
)

// unboundProjectNoAccountsError returns the user-facing error for when a
// project is unbound and no accounts exist for the given provider.
func unboundProjectNoAccountsError(provider domain.Provider) error {
	return fmt.Errorf(
		"project is not bound; no %s accounts found — run `valv manage account add %s` to create one",
		provider, provider,
	)
}

// ensureClaudeBindingReady resolves the Claude profile that should be used for
// the current project launch. It returns the resolved domain.Profile so
// runClaudeCommand can thread it into claudeservice.Options.OverrideProfile.
//
// Resolution order:
//  1. accountOverride non-empty: resolve by name via ProfileByName. No binding
//     row is written. Returns resolved profile immediately.
//  2. accountOverride empty: check Claude binding via StatusForProvider.
//     - Bound: return the existing profile.
//     - Unbound (ErrUnboundProject): enumerate ListProfiles(Claude):
//     -- 0 accounts: return unboundProjectNoAccountsError(claude).
//     -- 1 account: auto-bind, emit laslig notice, return bound profile.
//     -- 2+ accounts, TTY: launch picker, bind, return bound profile.
//     -- 2+ accounts, non-TTY: return error pointing to manual bind command.
//     - Any other error: wrap with "detect claude binding: %w" and return.
//
// Claude auth is in-container. This function does NOT call ensureManagedAccountReady.
func ensureClaudeBindingReady(cmd *cobra.Command, paths config.Paths, workingDir string, accountOverride string) (domain.Profile, error) {
	service, closeStore, err := openManageService(cmd, paths)
	if err != nil {
		return domain.Profile{}, fmt.Errorf("initialize manage service: %w", err)
	}
	defer closeStore()

	// Step 1: explicit override bypasses the binding store entirely.
	if accountOverride != "" {
		profile, err := service.ProfileByName(cmd.Context(), domain.ProviderClaude, accountOverride)
		if err != nil {
			return domain.Profile{}, fmt.Errorf("resolve override account %q: %w", accountOverride, err)
		}
		return profile, nil
	}

	// Step 2: check whether the project already has a Claude binding.
	status, err := service.StatusForProvider(cmd.Context(), workingDir, domain.ProviderClaude)
	if err == nil {
		// Already bound — return the existing profile.
		return status.Profile, nil
	}
	// Only proceed to auto-bind / picker when the error is ErrUnboundProject.
	// Any other error (store failures, project-detect failures) must propagate.
	if !errors.Is(err, domain.ErrUnboundProject) && !strings.Contains(err.Error(), domain.ErrUnboundProject.Error()) {
		return domain.Profile{}, fmt.Errorf("detect claude binding: %w", err)
	}

	// Project is unbound — enumerate Claude accounts.
	listResult, err := service.ListProfiles(cmd.Context(), domain.ProviderClaude)
	if err != nil {
		return domain.Profile{}, fmt.Errorf("list claude accounts: %w", err)
	}
	profiles := listResult.Profiles

	switch len(profiles) {
	case 0:
		return domain.Profile{}, unboundProjectNoAccountsError(domain.ProviderClaude)

	case 1:
		// Auto-bind the single account silently.
		profile := profiles[0]
		if _, err := service.BindProject(cmd.Context(), domain.ProviderClaude, profile.Name, workingDir); err != nil {
			return domain.Profile{}, fmt.Errorf("auto-bind claude account %q: %w", profile.Name, err)
		}
		if err := writeCLINotice(
			cmd.ErrOrStderr(),
			laslig.NoticeInfoLevel,
			"Project bound",
			fmt.Sprintf("Bound project to Claude account %q.", profile.Name),
		); err != nil {
			// Non-fatal: notice write failure must not block launch.
			_ = err
		}
		return profile, nil

	default:
		// 2+ accounts: use picker when TTY available.
		if !commandHasTTY(cmd.InOrStdin()) || !commandHasTTY(cmd.OutOrStdout()) {
			return domain.Profile{}, fmt.Errorf(
				"project is not bound to a Claude account; run `valv manage bind claude <name>` to bind one",
			)
		}
		selected, err := pickProfile(cmd, domain.ProviderClaude, profiles)
		if err != nil {
			return domain.Profile{}, fmt.Errorf("select claude account: %w", err)
		}
		if _, err := service.BindProject(cmd.Context(), domain.ProviderClaude, selected.Name, workingDir); err != nil {
			return domain.Profile{}, fmt.Errorf("bind claude account %q: %w", selected.Name, err)
		}
		return selected, nil
	}
}
