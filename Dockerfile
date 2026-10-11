# kpm — minimal runtime image (build from repo root: make docker-build)
FROM golang:1.27.2-alpine3.24 AS build
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

FROM gcr.io/distroless/static-debian13:nonroot
LABEL org.opencontainers.image.title="kpm"
LABEL org.opencontainers.image.description="Kubernetes Pods Metrics — sample and rightsizing recommendations"
LABEL org.opencontainers.image.source="https://github.com/hrodrig/kpm"
COPY --from=build /kpm /usr/local/bin/kpm
USER nonroot:nonroot
ENTRYPOINT ["/usr/local/bin/kpm"]
