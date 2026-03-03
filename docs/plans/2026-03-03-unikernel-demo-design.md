# Unikernel Demo Service — Design

## Purpose

Throwaway demo proving a Go HTTP microservice can be built and deployed as a nanos unikernel AMI on AWS using [nanovms/ops](https://github.com/nanovms/ops).

## Architecture

Single-file Go service using Huma v2 REST framework. Three public endpoints, no auth, no database. Deployed as an EC2 instance from a unikernel AMI created by `ops`.

## Endpoints

| Method | Path | Description |
|--------|------|-------------|
| GET | /healthz | Returns `{"status": "ok"}` |
| GET | /version | Returns build-time version, commit, timestamp |
| POST | /echo | Echoes the JSON request body back |

Version info injected via `go build -ldflags "-X main.Version=... -X main.GitCommit=... -X main.BuildTime=..."`.

Echo accepts arbitrary JSON (`any` type body), returns it unchanged.

## Project Structure

```
main.go              # Huma setup, all handlers, version vars
main_test.go         # Smoke test: start server, hit all endpoints
config.json          # Ops/nanos configuration
go.mod / go.sum      # Module: huma v2 dependency
script/build         # Cross-compile Go binary with ldflags
script/run           # ops run locally (port-forwarded)
script/image         # ops image create → AWS AMI
script/deploy        # ops instance create from AMI
```

## Build & Deploy

**Build constraints:** `GOOS=linux CGO_ENABLED=0` (nanos requires static ELF binary, pure-Go DNS resolver avoids libc crashes).

**config.json:**
```json
{
  "Env": { "PORT": "8080" },
  "RunConfig": {
    "Ports": ["8080"],
    "Memory": "512M"
  },
  "CloudConfig": {
    "BucketName": "<s3-bucket>",
    "Zone": "us-east-1"
  }
}
```

**Scripts:**
- `script/build` — compiles `dist/server` with version injection
- `script/run` — `ops run -p 8080 -c config.json dist/server`
- `script/image` — `ops image create dist/server -c config.json -i unikernel-demo -t aws`
- `script/deploy` — `ops instance create unikernel-demo -t aws --port 8080`

## Testing

Minimal smoke test: start the server on a random port, HTTP GET /healthz, GET /version, POST /echo with a JSON body, assert expected responses.

## Key Constraints (nanos/ops)

- Single process only (no fork/exec)
- No shell, no SSH
- `CGO_ENABLED=0` required (pure-Go DNS)
- Files must be explicitly included in config
- Ephemeral filesystem (no persistent local storage)
- NTP clock sync takes ~53s after boot
