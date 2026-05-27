package cli

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/charmbracelet/log"
	"github.com/spf13/cobra"

	dockeradapter "github.com/evanmschultz/valv/internal/adapters/docker"
	clauderuntime "github.com/evanmschultz/valv/internal/adapters/providers/claude"
	codexruntime "github.com/evanmschultz/valv/internal/adapters/providers/codex"
	"github.com/evanmschultz/valv/internal/config"
	"github.com/evanmschultz/valv/internal/domain"
	projectdetect "github.com/evanmschultz/valv/internal/project"
	runservice "github.com/evanmschultz/valv/internal/services/run"
)

// runRunFunc is the injectable RunE shape for `valv run`. The newRunCommand
// constructor wires the production implementation when run is nil; tests pass
// a stub function to assert pass-through args without exercising the full
// runtime stack.
type runRunFunc func(*cobra.Command, []string) error

// newRunCommand constructs the `valv run` cobra command. Like the provider
// runtime commands, it sets DisableFlagParsing so the prefix-only local-flag
// stripping in runRunCommand can extract --account/--provider before the
// target command is reached.
func newRunCommand(paths config.Paths, run runRunFunc) *cobra.Command {
	if run == nil {
		run = func(cmd *cobra.Command, args []string) error {
			return runRunCommand(cmd, paths, args)
		}
	}
	return &cobra.Command{
		Use:   "run --account <name> [--provider <provider>] <command> [args...]",
		Short: "Run a command in a per-account isolated container",
		Long: strings.TrimSpace(`
Launch any command inside a Valv-managed Docker runtime bound to the named account.

` + "`valv run`" + ` is the generic per-account launch primitive. ` + "`valv codex`" + ` and ` + "`valv claude`" + ` are first-class adapters over the same isolation model: per-account credential home, sibling-path-aware mounts, per-project overlay image, cross-provider in-container routing.

The ` + "`--account`" + ` and ` + "`--provider`" + ` flags are consumed only before the first non-flag positional. Any tokens after the first non-flag positional — including later occurrences of ` + "`--account`" + ` or ` + "`--provider`" + ` — are passed through to the target command verbatim. No ` + "`--`" + ` separator is required.

` + "`--provider`" + ` is optional. When omitted, the account name is resolved across all providers; a duplicate-name error is raised when the same account name exists in multiple providers and ` + "`--provider`" + ` is required to disambiguate.

` + "`valv run`" + ` is explicit-account only: it does not auto-bind projects, write binding rows, or mutate state. If the working directory has no persisted project record, ` + "`valv run`" + ` fails with bind guidance.
`),
		Example: strings.TrimSpace(`
valv run --account work bash
valv run --account work --provider codex sh -c "ls && pwd"
valv run --account work python -m http.server 8080
`),
		DisableFlagParsing: true,
		Args:               cobra.ArbitraryArgs,
		SilenceUsage:       true,
		RunE:               run,
	}
}

