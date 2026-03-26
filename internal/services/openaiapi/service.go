package openaiapi

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/charmbracelet/log"

	dockeradapter "github.com/evanmschultz/valv/internal/adapters/docker"
	codexruntime "github.com/evanmschultz/valv/internal/adapters/providers/codex"
	openaiapi "github.com/evanmschultz/valv/internal/api/openai"
	"github.com/evanmschultz/valv/internal/domain"
	"github.com/evanmschultz/valv/internal/pathutil"
	projectdetect "github.com/evanmschultz/valv/internal/project"
)

type Store interface {
	domain.ProjectRepository
	domain.BindingRepository
	domain.ProfileRepository
	domain.RuntimeRepository
}

type Executor interface {
	Run(context.Context, dockeradapter.ContainerRunRequest) error
	Exec(context.Context, dockeradapter.ContainerExecRequest) error
	Inspect(context.Context, string) error
	RemoveContainer(context.Context, dockeradapter.ContainerRemoveRequest) error
}

type DetectFunc func(string) (projectdetect.Result, error)

const DefaultIdleTTL = 2 * time.Minute

type Options struct {
	Store           Store
	Executor        Executor
	Detect          DetectFunc
	Image           dockeradapter.ImageRef
	User            string
	TempRoot        string
	StartPath       string
	WorkspaceAccess bool
	IdleTTL         time.Duration
	Now             func() time.Time
	Logger          *log.Logger
}

type Service struct {
	store           Store
	executor        Executor
	detect          DetectFunc
	image           dockeradapter.ImageRef
	user            string
	tempRoot        string
	startPath       string
	workspaceAccess bool
	idleTTL         time.Duration
	now             func() time.Time
	logger          *log.Logger
	artifacts       *runtimeArtifacts
}

type runtimeArtifacts struct {
	mu          sync.Mutex
	byContainer map[string]codexruntime.PreparedRuntime
}

func New(options Options) (Service, error) {
	if options.Store == nil {
		return Service{}, fmt.Errorf("new openai api service: store is required")
	}
	if options.Executor == nil {
		return Service{}, fmt.Errorf("new openai api service: executor is required")
	}
	if strings.TrimSpace(options.Image.Repository) == "" {
		return Service{}, fmt.Errorf("new openai api service: image repository is required")
	}
	if strings.TrimSpace(options.TempRoot) == "" {
		return Service{}, fmt.Errorf("new openai api service: temp root is required")
	}
	idleTTL := options.IdleTTL
	if idleTTL <= 0 {
		idleTTL = DefaultIdleTTL
	}
	now := options.Now
	if now == nil {
		now = func() time.Time { return time.Now().UTC() }
	}
	detect := options.Detect
	if detect == nil {
		detect = projectdetect.DetectFrom
	}
	return Service{
		store:           options.Store,
		executor:        options.Executor,
		detect:          detect,
		image:           options.Image,
		user:            strings.TrimSpace(options.User),
		tempRoot:        strings.TrimSpace(options.TempRoot),
		startPath:       strings.TrimSpace(options.StartPath),
		workspaceAccess: options.WorkspaceAccess,
		idleTTL:         idleTTL,
		now:             now,
		logger:          options.Logger,
		artifacts:       &runtimeArtifacts{byContainer: map[string]codexruntime.PreparedRuntime{}},
	}, nil
}

func (s Service) ValidateBinding(ctx context.Context) error {
	_, err := s.resolveBinding(ctx)
	return err
}

func (s Service) PruneExpiredRuntimes(ctx context.Context) (int, error) {
	resolved, err := s.resolveBinding(ctx)
	if err != nil {
		return 0, err
	}
	records, err := s.store.ListRuntimesByProjectID(ctx, resolved.project.ID)
	if err != nil {
		return 0, fmt.Errorf("prune expired runtimes for project %q: list runtimes: %w", resolved.project.Root, err)
	}
	now := s.now()
	removed := 0
	for _, record := range records {
		if !s.matchesRuntime(record, resolved.profile) {
			continue
		}
		if !s.runtimeExpired(record, now) {
			continue
		}
		if err := s.stopRuntime(ctx, record, "expired"); err != nil {
			return removed, fmt.Errorf("prune expired runtimes for project %q: stop runtime %q: %w", resolved.project.Root, record.ID, err)
		}
		removed++
	}
	return removed, nil
}

