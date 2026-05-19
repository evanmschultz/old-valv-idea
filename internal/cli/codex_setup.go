package cli

import (
	"errors"
	"fmt"

	"github.com/evanmschultz/laslig"
	"github.com/spf13/cobra"

	"github.com/evanmschultz/valv/internal/config"
	"github.com/evanmschultz/valv/internal/domain"
)

// ensureCodexAccountReadyForLaunch resolves the Codex profile for the current
// project launch and ensures host-side auth is ready. It returns the resolved
// domain.Profile so runCodexCommand can thread it into codexservice.Options.OverrideProfile.
//
// Resolution order:
//  1. accountOverride non-empty: resolve by name via ProfileByName. No binding
//     row is written. Proceeds to step 4.
//  2. accountOverride empty: check Codex binding via service.Status.
//     - Bound: store the bound profile and proceed to step 4.
//     - Unbound (ErrUnboundProject): enumerate ListProfiles(Codex):
//     -- 0 accounts: return unboundProjectNoAccountsError(codex).
//     -- 1 account: auto-bind silently, emit laslig notice, proceed to step 4.
//     -- 2+ accounts, TTY: launch picker, bind, proceed to step 4.
//     -- 2+ accounts, non-TTY: return error pointing to manual bind command.
//     - Any other error: wrap with "detect codex binding: %w" and return.
//  3. Skip step 4 (ensureManagedAccountReady) when codexArgsSkipAccountReady
//     returns true for args. The profile is still returned in this case.
//
// Codex auth is host-side. This function DOES call ensureManagedAccountReady
// (unlike ensureClaudeBindingReady, which skips it because Claude auth is
// in-container). This asymmetry is justified by runtime structure: Codex requires
// host-side login status checked before container launch; Claude does not.
func ensureCodexAccountReadyForLaunch(cmd *cobra.Command, paths config.Paths, workingDir string, accountOverride string, args []string) (domain.Profile, error) {
	service, closeStore, err := openManageService(cmd, paths)
	if err != nil {
		return domain.Profile{}, fmt.Errorf("initialize manage service: %w", err)
	}
	defer closeStore()

	var profile domain.Profile

	// Step 1: explicit override bypasses the binding store entirely.
	if accountOverride != "" {
		resolved, err := service.ProfileByName(cmd.Context(), domain.ProviderCodex, accountOverride)
		if err != nil {
			return domain.Profile{}, fmt.Errorf("resolve override account %q: %w", accountOverride, err)
		}
		profile = resolved
	} else {
		// Step 2: check whether the project already has a Codex binding.
		// service.Status hardcodes ProviderCodex internally — correct for Codex.
		status, err := service.Status(cmd.Context(), workingDir)
		if err == nil {
			// Already bound — use the existing profile.
			profile = status.Profile
		} else if !errors.Is(err, domain.ErrUnboundProject) {
			return domain.Profile{}, fmt.Errorf("detect codex binding: %w", err)
		} else {
			// Step 3: project is unbound — enumerate Codex accounts.
			listResult, err := service.ListProfiles(cmd.Context(), domain.ProviderCodex)
			if err != nil {
				return domain.Profile{}, fmt.Errorf("list codex accounts: %w", err)
			}
			profiles := listResult.Profiles

			switch len(profiles) {
			case 0:
				return domain.Profile{}, unboundProjectNoAccountsError(domain.ProviderCodex)

			case 1:
				// Auto-bind the single account silently.
				p := profiles[0]
				if _, err := service.BindProject(cmd.Context(), domain.ProviderCodex, p.Name, workingDir); err != nil {
					return domain.Profile{}, fmt.Errorf("auto-bind codex account %q: %w", p.Name, err)
				}
				if err := writeCLINotice(
					cmd.ErrOrStderr(),
					laslig.NoticeInfoLevel,
					"Project bound",
					fmt.Sprintf("Bound project to Codex account %q.", p.Name),
				); err != nil {
					// Non-fatal: notice write failure must not block launch.
					_ = err
				}
				profile = p

			default:
				// 2+ accounts: use picker when TTY available.
				if !commandHasTTY(cmd.InOrStdin()) || !commandHasTTY(cmd.OutOrStdout()) {
					return domain.Profile{}, fmt.Errorf(
						"project is not bound to a Codex account; run `valv manage bind codex <name>` to bind one",
					)
				}
				selected, err := pickProfile(cmd, domain.ProviderCodex, profiles)
				if err != nil {
					return domain.Profile{}, fmt.Errorf("select codex account: %w", err)
				}
				if _, err := service.BindProject(cmd.Context(), domain.ProviderCodex, selected.Name, workingDir); err != nil {
					return domain.Profile{}, fmt.Errorf("bind codex account %q: %w", selected.Name, err)
				}
				profile = selected
			}
		}
	}

	// Step 4: ensure host-side auth is ready. Skip when args indicate a
	// passthrough command (help, login, logout) that does not need a live session.
	if codexArgsSkipAccountReady(args) {
		return profile, nil
	}
	if err := ensureManagedAccountReady(cmd, profile.Provider, profile, accountAuthOptions{}); err != nil {
		return domain.Profile{}, fmt.Errorf("ensure account %q is ready: %w", profile.Name, err)
	}

	return profile, nil
}
