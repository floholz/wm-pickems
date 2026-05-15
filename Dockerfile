# ---- Stage 1: build the SvelteKit SPA ----
FROM node:22-alpine AS frontend
WORKDIR /app/frontend
COPY frontend/package.json frontend/package-lock.json* ./
RUN npm ci 2>/dev/null || npm install
COPY frontend/ ./
# adapter-static writes the SPA into /app/internal/web/build
RUN npm run build

# ---- Stage 2: build the Go binary with the SPA embedded ----
FROM golang:1.26-alpine AS backend
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
# Replace the committed placeholder with the freshly built SPA before embed.
COPY --from=frontend /app/internal/web/build ./internal/web/build
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /wm-pickems .

# ---- Stage 3: minimal runtime ----
FROM alpine:3.20
RUN apk add --no-cache ca-certificates tzdata && adduser -D -u 10001 app
COPY --from=backend /wm-pickems /usr/local/bin/wm-pickems
USER app
EXPOSE 8090
VOLUME ["/pb_data"]
ENTRYPOINT ["wm-pickems"]
CMD ["serve", "--http=0.0.0.0:8090", "--dir=/pb_data"]