// runRunCommand is the production RunE for `valv run`. It performs prefix-only
// stripping of --account / --provider, resolves the account across providers,
// runs provider-specific auth readiness, resolves the project image, prepares
// the provider runtime, and invokes the shared run service with an explicit
// command override.
func runRunCommand(cmd *cobra.Command, paths config.Paths, args []string) error {
	if logger := LoggerFromContext(cmd.Context()); logger != nil {
		logger.Debug("run run command arguments", "args", args)
	}

	parsed, remaining := stripRunLocalFlags(args)

	// When no target command remains after leading-flag stripping, print own
	// help. This covers `valv run`, `valv run --help`, `valv run -h`,
	// `valv run help`, and `valv run --account X` with no positional.
	if len(remaining) == 0 || isRunHelpArg(remaining[0]) {
		return cmd.Help()
	}

	// Required: --account.
	if strings.TrimSpace(parsed.account) == "" {
		return fmt.Errorf("run requires --account <name>; run `valv run --help` for usage")
	}

	workingDir, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("run run command: resolve working directory: %w", err)
	}

	manage, closeStore, err := openManageService(cmd, paths)
	if err != nil {
		return fmt.Errorf("run run command: %w", err)
	}
	defer closeStore()

	provider, profile, err := resolveAccountByName(cmd.Context(), manage, parsed.account, parsed.provider)
	if err != nil {
		return fmt.Errorf("run run command: %w", err)
	}

	// Provider-specific auth readiness. ensureManagedAccountReady dispatches
	// to ensureCodexAccountReady (host-side login) and ensureClaudeAccountReady
	// (in-container login). accountAuthOptions carries paths so claude can
	// resolve override profile if needed; SkipLogin stays false here.
	if err := ensureManagedAccountReady(cmd, provider, profile, accountAuthOptions{Paths: paths}); err != nil {
		return fmt.Errorf("run run command: %w", err)
	}

	// Detect project to compute the project root. The project record itself
	// must already exist; valv run does not auto-create. We open the store
	// directly here (rather than going through manageservice.Service) because
	// the manage Service does not expose project/binding/profile repository
	// methods; the launchers in claude/codex services already access the
	// store directly for this purpose.
	detected, err := projectdetect.DetectFrom(workingDir)
	if err != nil {
		return fmt.Errorf("run run command: detect project from %q: %w", workingDir, err)
	}

	store, err := openStore(paths)
	if err != nil {
		return fmt.Errorf("run run command: %w", err)
	}
	defer store.Close()

	projectRecord, err := store.ProjectByRoot(cmd.Context(), detected.Root)
	if err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			return fmt.Errorf("run run command: %w", unboundProjectBindHintError(parsed, provider, profile.Name))
		}
		return fmt.Errorf("run run command: lookup project %q: %w", detected.Root, err)
	}

	// Cross-provider profile lookup. When the OTHER provider has a binding
	// for this project, pass its profile home to PrepareRuntime so the cross-
	// mount is set up. ErrNotFound = skip silently.
	other := otherProvider(provider)
	var otherProfileHome string
	otherBinding, err := store.BindingByProjectID(cmd.Context(), projectRecord.ID, other)
	if err == nil {
		otherProfile, profileErr := store.ProfileByID(cmd.Context(), otherBinding.ProfileID)
		if profileErr == nil {
			otherProfileHome = otherProfile.HomePath
		}
	} else if !errors.Is(err, domain.ErrNotFound) {
		return fmt.Errorf("run run command: lookup %s binding for project %q: %w", other, projectRecord.Root, err)
	}

	// Image resolution: base ref → resolveProjectImage (overlay + override).
	baseRef, err := baseImageRefForProvider(provider)
	if err != nil {
		return fmt.Errorf("run run command: %w", err)
	}

	// Image-current check (parity with claude/codex launchers). Kept BEFORE
	// resolveProjectImage so the per-project ref flows into LaunchRequest.
	if err := ensureProviderImageCurrent(cmd, paths, provider); err != nil {
		return fmt.Errorf("run run command: %w", err)
	}

	projectImage, err := resolveProjectImage(cmd, paths, provider, workingDir, baseRef)
	if err != nil {
		return fmt.Errorf("run run command: %w", err)
	}

	logger := LoggerFromContext(cmd.Context())
	stdinTTY := commandHasTTY(cmd.InOrStdin())
	stdoutTTY := commandHasTTY(cmd.OutOrStdout())

	// Provider-specific runtime prep. Each provider returns its own
	// PreparedRuntime shape — we adapt to the shared run.PreparedRuntime
	// minimum contract (Env, EnvPassthrough, Mounts, Warnings, Cleanup).
	prepared, providerDescriptor, err := preparePerProviderRuntime(
		cmd.Context(), provider, profile, projectRecord.Root, paths, otherProfileHome, logger,
	)
	if err != nil {
		return fmt.Errorf("run run command: prepare runtime: %w", err)
	}

	service, err := runservice.New(runservice.Options{
		Executor: dockeradapter.NewExecutor(dockeradapter.NewSystemRunner("docker", cmd.InOrStdin(), cmd.OutOrStdout(), cmd.ErrOrStderr())),
		Image:    projectImage,
		User:     currentContainerUser(),
		TTY:      stdinTTY && stdoutTTY,
		Stdin:    stdinTTY,
		Logger:   logger,
		Notices:  cmd.ErrOrStderr(),
		Provider: providerDescriptor,
	})
	if err != nil {
		// Synchronously close the prepared runtime so cleanup isn't lost when
		// service construction fails before run.Service.Run takes ownership.
		if prepared != nil && prepared.Cleanup != nil {
			_ = prepared.Cleanup()
		}
		return fmt.Errorf("run run command: initialize launcher: %w", err)
	}

	// Fetch account env entries and convert to map for LaunchRequest.
	envEntries, err := store.ListAccountEnv(cmd.Context(), profile.ID)
	if err != nil {
		return fmt.Errorf("run run command: load account env for profile %q: %w", profile.ID, err)
	}
	accountEnv := domain.AccountEnvEntriesToMap(envEntries)

	launch := runservice.LaunchRequest{
		ProjectRoot: projectRecord.Root,
		WorkingDir:  workingDir,
		ProjectID:   projectRecord.ID,
		ProfileID:   profile.ID,
		ProjectName: projectRecord.Name,
		Prepared:    prepared,
		Command:     append([]string(nil), remaining...),
		AccountEnv:  accountEnv,
	}

	if err := service.Run(cmd.Context(), launch); err != nil {
		return fmt.Errorf("run run command: %w", err)
	}
	return nil
}

