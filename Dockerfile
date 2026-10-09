# Builds a linux/amd64 image with a static, CGO-free binary.
# Override base images with --build-arg (e.g. to use a mirror), and GOPROXY
# where proxy.golang.org is unreachable.
ARG GO_IMAGE=golang:1.27-alpine
ARG GOPROXY=https://proxy.golang.org,direct
ARG RUNTIME_IMAGE=alpine:3

FROM --platform=linux/amd64 ${GO_IMAGE} AS build
WORKDIR /src
ARG GOPROXY
ENV GOPROXY=${GOPROXY}
COPY go.mod go.sum ./
RUN go mod download
COPY . .
ARG VERSION=dev
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -trimpath -tags timetzdata \
    -ldflags "-s -w -X github.com/aligh5331/personal-budget-manger/internal/version.Version=${VERSION}" \
    -o /out/bot ./cmd/bot

FROM --platform=linux/amd64 ${RUNTIME_IMAGE}
RUN apk add --no-cache ca-certificates \
    && adduser -D -H -u 10001 bot \
    && mkdir -p /data && chown bot:bot /data
COPY --from=build /out/bot /usr/local/bin/bot
USER bot
ENV DATA_DIR=/data \
    LISTEN_ADDR=:8080
VOLUME ["/data"]
EXPOSE 8080
HEALTHCHECK --interval=30s --timeout=5s --start-period=30s \
    CMD wget -qO- http://127.0.0.1:8080/healthz >/dev/null || exit 1
ENTRYPOINT ["/usr/local/bin/bot"]