func (s Service) Complete(ctx context.Context, request openaiapi.Request) (openaiapi.Result, error) {
	resolved, err := s.resolveBinding(ctx)
	if err != nil {
		return openaiapi.Result{}, err
	}
	runtimeRecord, err := s.leaseRuntime(ctx, resolved)
	if err != nil {
		return openaiapi.Result{}, err
	}
	prepared, ok := s.lookupArtifacts(runtimeRecord.ContainerID)
	if !ok {
		return openaiapi.Result{}, fmt.Errorf("complete chat request: runtime %q is missing prepared artifacts", runtimeRecord.ContainerID)
	}
	tempDir, err := os.MkdirTemp(s.tempRoot, "openai-exec-")
	if err != nil {
		return openaiapi.Result{}, fmt.Errorf("complete chat request: create temp dir: %w", err)
	}
	defer os.RemoveAll(tempDir)

	resultPath := filepath.Join(tempDir, "last-message.txt")
	req, err := s.buildExecRequest(request, runtimeRecord, resolved.project, resolved.profile, tempDir, resultPath, prepared)
	if err != nil {
		return openaiapi.Result{}, fmt.Errorf("complete chat request: build docker exec request: %w", err)
	}
	s.debug("executing headless codex request", "project", resolved.project.Root, "profile", resolved.profile.Name, "runtime", runtimeRecord.ContainerID, "workspace_access", s.workspaceAccess)
	if err := s.executor.Exec(ctx, req); err != nil {
		return openaiapi.Result{}, fmt.Errorf("complete chat request: execute runtime %q: %w", runtimeRecord.ContainerID, err)
	}
	runtimeRecord.Status = "running"
	runtimeRecord.UpdatedAt = s.now()
	if _, err := s.store.UpsertRuntime(ctx, runtimeRecord); err != nil {
		return openaiapi.Result{}, fmt.Errorf("complete chat request: update runtime %q: %w", runtimeRecord.ID, err)
	}
	content, err := os.ReadFile(resultPath)
	if err != nil {
		return openaiapi.Result{}, fmt.Errorf("complete chat request: read output message: %w", err)
	}
	return openaiapi.Result{
		Model:        request.Model,
		Content:      strings.TrimRight(string(content), "\n"),
		FinishReason: "stop",
		CreatedAt:    time.Now().UTC(),
	}, nil
}

type resolvedBinding struct {
	project domain.Project
	profile domain.Profile
}

func (s Service) resolveBinding(ctx context.Context) (resolvedBinding, error) {
	cwd := s.startPath
	if strings.TrimSpace(cwd) == "" {
		var err error
		cwd, err = os.Getwd()
		if err != nil {
			return resolvedBinding{}, fmt.Errorf("resolve binding: determine working directory: %w", err)
		}
	}
	workingDir, err := pathutil.Normalize(cwd)
	if err != nil {
		return resolvedBinding{}, fmt.Errorf("resolve binding: normalize working directory: %w", err)
	}
	projectResult, err := s.detect(workingDir)
	if err != nil {
		return resolvedBinding{}, fmt.Errorf("resolve binding: detect project from %q: %w", workingDir, err)
	}
	projectRecord, err := s.store.ProjectByRoot(ctx, projectResult.Root)
	if err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			return resolvedBinding{}, fmt.Errorf("resolve binding: project %q: %w", projectResult.Root, domain.ErrUnboundProject)
		}
		return resolvedBinding{}, fmt.Errorf("resolve binding: lookup project %q: %w", projectResult.Root, err)
	}
	binding, err := s.store.BindingByProjectID(ctx, projectRecord.ID)
	if err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			return resolvedBinding{}, fmt.Errorf("resolve binding: project %q: %w", projectRecord.Root, domain.ErrUnboundProject)
		}
		return resolvedBinding{}, fmt.Errorf("resolve binding: lookup binding for project %q: %w", projectRecord.Root, err)
	}
	profile, err := s.store.ProfileByID(ctx, binding.ProfileID)
	if err != nil {
		return resolvedBinding{}, fmt.Errorf("resolve binding: lookup profile %q: %w", binding.ProfileID, err)
	}
	return resolvedBinding{project: projectRecord, profile: profile}, nil
}

func (s Service) buildRuntimeRequest(runtimeRecord domain.RuntimeRecord, project domain.Project, profile domain.Profile, prepared codexruntime.PreparedRuntime) (dockeradapter.ContainerRunRequest, error) {
	mounts := append([]dockeradapter.MountSpec{}, prepared.Mounts...)
	if !s.workspaceAccess {
		mounts = filterProjectScopedMounts(mounts, project.Root)
	}
	mounts = append(mounts, dockeradapter.NewMountSpec(s.tempRoot, s.tempRoot, false))
	workingDir := "/tmp"
	if s.workspaceAccess {
		mounts = append(mounts, dockeradapter.NewMountSpec(project.Root, project.Root, false))
		workingDir = project.Root
	}
	return dockeradapter.ContainerRunRequest{
		Name:       runtimeRecord.ContainerID,
		Image:      s.image,
		WorkingDir: workingDir,
		Env:        prepared.Env,
		Labels: map[string]string{
			"io.valv.managed":          "true",
			"io.valv.provider":         "codex",
			"io.valv.scope":            "api",
			"io.valv.project_id":       project.ID,
			"io.valv.profile_id":       profile.ID,
			"io.valv.workspace_access": fmt.Sprintf("%t", s.workspaceAccess),
		},
		Mounts:   mounts,
		Args:     []string{"-lc", "while :; do sleep 60; done"},
		Detached: true,
		User:     s.user,
		Extra:    []string{"--entrypoint", "/bin/sh"},
	}, nil
}

