# Unikernel Demo Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Build a single-file Go HTTP service with Huma v2, deployable as a nanos unikernel AMI on AWS.

**Architecture:** One `main.go` with three endpoints (healthz, version, echo) using Huma v2's stdlib adapter (`humago`). Build scripts cross-compile a static Linux binary, then use `ops` to create and deploy an AMI.

**Tech Stack:** Go 1.25, Huma v2.37.2, nanovms/ops, AWS (AMI/EC2)

---

### Task 1: Initialize Go module and add Huma dependency

**Files:**
- Create: `go.mod`

**Step 1: Initialize the module**

Run: `go mod init github.com/shakefu/unikernel-demo`
Expected: `go.mod` created

**Step 2: Add Huma dependency**

Run: `go get github.com/danielgtaylor/huma/v2@v2.37.2`
Expected: `go.mod` and `go.sum` updated with huma dependency

**Step 3: Commit**

```bash
git add go.mod go.sum
git commit -m "chore: initialize go module with huma v2 dependency"
```

---

### Task 2: Write the smoke test

**Files:**
- Create: `main_test.go`

**Step 1: Write the failing test**

```go
package main

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/danielgtaylor/huma/v2/humatest"
)

func TestHealthz(t *testing.T) {
	_, api := humatest.New(t, huma.DefaultConfig("Test", "0.0.0"))
	registerRoutes(api)

	resp := api.Get("/healthz")
	if resp.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.Code)
	}
	body := resp.Body.String()
	if !strings.Contains(body, `"status":"ok"`) {
		t.Fatalf("expected ok status, got %s", body)
	}
}

func TestVersion(t *testing.T) {
	_, api := humatest.New(t, huma.DefaultConfig("Test", "0.0.0"))
	registerRoutes(api)

	resp := api.Get("/version")
	if resp.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.Code)
	}
	body := resp.Body.String()
	if !strings.Contains(body, `"version"`) {
		t.Fatalf("expected version field, got %s", body)
	}
}

func TestEcho(t *testing.T) {
	_, api := humatest.New(t, huma.DefaultConfig("Test", "0.0.0"))
	registerRoutes(api)

	resp := api.Post("/echo", strings.NewReader(`{"message":"hello","count":42}`))
	if resp.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.Code)
	}
	body := resp.Body.String()
	if !strings.Contains(body, `"message":"hello"`) {
		t.Fatalf("expected echo of message, got %s", body)
	}
	if !strings.Contains(body, `"count":42`) {
		t.Fatalf("expected echo of count, got %s", body)
	}
}

func TestEchoEmpty(t *testing.T) {
	_, api := humatest.New(t, huma.DefaultConfig("Test", "0.0.0"))
	registerRoutes(api)

	resp := api.Post("/echo", strings.NewReader(`{}`))
	if resp.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.Code)
	}
}
```

