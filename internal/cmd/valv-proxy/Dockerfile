# Stage 1: build
# Build context is the repo root: `docker buildx build -f internal/cmd/valv-proxy/Dockerfile .`
FROM golang:1.26.1-alpine AS builder

WORKDIR /src
COPY . .

RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -o /valv-proxy ./internal/cmd/valv-proxy

# Stage 2: minimal runtime image
# VALV_PROXY_ALLOWLIST is NOT baked — inject at `docker run` time via -e.
# D.1 (Service.Provision) sets both VALV_PROXY_ADDR and VALV_PROXY_ALLOWLIST.
FROM gcr.io/distroless/static-debian12:nonroot

COPY --from=builder /valv-proxy /valv-proxy

ENV VALV_PROXY_ADDR=:8080

EXPOSE 8080

ENTRYPOINT ["/valv-proxy"]
