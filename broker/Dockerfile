ARG GO_IMAGE=golang:1.25-alpine
FROM --platform=$BUILDPLATFORM ${GO_IMAGE} AS build

ARG TARGETOS
ARG TARGETARCH
ARG VERSION=dev

WORKDIR /src
# Copy module files first for layer caching, then the source.
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=${TARGETOS} GOARCH=${TARGETARCH} \
    go build -trimpath -buildvcs=false \
    -ldflags="-s -w -X main.buildVersion=${VERSION}" \
    -o /out/platform ./cmd/platform

FROM gcr.io/distroless/static-debian12:nonroot
# Distroless static provides a nonroot user (65532) and CA certificates for
# reaching container registries over HTTPS, while staying minimal.
COPY --from=build /out/platform /platform
USER 65532:65532
EXPOSE 8080
ENTRYPOINT ["/platform"]
