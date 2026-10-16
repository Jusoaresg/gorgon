# Stage 1: Build the backend
FROM --platform=$BUILDPLATFORM golang:1.26-alpine AS builder

ARG TARGETOS
ARG TARGETARCH

WORKDIR /app

COPY go.mod go.sum ./
RUN --mount=type=cache,target=/go/pkg/mod \
    go mod download

COPY . .

RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    CGO_ENABLED=0 GOOS=${TARGETOS:-linux} GOARCH=${TARGETARCH} \
    go build -trimpath -ldflags="-s -w" -o /app/main .


# Stage 2: Create the final image
FROM alpine:3.22
WORKDIR /app

ENV IN_DOCKER=true
ENV GORGON_PORT=8080

RUN adduser -D -u 1000 -h /app gorgon \
    && mkdir -p /downloads /shows /configs \
    && chown -R gorgon:gorgon /app /downloads /shows /configs

COPY --from=builder /app/main /app/main

USER gorgon

EXPOSE 8080

HEALTHCHECK --interval=30s --timeout=5s --start-period=15s --retries=3 \
  CMD wget -qO- http://localhost:${GORGON_PORT}/ >/dev/null || exit 1

CMD ["/app/main"]
