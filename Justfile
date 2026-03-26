set shell := ["bash", "-eu", "-o", "pipefail", "-c"]
dev_home_file := ".tmp/dev-home.path"
dev_image_repo := "valv-codex-dev"
dev_image_tag := "dev"
dev_image := dev_image_repo + ":" + dev_image_tag

[private]
verify-bootstrap:
  @test -f AGENTS.md
  @test -f PLAN.md
  @test -f go.mod
  @test -f .github/workflows/ci.yml

fmt:
  @if ! find . -type f -name '*.go' -not -path './.git/*' -not -path './.tmp/*' -not -path './.worklog/*' -print -quit | grep -q .; then \
    echo "skip fmt: no Go files found"; \
  else \
    find . -type f -name '*.go' -not -path './.git/*' -not -path './.tmp/*' -not -path './.worklog/*' -print0 | xargs -0 gofmt -w; \
  fi

build:
  @GOFLAGS="${GOFLAGS:+$GOFLAGS }-buildvcs=false" go build -o ./valv ./cmd/valv

[private]
ensure-dev-home:
  @mkdir -p .tmp
  @if [ ! -f {{dev_home_file}} ]; then \
    mktemp -d "${TMPDIR:-/tmp}/valv-dev.XXXXXX" > {{dev_home_file}}; \
  fi; \
  dev_home="$(cat {{dev_home_file}})"; \
  mkdir -p "$dev_home"

dev-home: ensure-dev-home
  @dev_home="$(cat {{dev_home_file}})"; \
  printf "Disposable dev home: %s\n" "$dev_home"; \
  printf "This temp path is only used by 'just dev ...' commands and can be removed with 'just dev-clean'.\n"

dev-reset:
  @mkdir -p .tmp
  @if [ -f {{dev_home_file}} ]; then \
    old_home="$(cat {{dev_home_file}})"; \
    rm -rf "$old_home"; \
    rm -f {{dev_home_file}}; \
  fi
  @mktemp -d "${TMPDIR:-/tmp}/valv-dev.XXXXXX" > {{dev_home_file}}
  @dev_home="$(cat {{dev_home_file}})"; \
  printf "Disposable dev home reset: %s\n" "$dev_home"; \
  printf "This temp path is only used by 'just dev ...' commands and can be removed with 'just dev-clean'.\n"

dev-clean:
  @if [ -f {{dev_home_file}} ]; then \
    dev_home="$(cat {{dev_home_file}})"; \
    rm -rf "$dev_home"; \
    rm -f {{dev_home_file}}; \
  fi
  @images="$(docker image ls {{dev_image_repo}} --format '{{"{{.Repository}}:{{.Tag}}"}}')"; \
  if [ -n "$images" ]; then \
    docker image rm --force $images >/dev/null; \
  fi

dev *ARGS: build ensure-dev-home
  @dev_home="$(cat {{dev_home_file}})"; \
  host_home="$(cd ~ && pwd)"; \
  HOME="$dev_home" DOCKER_CONFIG="$host_home/.docker" VALV_REAL_HOME="$host_home" VALV_CODEX_IMAGE="{{dev_image}}" ./valv {{ARGS}}

[private]
fmt-check:
  @if ! find . -type f -name '*.go' -not -path './.git/*' -not -path './.tmp/*' -not -path './.worklog/*' -print -quit | grep -q .; then \
    echo "skip fmt-check: no Go files found"; \
  else \
    out="$(find . -type f -name '*.go' -not -path './.git/*' -not -path './.tmp/*' -not -path './.worklog/*' -print0 | xargs -0 gofmt -l)"; \
    if [ -n "$out" ]; then \
      echo "gofmt required for:"; \
      echo "$out"; \
      exit 1; \
    fi; \
  fi

test:
  @GOFLAGS="${GOFLAGS:+$GOFLAGS }-buildvcs=false" go test -count=1 ./...

integration:
  @GOFLAGS="${GOFLAGS:+$GOFLAGS }-buildvcs=false" go test -tags=integration -count=1 -run '^TestCodexCommandRunsFixtureImageEndToEnd$' ./internal/cli

race:
  @GOFLAGS="${GOFLAGS:+$GOFLAGS }-buildvcs=false" go test -race -count=1 ./...

coverage:
  @tmp="$(mktemp)"; \
  trap 'rm -f "$tmp"' EXIT; \
  GOFLAGS="${GOFLAGS:+$GOFLAGS }-buildvcs=false" go test -count=1 ./... -cover | tee "$tmp"; \
  awk 'BEGIN {bad=0} \
    /^ok[[:space:]]/ && /coverage:/ { \
      cov=$5; \
      gsub(/%/, "", cov); \
      if (cov < 70) { \
        print "coverage below 70%:", $2, cov "%"; \
        bad=1; \
      } \
    } \
    END {exit bad}' "$tmp"

check: verify-bootstrap fmt-check test race coverage

ci: check
