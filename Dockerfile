FROM node:26-alpine AS frontend
WORKDIR /frontend
# Accept git commit hash as build argument with default value
ARG VITE_GIT_COMMIT_HASH
COPY frontend/package.json frontend/package-lock.json ./
RUN npm ci
COPY frontend/ .
RUN npm run build

FROM node:26-alpine AS docs
WORKDIR /docs
COPY docs/package.json docs/package-lock.json ./
RUN npm ci
COPY docs/ .
RUN npm run build



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
COPY --from=frontend /frontend/dist /app/frontend/dist
COPY --from=docs /docs/dist /app/docs/dist
EXPOSE 8080
ENTRYPOINT ["/app/solis"]
