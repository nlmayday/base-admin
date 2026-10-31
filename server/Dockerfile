# syntax=docker/dockerfile:1
# Multi-stage build: a static binary in a distroless, non-root image.
FROM golang:1.27.3-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN --mount=type=cache,target=/go/pkg/mod go mod download
COPY . .
ARG VERSION=dev
RUN --mount=type=cache,target=/go/pkg/mod --mount=type=cache,target=/root/.cache/go-build \
    CGO_ENABLED=0 go build -trimpath -ldflags="-s -w -X base-admin/server/internal/platform/buildinfo.Version=${VERSION}" -o /out/admin ./cmd/admin

FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=build /out/admin /admin
# Local file storage (file.driver=local) lives here; mount a volume.
VOLUME ["/data"]
USER nonroot:nonroot
EXPOSE 8080 9090
ENTRYPOINT ["/admin"]
CMD ["serve"]
