ARG GO_IMAGE=golang:1.27.2-alpine@sha256:f92b6ef800e499660581efdabdf25d9d817a9d124eaf900924f0504e7e27e12d
FROM --platform=$BUILDPLATFORM ${GO_IMAGE} AS build
ARG TARGETOS
ARG TARGETARCH
ARG VERSION=fixture-v1
WORKDIR /src
COPY go.mod main.go ./
RUN CGO_ENABLED=0 GOOS=${TARGETOS} GOARCH=${TARGETARCH} go build -trimpath -buildvcs=false \
    -ldflags="-s -w -X main.version=${VERSION}" -o /out/workload .
FROM scratch
ARG VERSION=fixture-v1
LABEL org.opencontainers.image.version=$VERSION
COPY --from=build /out/workload /workload
USER 65532:65532
EXPOSE 8080
ENTRYPOINT ["/workload"]