// parsedRunFlags is the result of prefix-only local flag stripping.
type parsedRunFlags struct {
	// account is the value of --account / --account=, empty if absent.
	account string
	// provider is the value of --provider / --provider=, empty if absent.
	provider string
	// accountExplicit and providerExplicit record whether the flag appeared
	// in the prefix. Currently only providerExplicit is consulted by the
	// bind-hint formatter; accountExplicit is reserved for future use.
	accountExplicit  bool
	providerExplicit bool
}

// stripRunLocalFlags consumes --account / --provider flags ONLY from the
// leading prefix of args. The scan stops at the first non-flag positional
// token; all subsequent args — including later --account / --provider — are
// returned as the remaining slice unchanged.
//
// Behavior contrasts with stripAccountFlag (which scans until `--` for the
// provider launchers' broader pass-through semantics). For `valv run`, where
// every positional may itself accept its own --account / --provider, prefix-
// only stripping is the only correct mode — see drop PLAN.md Schema Decision
// "valv run local-flag extraction is prefix-only".
func stripRunLocalFlags(args []string) (parsedRunFlags, []string) {
	parsed := parsedRunFlags{}
	i := 0
	for i < len(args) {
		arg := args[i]
		switch {
		case arg == "--":
			// End-of-options sentinel: stop stripping; the sentinel itself is
			// preserved in remaining so the target command sees it.
			return parsed, append([]string(nil), args[i:]...)
		case arg == "--account":
			if i+1 >= len(args) {
				// Malformed: --account with no following value. Stop stripping;
				// preserve the malformed flag in remaining so a downstream error
				// path can complain.
				return parsed, append([]string(nil), args[i:]...)
			}
			parsed.account = args[i+1]
			parsed.accountExplicit = true
			i += 2
		case strings.HasPrefix(arg, "--account="):
			value := arg[len("--account="):]
			if value == "" {
				// Malformed: --account= with empty value. Stop stripping.
				return parsed, append([]string(nil), args[i:]...)
			}
			parsed.account = value
			parsed.accountExplicit = true
			i++
		case arg == "--provider":
			if i+1 >= len(args) {
				return parsed, append([]string(nil), args[i:]...)
			}
			parsed.provider = args[i+1]
			parsed.providerExplicit = true
			i += 2
		case strings.HasPrefix(arg, "--provider="):
			value := arg[len("--provider="):]
			if value == "" {
				return parsed, append([]string(nil), args[i:]...)
			}
			parsed.provider = value
			parsed.providerExplicit = true
			i++
		default:
			// First non-flag positional reached: everything from here is the
			// target command and is passed through unchanged.
			return parsed, append([]string(nil), args[i:]...)
		}
	}
	return parsed, nil
}

// isRunHelpArg reports whether arg requests help for `valv run` itself when it
// appears as the first non-flag-stripped token. `valv run --help` (no target
// command) prints `valv run`'s help; `valv run cmd --help` passes `--help`
// through to the target command.
func isRunHelpArg(arg string) bool {
	switch arg {
	case "--help", "-h", "help":
		return true
	default:
		return false
	}
}

// unboundProjectBindHintError returns the user-facing error for the case
// where the project has no persisted record. The bind hint preserves
// runtime provider context: when --provider was explicit, the hint uses
// `valv account bind <name> --provider <provider>`; when the provider was
// uniquely inferred via resolveAccountByName, the shorter form is used.
func unboundProjectBindHintError(parsed parsedRunFlags, provider domain.Provider, accountName string) error {
	if parsed.providerExplicit {
		return fmt.Errorf(
			"project is not bound; run `valv account bind %s --provider %s` to create the project record",
			accountName, provider,
		)
	}
	return fmt.Errorf(
		"project is not bound; run `valv account bind %s` to create the project record",
		accountName,
	)
}

// otherProvider returns the cross-provider identifier for the given provider.
// Codex ↔ Claude. Returns empty string for an unsupported provider.
func otherProvider(provider domain.Provider) domain.Provider {
	switch provider {
	case domain.ProviderClaude:
		return domain.ProviderCodex
	case domain.ProviderCodex:
		return domain.ProviderClaude
	default:
		return ""
	}
}

