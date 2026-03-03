# Unikernel Demo

A throwaway demo proving a Go HTTP microservice can be built and deployed as a [nanos](https://nanovms.com/) unikernel AMI on AWS using [nanovms/ops](https://github.com/nanovms/ops).

## What's Here

Single-file Go service using [Huma v2](https://huma.rocks/) with three endpoints:

| Method | Path       | Description                              |
|--------|------------|------------------------------------------|
| GET    | `/healthz` | Returns `{"status": "ok"}`              |
| GET    | `/version` | Returns build version, commit, timestamp |
| POST   | `/echo`    | Echoes the JSON request body back        |

## Prerequisites

- Go 1.25+
- [ops](https://ops.city/) (`curl https://ops.city/get.sh -sSfL | sh`)
- QEMU (`brew install qemu` on macOS)
- AWS credentials configured (`~/.aws/credentials`) for cloud deployment

## Quick Start

```sh
# Run unit tests (host-side)
go test -v ./...

# Build the binary (static Linux amd64 ELF)
script/build

# Run integration tests (boots unikernel, hits all endpoints)
script/test-unikernel

# Run locally as a unikernel (requires ops + QEMU)
script/run

# Test it manually
curl localhost:8080/healthz
curl localhost:8080/version
curl -X POST localhost:8080/echo -H 'Content-Type: application/json' -d '{"hello": "world"}'
```

## Deploy to AWS

### Prerequisites

1. **AWS credentials** configured via `~/.aws/credentials` or environment variables
2. **An S3 bucket** in your target region (ops CLI requires it even though image upload uses EBS Direct APIs)

On first run, ops will create a `vmimport` IAM role automatically. Your credentials need permissions for EC2, EBS, S3, and IAM role management.

### Build, Image, Deploy

AWS settings are passed via environment variables, keeping `config.json` cloud-agnostic:

| Variable | Required | Default | Description |
|----------|----------|---------|-------------|
| `OPS_BUCKET` | Yes | -- | S3 bucket you own in the target region |
| `AWS_ZONE` | No | `us-east-1` | AWS region |
| `INSTANCE_FLAVOR` | No | `t2.micro` | EC2 instance type |
| `IMAGE_NAME` | No | `unikernel-demo` | AMI name |

```sh
# Build the binary
script/build

# Create an AMI (uploads via EBS Direct, registers AMI)
OPS_BUCKET=my-ops-bucket script/image

# Launch a t2.micro EC2 instance from the AMI
script/deploy

# Or override region and instance type:
AWS_ZONE=us-west-2 INSTANCE_FLAVOR=t3.small script/deploy
```

### Manage Instances

```sh
# List running instances
ops instance list -t aws -z us-east-1

# View console output / logs
ops instance logs <instance-name> -t aws -z us-east-1

# Delete an instance
ops instance delete <instance-name> -t aws -z us-east-1

# Delete the AMI when done
ops image delete unikernel-demo -t aws -z us-east-1
```

### Quick Validation

Once the instance is running, `ops instance list` shows its public IP:

```sh
PUBLIC_IP=$(ops instance list -t aws -z us-east-1 2>/dev/null | grep unikernel | awk '{print $NF}')
curl http://${PUBLIC_IP}:8080/healthz
curl http://${PUBLIC_IP}:8080/version
```

## Project Structure

```
main.go                Single-file Huma v2 service (all handlers)
main_test.go           Host-side smoke tests for all endpoints
config.json            Nanos/ops unikernel + AWS cloud configuration
script/build           Cross-compile static Linux amd64 binary with version ldflags
script/test-unikernel  Boot unikernel in QEMU and integration test all endpoints
script/run             Run locally as a unikernel via ops + QEMU
script/image           Create AWS AMI via ops (EBS Direct upload)
script/deploy          Launch EC2 instance from AMI
nanos.md               Nanos unikernel research: limitations, Go compat, testing
```

## Testing

There are two layers of testing:

**Unit tests** run on the host machine using Go's standard `testing` package and `humatest`:

```sh
go test -v ./...
```

These validate handler logic but do **not** prove the binary works inside the nanos unikernel (different syscalls, networking stack, filesystem).

**Unikernel integration tests** boot the actual binary as a unikernel via `ops run` + QEMU and validate all endpoints over HTTP:

```sh
script/build
script/test-unikernel
```

The integration test script:
1. Boots the unikernel in the background
2. Waits for `/healthz` to respond (retry with 2s backoff, 30s timeout)
3. Tests all 3 endpoints (healthz, version, echo) with HTTP assertions
4. Reports pass/fail and cleans up the QEMU process

This is the same pattern the [nanos project itself uses](https://github.com/nanovms/nanos/tree/master/test/e2e) to validate applications. See [nanos.md](nanos.md) for more on testing limitations.

## Build Details

The build script cross-compiles with `GOOS=linux GOARCH=amd64 CGO_ENABLED=0` to produce a statically linked ELF binary. Version info is injected via ldflags:

```sh
# Override version at build time
VERSION=1.0.0 script/build
```

Environment variables: `VERSION`, `GIT_COMMIT` (defaults to git describe/rev-parse).

## Unikernel Constraints

Nanos unikernels have specific limitations to be aware of:

- Single process only (no fork/exec/shell)
- No SSH access
- `CGO_ENABLED=0` required (pure-Go DNS resolver)
- Files must be explicitly included in `config.json`
- Ephemeral filesystem (no persistent local storage without volumes)

See [nanos.md](nanos.md) for comprehensive research on nanos limitations, Go compatibility, and operational considerations.
