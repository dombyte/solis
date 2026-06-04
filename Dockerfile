FROM node:26-alpine AS frontend
WORKDIR /frontend
COPY frontend/package.json frontend/package-lock.json ./
RUN npm install
COPY frontend/ .
RUN npm run build



FROM golang:1.26.4-alpine AS builder
WORKDIR /app
RUN apk --no-cache add ca-certificates tzdata

COPY go.mod go.sum* ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 go build \
    -ldflags="-w -s" \
    -a \
    -installsuffix cgo \
    -o solis ./cmd/main.go


FROM scratch
WORKDIR /app
COPY --from=ghcr.io/tarampampam/microcheck:1 /bin/httpcheck /bin/httpcheck
COPY --from=builder /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/
COPY --from=builder /usr/share/zoneinfo /usr/share/zoneinfo
COPY --from=builder /app/solis /app
COPY --from=frontend /frontend/dist /app/frontend/dist
EXPOSE 8080
ENTRYPOINT ["/app/solis"]
