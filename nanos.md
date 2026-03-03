# Nanos Unikernel: What You Need to Know

## TL;DR

Nanos is a production-oriented unikernel that runs a single application as a lightweight VM. It supports x86-64 (primary) and ARM64 (maturing). Go works well with `CGO_ENABLED=0`, but some syscalls are missing and there's no fork/exec/shell by design. Debugging in production is hard -- no SSH, no shell, GDB only works locally. Active development, ~3k GitHub stars, used by the U.S. Air Force.

---

## Platform Support

| Architecture | Maturity | Local Dev (Apple Silicon) | AWS |
|---|---|---|---|
| **x86-64** | Production | QEMU emulated (slow) | Full support (AMI) |
| **ARM64** | Maturing | QEMU + HVF (native speed) | Graviton (t4g confirmed) |
| **RISC-V** | Experimental | QEMU only | No |

**No bare metal** -- always requires a hypervisor (QEMU/KVM, Firecracker, Hyper-V, VMware, Cloud Hypervisor).

Supported cloud providers: AWS, GCP, Azure, OCI, Vultr, DigitalOcean, UpCloud, OpenStack, Proxmox, vSphere.

---

## Go Compatibility

### Works Well

| Feature | Status |
|---|---|
| Goroutines, GC, scheduler | Fully functional |
| `net/http` server & client | Works (tested under load) |
| TLS/HTTPS | Works (include CA certs in image) |
| DNS resolution | Works with `CGO_ENABLED=0` |
| `os/signal` (SIGTERM) | Works (catches VM poweroff) |
| `time.Now()`, timers | Works |
| Basic file I/O | Works |
| CGO (static) | **Recommended**: `CGO_ENABLED=0` |
| CGO (dynamic) | Works but must package .so files |

### Does NOT Work

