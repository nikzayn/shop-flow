# ---- build stage: runs on YOUR machine's architecture ----
FROM --platform=$BUILDPLATFORM golang:???-alpine AS build   # ??? = Go version in go.mod
ARG TARGETOS TARGETARCH                                      # set by buildx for each target platform
WORKDIR /src
    
COPY go.mod go.sum ./
RUN --mount=type=cache,target=/go/pkg/mod go mod download    # cached until go.mod changes
    
COPY ??? ???                                                  # copy only what the build needs
RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    CGO_ENABLED=0 GOOS=$TARGETOS GOARCH=$TARGETARCH \
    go build -trimpath -ldflags="-s -w" -o /out/catalog ???  # ??? = package path
    
    # ---- runtime stage: just the binary ----
FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=build ??? /catalog
USER nonroot:nonroot
EXPOSE 8080
ENTRYPOINT ["/catalog"]