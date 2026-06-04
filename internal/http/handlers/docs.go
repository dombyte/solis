// Package handlers provides HTTP request handlers for the Solis monitor API.
//
//go:generate swag init -g handlers.go -o ../../../docs
package handlers

// Swagger Documentation Generation
// ================================
//
// This file contains the swag generation directive for OpenAPI/Swagger documentation.
//
// To regenerate documentation after updating handler annotations:
//
//   Option 1: From project root
//
//   ```bash
//   swag init -g internal/http/handlers/handlers.go -o docs
//   ```
//
//   Option 2: From this directory
//
//   ```bash
//   go generate
//   ```
//
// Both commands will:
//   - Scan Swagger annotations (// @Summary, // @Description, etc.) in handlers.go
//   - Regenerate docs/docs.go, docs/swagger.json, and docs/swagger.yaml
//   - Embed the API specification with all endpoint definitions
//
// After regenerating, rebuild the application:
//
//   ```bash
//   go build ./...
//   ```
//
// The Swagger UI at /api will automatically serve the updated documentation.
