set dotenv-load

# List recipes
default:
    @just --list

# Bootstrap a fresh checkout: install JS deps, generate code, build the frontend.
bootstrap: install gen build-web

# Install JS dependencies for the frontend (also installs buf TS plugins).
install:
    cd web && npm install

# Regenerate Go + TS from proto/. Run this after editing balloons.proto.
gen:
    buf generate

# Lint the protobuf files.
lint:
    buf lint

# Format the protobuf files in place.
fmt-proto:
    buf format -w

# Format Go source in place.
fmt-go:
    gofmt -w .

# Format everything.
fmt: fmt-proto fmt-go

# `go vet` across all packages.
vet:
    go vet ./...

# Run the Go tests.
test:
    go test ./...

# Sync go.mod / go.sum.
tidy:
    go mod tidy

# Build the frontend bundle once (web/dist/app.js + styles.css).
build-web:
    cd web && npm run build

# Watch frontend sources: rebuilds CSS and JS on change.
watch:
    cd web && npm run watch

# Run the Go server. `.env` is loaded automatically.
run:
    go run ./cmd/server

# Build a release binary into bin/server.
build-server:
    go build -o bin/server ./cmd/server

# Build everything for release.
build: build-web build-server

# Delete generated and built artifacts.
clean:
    rm -rf gen web/src/gen web/dist bin
