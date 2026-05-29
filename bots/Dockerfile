# wm-pickems bot — one image, one container per bot. Configuration is entirely
# via environment (BOT_EMAIL / BOT_PASSWORD / ANTHROPIC_API_KEY / CLAUDE_MODEL /
# WMP_BASE_URL / BOT_LEAGUE_CODE), so the same image runs every bot — give each
# container its own env. Build context is this bots/ directory (separate Go
# module from the main app).

# ---- Stage 1: build the Go binary ----
FROM golang:1.26-alpine AS build
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /wm-pickems-bot .

# ---- Stage 2: minimal runtime ----
FROM alpine:3.20

ARG VERSION=dev
ARG REVISION=unknown
ARG CREATED=
LABEL org.opencontainers.image.title="wm-pickems-bot" \
      org.opencontainers.image.description="Claude (and future) bot players for wm-pickems" \
      org.opencontainers.image.url="https://github.com/floholz/wm-pickems" \
      org.opencontainers.image.source="https://github.com/floholz/wm-pickems" \
      org.opencontainers.image.licenses="GPL-3.0-only" \
      org.opencontainers.image.version="${VERSION}" \
      org.opencontainers.image.revision="${REVISION}" \
      org.opencontainers.image.created="${CREATED}"

# ca-certificates: HTTPS to the Anthropic API and the (possibly TLS) app.
# tzdata: correct time handling. No server, so no port or volume.
RUN apk add --no-cache ca-certificates tzdata \
	&& adduser -D -u 10001 app
COPY --from=build /wm-pickems-bot /usr/local/bin/wm-pickems-bot
USER app
ENTRYPOINT ["wm-pickems-bot"]
# Long-running container: loop on a schedule. Override (e.g. drop --loop for a
# one-shot under an external scheduler) by setting the container command.
CMD ["--loop", "--interval=1h"]