func filterProjectScopedMounts(mounts []dockeradapter.MountSpec, projectRoot string) []dockeradapter.MountSpec {
	if strings.TrimSpace(projectRoot) == "" || len(mounts) == 0 {
		return mounts
	}
	filtered := mounts[:0]
	for _, mount := range mounts {
		if mount.Target == projectRoot || strings.HasPrefix(mount.Target, projectRoot+string(filepath.Separator)) {
			continue
		}
		filtered = append(filtered, mount)
	}
	return filtered
}

func (s Service) buildExecRequest(request openaiapi.Request, runtimeRecord domain.RuntimeRecord, project domain.Project, profile domain.Profile, tempDir string, resultPath string, prepared codexruntime.PreparedRuntime) (dockeradapter.ContainerExecRequest, error) {
	prompt := renderPrompt(request)
	workingDir := "/tmp"
	args := []string{"codex", "exec", "--json", "--output-last-message", resultPath, "--skip-git-repo-check"}
	if s.workspaceAccess {
		workingDir = project.Root
		args = append(args, "--cd", project.Root)
	}
	if strings.TrimSpace(request.Model) != "" {
		args = append(args, "--model", request.Model)
	}
	args = append(args, prompt)
	return dockeradapter.ContainerExecRequest{
		ContainerID: runtimeRecord.ContainerID,
		WorkingDir:  workingDir,
		Env:         prepared.Env,
		Args:        args,
		User:        s.user,
	}, nil
}

func (s Service) leaseRuntime(ctx context.Context, resolved resolvedBinding) (domain.RuntimeRecord, error) {
	records, err := s.store.ListRuntimesByProjectID(ctx, resolved.project.ID)
	if err != nil {
		return domain.RuntimeRecord{}, fmt.Errorf("lease runtime for project %q: list runtimes: %w", resolved.project.Root, err)
	}
	now := s.now()
	for _, record := range records {
		if !s.matchesRuntime(record, resolved.profile) {
			continue
		}
		if s.runtimeExpired(record, now) {
			if err := s.stopRuntime(ctx, record, "expired"); err != nil {
				return domain.RuntimeRecord{}, fmt.Errorf("lease runtime for project %q: expire runtime %q: %w", resolved.project.Root, record.ID, err)
			}
			continue
		}
		if err := s.executor.Inspect(ctx, record.ContainerID); err != nil {
			record.Status = "missing"
			record.UpdatedAt = now
			if _, upsertErr := s.store.UpsertRuntime(ctx, record); upsertErr != nil {
				return domain.RuntimeRecord{}, fmt.Errorf("lease runtime for project %q: update missing runtime %q: %w", resolved.project.Root, record.ID, upsertErr)
			}
			s.debug("warm runtime missing, starting replacement", "runtime", record.ID, "container", record.ContainerID)
			continue
		}
		record.Status = "running"
		record.UpdatedAt = now
		record, err = s.store.UpsertRuntime(ctx, record)
		if err != nil {
			return domain.RuntimeRecord{}, fmt.Errorf("lease runtime for project %q: touch runtime %q: %w", resolved.project.Root, record.ID, err)
		}
		if _, ok := s.lookupArtifacts(record.ContainerID); !ok {
			if err := s.stopRuntime(ctx, record, "stale"); err != nil {
				return domain.RuntimeRecord{}, fmt.Errorf("lease runtime for project %q: stop stale runtime %q: %w", resolved.project.Root, record.ID, err)
			}
			continue
		}
		s.debug("reusing warm runtime", "runtime", record.ID, "container", record.ContainerID)
		return record, nil
	}
	return s.startRuntime(ctx, resolved, now)
}

