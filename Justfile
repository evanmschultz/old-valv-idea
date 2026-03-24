set shell := ["bash", "-eu", "-o", "pipefail", "-c"]

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