| Feature | Why |
|---|---|
| `os/exec` | **No fork/exec by design** -- any library that shells out will fail |
| `/proc/self/maps` | Not implemented |
| `/sys` filesystem | Not implemented |
| `splice()` syscall | Missing (open issue #2106) -- Go falls back to slower userspace copy |
| Go ARM64 | Experimental, issues reported (#1965) |

### Go-Specific Gotchas

- **Always build with `CGO_ENABLED=0 GOOS=linux`** -- avoids libc DNS crashes
- **Use `import _ "time/tzdata"`** to embed timezone data (no `/usr/share/zoneinfo` by default)
- **Set `NameServers`** in config.json -- defaults to 8.8.8.8
- **Check dependencies for `os/exec` imports** -- any library that calls `exec.Command()` breaks
- **Allocate sufficient memory** -- below thresholds, performance degrades sharply vs. containers
- **Each Go release can break things** -- nanos tracks syscall changes, use latest stable of both

---

## Key Limitations

### Fundamental (by design)

- **Single process only** -- no fork, no exec, no shell, no SSH
- **No users/permissions** -- `getuid`, `chmod`, etc. are stubbed
- **No interactive access** -- communicate only via network
- **Immutable deployments** -- every change requires a new image
- **Hypervisor required** -- cannot run on bare metal

### Syscalls (~90+ missing)

Most impactful missing syscalls:

| Syscall | Impact |
|---|---|
| `fork()` / `vfork()` / `execve()` | Multi-process servers (nginx workers, Varnish) fail |
| `splice()` / `tee()` | Performance-sensitive I/O paths fall back to slower copy |
| `ptrace()` | No debugger attachment in production |
| System V IPC (shmget, semget, msgget) | All missing |
| `mount()` / `umount()` | Volumes configured at build time only |
| `seccomp()` | Container-style sandboxing unavailable |

### Networking

- **No raw sockets** -- no ping, no packet capture, no custom protocols
- **No SCTP**
- `SO_REUSEPORT` not implemented
- `TCP_CORK`, `TCP_DEFER_ACCEPT`, `TCP_QUICKACK`, `TCP_FASTOPEN` all non-functional
- lwIP stack (not Linux TCP/IP) -- subtle behavioral differences under load
- `AF_UNIX` SCM_RIGHTS (passing file descriptors) not supported

### Filesystem (TFS)

- No POSIX ACLs or extended attributes (xattrs recently added)
- `flock()` is stubbed (no-op) -- no actual file locking
- No sparse files
- No disk quotas
- `/proc` very limited, `/sys` minimal
- Ephemeral by default -- use volumes for persistence

### Signals

- Basic signal handling works, `SIGTERM` catchable
- `SA_NOCLDSTOP`, `SA_NOCLDWAIT`, `SA_RESETHAND` unsupported
- Cross-process signals impossible (single process)
- Core dumps not implemented

---

## Operational Warnings

### Debugging

- **No production debugging** -- GDB only works locally with QEMU, not cloud instances
- Must reproduce issues locally to diagnose
- `--trace` flag and `--syscall-summary` provide some visibility
- Compile with `-g` for useful GDB output

### Logging

- **No default logging in cloud** -- logs vanish unless you configure a klib
- CloudWatch klib for AWS (requires IAM role + `tls` klib)
- Syslog klib for any platform
- Must choose logging strategy before deploying (can't add klibs to running instance)

### Monitoring

- **No Prometheus exporter** built in -- your app must expose metrics
- **No agent-based monitoring** (no Datadog/New Relic agents -- they need separate processes)
- CloudWatch metrics klib provides memory utilization on AWS
- Use cloud-native health checks (ALB/NLB target groups)

### Deployments

- **No built-in orchestration** -- no rolling updates, no canary, no blue/green
- Build your own with cloud-native tools (ASGs, Terraform, etc.)
- Every config change = new image + new instance
- NanoVMs discourages Kubernetes ("evaluate if you really need kubernetes")

### Secrets

- `Env` in config.json is baked into the image (extractable)
- `userdata_env` klib reads from IMDS at boot (not a secure secrets store)
- `cloud_init` klib can fetch from HTTP(S) sources
- No native Vault/Secrets Manager integration

### Time

- NTP requires `ntp` klib -- not built in
- Without NTP, clock depends on hypervisor TSC
- ~53 seconds to sync after boot (4 samples x 16-second poll)

---

## Performance

| Metric | Nanos | vs. Docker |
|---|---|---|
| Boot time | ~12ms median | 13-18x faster |
| Image size (Go) | 7.6 MB | Comparable |
| Throughput (Go, 8 CPU) | 76k req/s | Docker 2x faster |
| Throughput (Go, 1 CPU, 512MB) | 70k req/s | **Nanos 11.7x faster** |

**Key finding**: Nanos excels in resource-constrained environments (1 CPU, limited memory). Under generous resources, Linux/Docker often wins on raw throughput. **Below a memory threshold, Nanos performance degrades sharply.**

---

## Security

### Strengths
- No shell, no users, no privilege escalation path
- ASLR, W^X, stack canaries all enabled
- Minimal codebase (~10s of thousands of lines vs. Linux 15M+)
- Rated best among unikernels by X41 D-Sec security audit
- `sandbox` klib provides pledge/unveil (OpenBSD-style)

### Weaknesses
- Single address space -- compromised app has full access to network stack
- Hypervisor escape defeats everything
- Weak entropy on virtual platforms (rdtsc-based seeding)
- IMDS/SSRF risk without firewall klib
- `curl | sh` install with only MD5 hashes

---

## Community & Maturity

- **GitHub**: nanos 3,073 stars / ops 1,468 stars (active, last push March 2026)
- **Release cadence**: every 3-6 months (0.1.54 released June 2025)
- **Team**: Full-time paid kernel engineers (NanoVMs, Inc.)
- **Notable users**: U.S. Air Force (ABMS, $950M ceiling IDIQ), Amgen
- **88 open kernel issues, 144 open ops issues** -- actively maintained but still maturing
- **Documentation**: acknowledged by team as needing work

---

## Testing Go Programs in Nanos

### The Problem

Running `go test` on the host does **not** validate that your binary works inside nanos. The unikernel has a different syscall surface, networking stack (lwIP vs. Linux TCP/IP), and filesystem (TFS). A test suite that passes on Linux may fail inside nanos due to missing syscalls or behavioral differences.

### Approach: HTTP Smoke Tests (Recommended)

The nanos project itself uses a "test from outside" pattern ([test/e2e/e2e.go](https://github.com/nanovms/nanos/tree/master/test/e2e)):

1. Build the binary
2. Boot it as a unikernel via `ops run` in the background
3. Make HTTP requests from the host with retry/backoff
4. Validate responses
5. Kill the QEMU process group on teardown

This is what `script/test-unikernel` implements in this project.

### Alternative: `go test -c` Inside Nanos

Compiling a Go test binary with `go test -c` and running it via `ops run` is **architecturally sound but undocumented**. The test binary is a standard ELF executable. Pass test flags via `Args` in config.json:

```json
{ "Args": ["-test.v", "-test.run", "TestSomething"] }
```

### Critical Limitation: Exit Code Propagation

**`ops run` does not propagate guest exit codes to the host.** The `Start()` function in ops always returns nil regardless of what happens inside the VM. This means:

- `ops run mytest && echo "passed"` always says "passed"
- CI pipelines cannot use `ops run` exit codes for pass/fail

Under the hood, nanos writes exit codes to QEMU's `isa-debug-exit` device (port 0x501), which transforms them: `(code << 1) | 1`. The nanos Makefile reverses this with `|| exit $(($?>>1))`, but ops discards the code entirely.

**Workarounds:**
- Parse stdout for Go's `PASS`/`FAIL` output
- Use HTTP smoke tests (recommended)
- Run QEMU directly via `ops build` + manual QEMU invocation with `|| exit $(($?>>1))`

### No `ops test` Command

There is no built-in testing, health check, readiness probe, or CI/CD integration in the ops CLI. No GitHub Action exists for ops either.

---

## Recommendations for This Project

1. **Build with `CGO_ENABLED=0 GOOS=linux GOARCH=amd64`** (already doing this)
2. **Use `--arch=amd64` on Apple Silicon** for local testing (already fixed)
3. **Add `ntp` and `tls` klibs** to config.json for production
4. **Plan logging upfront** -- add `cloudwatch` + `tls` klibs for AWS
5. **Avoid any dependency that uses `os/exec`**
6. **Allocate at least 256-512MB memory** to avoid performance cliffs
7. **Build deployment automation** -- ops CLI alone won't give you zero-downtime
8. **Use the nightly nanos channel** for best Go compatibility
9. **Run integration tests against the unikernel** -- don't rely on host-side `go test` alone
10. **Don't depend on `ops run` exit codes in CI** -- use HTTP smoke tests or parse stdout