func (s Service) startRuntime(ctx context.Context, resolved resolvedBinding, now time.Time) (domain.RuntimeRecord, error) {
	containerID := s.runtimeContainerID()
	record, err := domain.NewRuntimeRecord(domain.ProviderCodex, resolved.project.ID, resolved.profile.ID, domain.ModeFresh, containerID, s.image.String(), "starting")
	if err != nil {
		return domain.RuntimeRecord{}, fmt.Errorf("start runtime for project %q: %w", resolved.project.Root, err)
	}
	record.CreatedAt = now
	record.UpdatedAt = now
	record, err = s.store.UpsertRuntime(ctx, record)
	if err != nil {
		return domain.RuntimeRecord{}, fmt.Errorf("start runtime for project %q: persist runtime: %w", resolved.project.Root, err)
	}
	prepared, err := codexruntime.PrepareRuntime(ctx, codexruntime.PrepareRequest{
		ProfileHome: resolved.profile.HomePath,
		ProjectRoot: resolved.project.Root,
		TempRoot:    s.tempRoot,
		Logger:      s.logger,
	})
	if err != nil {
		return domain.RuntimeRecord{}, fmt.Errorf("start runtime for project %q: prepare runtime: %w", resolved.project.Root, err)
	}
	request, err := s.buildRuntimeRequest(record, resolved.project, resolved.profile, prepared)
	if err != nil {
		_ = prepared.Close()
		return domain.RuntimeRecord{}, fmt.Errorf("start runtime for project %q: build docker run request: %w", resolved.project.Root, err)
	}
	if err := s.executor.Run(ctx, request); err != nil {
		_ = prepared.Close()
		return domain.RuntimeRecord{}, fmt.Errorf("start runtime for project %q: launch runtime container: %w", resolved.project.Root, err)
	}
	s.storeArtifacts(record.ContainerID, prepared)
	record.Status = "running"
	record.UpdatedAt = s.now()
	record, err = s.store.UpsertRuntime(ctx, record)
	if err != nil {
		return domain.RuntimeRecord{}, fmt.Errorf("start runtime for project %q: update runtime status: %w", resolved.project.Root, err)
	}
	s.debug("started warm runtime", "runtime", record.ID, "container", record.ContainerID, "workspace_access", s.workspaceAccess)
	return record, nil
}

func (s Service) stopRuntime(ctx context.Context, record domain.RuntimeRecord, status string) error {
	if err := s.executor.RemoveContainer(ctx, dockeradapter.ContainerRemoveRequest{IDs: []string{record.ContainerID}, Force: true}); err != nil {
		return err
	}
	if prepared, ok := s.takeArtifacts(record.ContainerID); ok {
		if err := prepared.Close(); err != nil {
			return err
		}
	}
	record.Status = status
	record.UpdatedAt = s.now()
	_, err := s.store.UpsertRuntime(ctx, record)
	return err
}

func (s Service) matchesRuntime(record domain.RuntimeRecord, profile domain.Profile) bool {
	if record.Provider != domain.ProviderCodex {
		return false
	}
	if record.ProfileID != profile.ID {
		return false
	}
	if record.ImageRef != s.image.String() {
		return false
	}
	if record.Status != "running" {
		return false
	}
	return strings.HasPrefix(record.ContainerID, s.runtimeContainerPrefix())
}

func (s Service) runtimeExpired(record domain.RuntimeRecord, now time.Time) bool {
	if s.idleTTL <= 0 {
		return false
	}
	return now.Sub(record.UpdatedAt) > s.idleTTL
}

func (s Service) runtimeContainerPrefix() string {
	if s.workspaceAccess {
		return "valv-api-codex-ws-"
	}
	return "valv-api-codex-nowork-"
}

func (s Service) runtimeContainerID() string {
	return s.runtimeContainerPrefix() + fmt.Sprintf("%d", s.now().UnixNano())
}

func renderPrompt(request openaiapi.Request) string {
	var parts []string
	for _, message := range request.Messages {
		role := strings.ToUpper(strings.TrimSpace(string(message.Role)))
		name := strings.TrimSpace(message.Name)
		if name != "" {
			role += " (" + name + ")"
		}
		parts = append(parts, role+":\n"+strings.TrimSpace(message.Content))
	}
	return strings.Join(parts, "\n\n")
}

func (s Service) debug(msg string, keyvals ...any) {
	if s.logger == nil {
		return
	}
	s.logger.Debug(msg, keyvals...)
}

func (s Service) storeArtifacts(containerID string, prepared codexruntime.PreparedRuntime) {
	if s.artifacts == nil {
		return
	}
	s.artifacts.mu.Lock()
	defer s.artifacts.mu.Unlock()
	s.artifacts.byContainer[containerID] = prepared
}

func (s Service) lookupArtifacts(containerID string) (codexruntime.PreparedRuntime, bool) {
	if s.artifacts == nil {
		return codexruntime.PreparedRuntime{}, false
	}
	s.artifacts.mu.Lock()
	defer s.artifacts.mu.Unlock()
	prepared, ok := s.artifacts.byContainer[containerID]
	return prepared, ok
}

func (s Service) takeArtifacts(containerID string) (codexruntime.PreparedRuntime, bool) {
	if s.artifacts == nil {
		return codexruntime.PreparedRuntime{}, false
	}
	s.artifacts.mu.Lock()
	defer s.artifacts.mu.Unlock()
	prepared, ok := s.artifacts.byContainer[containerID]
	if ok {
		delete(s.artifacts.byContainer, containerID)
	}
	return prepared, ok
}