// baseImageRefForProvider returns the base image ref for the provider,
// honoring the VALV_<PROVIDER>_IMAGE override. resolveProjectImage then
// either returns this ref unchanged (empty manifest OR override active) or
// builds an overlay derived from it.
func baseImageRefForProvider(provider domain.Provider) (dockeradapter.ImageRef, error) {
	switch provider {
	case domain.ProviderClaude:
		return claudeImageRef(), nil
	case domain.ProviderCodex:
		return codexImageRef(), nil
	default:
		return dockeradapter.ImageRef{}, fmt.Errorf("unsupported provider %q", provider)
	}
}

// ensureProviderImageCurrent dispatches to the provider-specific image-current
// helper used by the existing launchers, preserving DROP_12 Unit 12.4
// ordering: image-current MUST run before resolveProjectImage so that the
// per-project ref derives from the post-update base.
func ensureProviderImageCurrent(cmd *cobra.Command, paths config.Paths, provider domain.Provider) error {
	switch provider {
	case domain.ProviderClaude:
		return ensureClaudeImageCurrent(cmd, paths)
	case domain.ProviderCodex:
		return ensureCodexImageCurrent(cmd, paths)
	default:
		return fmt.Errorf("unsupported provider %q", provider)
	}
}

// preparePerProviderRuntime invokes the provider-specific PrepareRuntime path
// and adapts the result to runservice.PreparedRuntime's minimum contract. It
// also returns the runservice.Provider descriptor (display name + container
// name prefix + notice prefix) for the resolved provider.
func preparePerProviderRuntime(
	ctx context.Context,
	provider domain.Provider,
	profile domain.Profile,
	projectRoot string,
	paths config.Paths,
	otherProfileHome string,
	logger *log.Logger,
) (*runservice.PreparedRuntime, runservice.Provider, error) {
	switch provider {
	case domain.ProviderClaude:
		prepared, err := clauderuntime.PrepareRuntime(ctx, clauderuntime.PrepareRequest{
			ProfileHome:              profile.HomePath,
			SharedHome:               "",
			ProjectRoot:              projectRoot,
			TempRoot:                 paths.TempCacheDir,
			OtherProviderProfileHome: otherProfileHome,
			Logger:                   logger,
		})
		if err != nil {
			return nil, runservice.Provider{}, err
		}
		return adaptClaudePreparedRuntime(prepared), runservice.Provider{
			Name:                "claude",
			ContainerNamePrefix: "valv-claude-interactive",
			NoticePrefix:        "Valv note",
		}, nil
	case domain.ProviderCodex:
		// SharedHome empty here: valv run does not currently derive the
		// Codex shared host home (codexservice.sharedCodexStateHome). That
		// behavior is provider-wrapper-specific and is preserved in the
		// claude/codex CLI commands themselves. For valv run, the explicit
		// override semantics treat the profile home as both profile and
		// shared, matching the isolated-account model.
		prepared, err := codexruntime.PrepareRuntime(ctx, codexruntime.PrepareRequest{
			ProfileHome:              profile.HomePath,
			SharedHome:               "",
			ProjectRoot:              projectRoot,
			TempRoot:                 paths.TempCacheDir,
			OtherProviderProfileHome: otherProfileHome,
			Logger:                   logger,
		})
		if err != nil {
			return nil, runservice.Provider{}, err
		}
		return adaptCodexPreparedRuntime(prepared), runservice.Provider{
			Name:                "codex",
			ContainerNamePrefix: "valv-codex-interactive",
			NoticePrefix:        "Valv MCP note",
		}, nil
	default:
		return nil, runservice.Provider{}, fmt.Errorf("unsupported provider %q", provider)
	}
}

// adaptClaudePreparedRuntime constructs a runservice.PreparedRuntime from the
// claude provider's PreparedRuntime. The Cleanup closure delegates to the
// provider's Close, preserving sync-back and temp-dir removal.
func adaptClaudePreparedRuntime(p clauderuntime.PreparedRuntime) *runservice.PreparedRuntime {
	return &runservice.PreparedRuntime{
		Env:            p.Env,
		EnvPassthrough: p.EnvPassthrough,
		Mounts:         p.Mounts,
		Warnings:       p.Warnings,
		Cleanup:        func() error { return p.Close() },
	}
}

// adaptCodexPreparedRuntime mirrors adaptClaudePreparedRuntime for codex.
func adaptCodexPreparedRuntime(p codexruntime.PreparedRuntime) *runservice.PreparedRuntime {
	return &runservice.PreparedRuntime{
		Env:            p.Env,
		EnvPassthrough: p.EnvPassthrough,
		Mounts:         p.Mounts,
		Warnings:       p.Warnings,
		Cleanup:        func() error { return p.Close() },
	}
}
