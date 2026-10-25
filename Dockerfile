# kpm — minimal runtime image (build from repo root: make docker-build)
# Digest is the multi-arch index, checked 2026-10-25.
FROM golang:1.27.3-alpine3.24@sha256:b8c9b82affbbb82a020bbcf998dad2ba9516944ac7e883524642a481a9505e9c AS build
ARG VERSION=dev
ARG COMMIT=unknown
ARG BUILDDATE=unknown
ARG BRANCH=unknown
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY cmd/ ./cmd/
COPY internal/ ./internal/
COPY configs/ ./configs/
RUN CGO_ENABLED=0 go build -trimpath \
	-ldflags "-s -w -X github.com/hrodrig/kpm/internal/cli.Version=${VERSION} -X github.com/hrodrig/kpm/internal/cli.Commit=${COMMIT} -X github.com/hrodrig/kpm/internal/cli.BuildDate=${BUILDDATE} -X github.com/hrodrig/kpm/internal/cli.Branch=${BRANCH}" \
	-o /kpm ./cmd/kpm

FROM gcr.io/distroless/static-debian13:nonroot@sha256:e2e927ec666bae08560abb3c55d0659eceabb657f56b6782ab500a9fc7f555e3
LABEL org.opencontainers.image.title="kpm"
LABEL org.opencontainers.image.description="Kubernetes Pods Metrics — sample and rightsizing recommendations"
LABEL org.opencontainers.image.source="https://github.com/hrodrig/kpm"
COPY --from=build /kpm /usr/local/bin/kpm
USER nonroot:nonroot
ENTRYPOINT ["/usr/local/bin/kpm"]
