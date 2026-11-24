ARG GO_IMAGE=golang:1.27.2-alpine@sha256:f92b6ef800e499660581efdabdf25d9d817a9d124eaf900924f0504e7e27e12d
FROM --platform=$BUILDPLATFORM ${GO_IMAGE} AS build
ARG TARGETOS
ARG TARGETARCH
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY integration/fixtures/barrier ./integration/fixtures/barrier
RUN CGO_ENABLED=0 GOOS=${TARGETOS} GOARCH=${TARGETARCH} go build -trimpath -buildvcs=false -ldflags='-s -w' -o /out/barrier ./integration/fixtures/barrier
FROM scratch
COPY --from=build /out/barrier /barrier
USER 65532:65532
ENTRYPOINT ["/barrier"]