Note: The exact `humatest` API may need adjustment — check the [humatest docs](https://pkg.go.dev/github.com/danielgtaylor/huma/v2/humatest) during implementation. The key contract is: `registerRoutes(api)` sets up all handlers, then we test via HTTP calls.

**Step 2: Run test to verify it fails**

Run: `go test -v -run TestHealthz ./...`
Expected: FAIL — `registerRoutes` not defined

**Step 3: Commit**

```bash
git add main_test.go
git commit -m "test: add smoke tests for healthz, version, and echo endpoints"
```

---

### Task 3: Implement main.go with all handlers

**Files:**
- Create: `main.go`

**Step 1: Write the implementation**

```go
package main

import (
	"context"
	"fmt"
	"net/http"
	"os"

	"github.com/danielgtaylor/huma/v2"
	"github.com/danielgtaylor/huma/v2/adapters/humago"
)

var (
	Version   = "0.0.0"
	GitCommit = "unknown"
	BuildTime = "unknown"
)

type HealthOutput struct {
	Body struct {
		Status string `json:"status" doc:"Health status"`
	}
}

type VersionOutput struct {
	Body struct {
		Version   string `json:"version" doc:"Semantic version"`
		GitCommit string `json:"gitCommit,omitempty" doc:"Git commit hash"`
		BuildTime string `json:"buildTime,omitempty" doc:"Build timestamp (UTC)"`
	}
}

type EchoInput struct {
	Body any `json:"body" doc:"Arbitrary JSON to echo back"`
}

type EchoOutput struct {
	Body any
}

func registerRoutes(api huma.API) {
	huma.Register(api, huma.Operation{
		OperationID: "healthz",
		Method:      http.MethodGet,
		Path:        "/healthz",
		Summary:     "Health check",
	}, func(_ context.Context, _ *struct{}) (*HealthOutput, error) {
		out := &HealthOutput{}
		out.Body.Status = "ok"
		return out, nil
	})

	huma.Register(api, huma.Operation{
		OperationID: "getVersion",
		Method:      http.MethodGet,
		Path:        "/version",
		Summary:     "Build version info",
	}, func(_ context.Context, _ *struct{}) (*VersionOutput, error) {
		out := &VersionOutput{}
		out.Body.Version = Version
		out.Body.GitCommit = GitCommit
		out.Body.BuildTime = BuildTime
		return out, nil
	})

	huma.Register(api, huma.Operation{
		OperationID: "echo",
		Method:      http.MethodPost,
		Path:        "/echo",
		Summary:     "Echo JSON body",
	}, func(_ context.Context, in *EchoInput) (*EchoOutput, error) {
		return &EchoOutput{Body: in.Body}, nil
	})
}

func main() {
	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	mux := http.NewServeMux()
	api := humago.New(mux, huma.DefaultConfig("Unikernel Demo", Version))
	registerRoutes(api)

	fmt.Printf("Listening on :%s\n", port)
	if err := http.ListenAndServe(":"+port, mux); err != nil {
		fmt.Fprintf(os.Stderr, "server error: %v\n", err)
		os.Exit(1)
	}
}
```

Note: The echo handler's input/output types may need tweaking based on how Huma handles `any` body types. The implementer should consult Huma docs if the tests don't pass as-is and adjust the type definitions accordingly. The key behavior is: POST JSON in, same JSON out.

**Step 2: Run tests**

Run: `go test -v ./...`
Expected: All 4 tests PASS

**Step 3: Quick manual sanity check**

Run: `go run . &` then `curl localhost:8080/healthz` then kill the background process.
Expected: `{"status":"ok"}`

**Step 4: Commit**

```bash
git add main.go
git commit -m "feat: add huma v2 service with healthz, version, and echo endpoints"
```

---

### Task 4: Create ops config and build script

**Files:**
- Create: `config.json`
- Create: `script/build`

**Step 1: Create config.json**

```json
{
  "Env": {
    "PORT": "8080"
  },
  "RunConfig": {
    "Ports": ["8080"],
    "Memory": "512M"
  },
  "CloudConfig": {
    "BucketName": "",
    "Zone": "us-east-1"
  }
}
```

Note: `BucketName` must be filled in with an actual S3 bucket before running `script/image`. Leave it empty for now — the scripts will validate it.

**Step 2: Create script/build**

```bash
#!/usr/bin/env bash
set -euo pipefail

VERSION="${VERSION:-$(git describe --tags 2>/dev/null || echo "0.0.0")}"
GIT_COMMIT="${GIT_COMMIT:-$(git rev-parse --short HEAD 2>/dev/null || echo "unknown")}"
BUILD_TIME="$(date -u +"%Y-%m-%dT%H:%M:%SZ")"

LDFLAGS="-X main.Version=${VERSION} -X main.GitCommit=${GIT_COMMIT} -X main.BuildTime=${BUILD_TIME}"

echo "Building dist/server..."
echo "  Version:   ${VERSION}"
echo "  Commit:    ${GIT_COMMIT}"
echo "  BuildTime: ${BUILD_TIME}"

mkdir -p dist
GOOS=linux CGO_ENABLED=0 go build -ldflags "${LDFLAGS}" -o dist/server .

echo "Built dist/server"
file dist/server
```

**Step 3: Run the build**

Run: `script/build`
Expected: `dist/server` created, `file` output shows `ELF 64-bit LSB executable`

**Step 4: Add dist/ to .gitignore**

```
dist/
```

**Step 5: Commit**

```bash
git add config.json script/build .gitignore
git commit -m "build: add ops config and build script"
```

---

### Task 5: Create ops run/image/deploy scripts

**Files:**
- Create: `script/run`
- Create: `script/image`
- Create: `script/deploy`

**Step 1: Create script/run**

```bash
#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_DIR="$(dirname "${SCRIPT_DIR}")"

BINARY="${PROJECT_DIR}/dist/server"

if [[ ! -f "${BINARY}" ]]; then
    echo "Binary not found at ${BINARY}. Run script/build first."
    exit 1
fi

echo "Running unikernel locally on port 8080..."
ops run -p 8080 -c "${PROJECT_DIR}/config.json" "${BINARY}"
```

**Step 2: Create script/image**

```bash
#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_DIR="$(dirname "${SCRIPT_DIR}")"

BINARY="${PROJECT_DIR}/dist/server"
IMAGE_NAME="${IMAGE_NAME:-unikernel-demo}"

if [[ ! -f "${BINARY}" ]]; then
    echo "Binary not found at ${BINARY}. Run script/build first."
    exit 1
fi

# Validate AWS bucket is set in config
BUCKET=$(python3 -c "import json; print(json.load(open('${PROJECT_DIR}/config.json'))['CloudConfig']['BucketName'])" 2>/dev/null || echo "")
if [[ -z "${BUCKET}" ]]; then
    echo "Error: CloudConfig.BucketName is empty in config.json."
    echo "Set it to an S3 bucket you own before creating an image."
    exit 1
fi

echo "Creating AWS AMI: ${IMAGE_NAME}..."
ops image create "${BINARY}" \
    -c "${PROJECT_DIR}/config.json" \
    -i "${IMAGE_NAME}" \
    -t aws

echo "AMI created: ${IMAGE_NAME}"
echo "List images with: ops image list -t aws"
```

**Step 3: Create script/deploy**

```bash
#!/usr/bin/env bash
set -euo pipefail

IMAGE_NAME="${IMAGE_NAME:-unikernel-demo}"

echo "Launching instance from AMI: ${IMAGE_NAME}..."
ops instance create "${IMAGE_NAME}" \
    -t aws \
    --port 8080

echo ""
echo "Instance launched. List instances with: ops instance list -t aws"
echo "View logs with: ops instance logs <instance-name> -t aws"
echo "Delete with: ops instance delete <instance-name> -t aws"
```

**Step 4: Make all scripts executable**

Run: `chmod +x script/run script/image script/deploy`

**Step 5: Commit**

```bash
git add script/run script/image script/deploy
git commit -m "build: add ops run, image, and deploy scripts"
```

---

### Task 6: Verify local build and test end-to-end

**Step 1: Run full test suite**

Run: `go test -v ./...`
Expected: All tests pass

**Step 2: Run the build**

Run: `script/build`
Expected: `dist/server` created successfully

**Step 3: Install ops (if not already installed)**

Run: `ops version` — if not found:
Run: `curl https://ops.city/get.sh -sSfL | sh`

**Step 4: Test locally with ops**

Run: `script/run`
In another terminal: `curl localhost:8080/healthz`
Expected: `{"status":"ok"}`

Note: This step requires QEMU installed (`brew install qemu`). If ops or QEMU isn't available, skip this step — the build verification in step 2 is sufficient to prove the binary is correct.

**Step 5: Commit any final adjustments**

```bash
git add -A
git commit -m "chore: final adjustments from local testing"
```

(Only if there are changes to commit.)
