FROM node:26-alpine AS frontend
WORKDIR /frontend
COPY frontend/package.json frontend/package-lock.json ./
RUN npm ci
COPY frontend/ .
# Same version as the binary (make docker passes it); the UI compares it with /api/version.
ARG VERSION=dev
RUN VITE_APP_VERSION=${VERSION} npm run build

FROM node:26-alpine AS docs
WORKDIR /docs
COPY docs/package.json docs/package-lock.json ./
RUN npm ci
COPY docs/ .
ARG VERSION=dev
RUN VITE_APP_VERSION=${VERSION} npm run build

FROM golang:1.27.1-alpine AS builder
WORKDIR /app
RUN apk --no-cache add ca-certificates tzdata

COPY go.mod go.sum* ./
RUN go mod download

# Build information for the startup log and `solis version` (make docker passes them).
ARG VERSION=dev
ARG COMMIT=unknown
ARG BUILD_DATE=unknown

COPY . .
# The web assets are embedded into the binary (go:embed), so they must exist before go build.
COPY --from=frontend /frontend/dist ./frontend/dist
COPY --from=docs /docs/dist ./docs/dist
RUN CGO_ENABLED=0 go build \
    -ldflags="-w -s -X main.Version=${VERSION} -X main.Commit=${COMMIT} -X main.BuildDate=${BUILD_DATE} -X main.GoVersion=$(go env GOVERSION)" \
    -a \
    -installsuffix cgo \
    -o solis ./cmd


FROM scratch
WORKDIR /app
COPY --from=ghcr.io/tarampampam/microcheck:1 /bin/httpcheck /bin/httpcheck
COPY --from=builder /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/
COPY --from=builder /usr/share/zoneinfo /usr/share/zoneinfo
COPY --from=builder /app/solis /app
EXPOSE 8080
ENTRYPOINT ["/app/solis"]
