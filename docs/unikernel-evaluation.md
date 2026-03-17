# Unikernel Evaluation

Evaluation of unikernels as a deployment target for stateless Go HTTP services on AWS, as an alternative to containers or Kubernetes.

## Motivation

We want pod-like scaling speed and resource efficiency without Kubernetes overhead or the AWS configuration complexity of ECS/Fargate. The target architecture is:

- ALB (external APIs) or NLB (internal APIs) in the DMZ
- Auto Scaling Group of unikernel instances behind the load balancer
- Graceful shutdown enabling clean scale-in
- Pre-emptive scaling to workload hours

This gives us fast, simple, horizontally scalable compute with no orchestrator. The ASG + launch template model means deploys are just AMI swaps.

## AMI Backing: EBS vs Instance Store

**Instance-store backed AMIs are effectively end-of-life.** They only work on legacy instance types (C1, C3, D2, I2, M1, M2, M3, R3, X1) and require deprecated tooling (`ec2-bundle-vol`, `ec2-upload-bundle`).

Note: this is distinct from NVMe instance store *volumes*, which are alive and well on modern `d`-suffix instance types (c5d, m5d, r6id, etc.). Those are additional block devices, not the root volume.

All unikernel AMIs must be **EBS-backed**. For images this small (~7.6 MB), EBS overhead is negligible.

## Unikernel Landscape

### Nanos (NanoVMs)

- **Syscall coverage**: ~200 of 300+ Linux syscalls. No published manifest to audit against; must grep kernel source or test empirically
- **AWS tooling**: Best in class. `ops image create` handles local build, EBS Direct upload, and AMI registration in one command
- **Maturity**: Commercial backing (NanoVMs, Inc.), ~3k GitHub stars, used by U.S. Air Force
- **Limitations**: No fork/exec by design, limited debugging (no SSH, GDB only local), lwIP networking stack (not Linux TCP/IP)
- **Go compatibility**: Good with `CGO_ENABLED=0`. `net/http`, TLS, DNS all work. See [nanos.md](../nanos.md) for details

### Unikraft (Linux Foundation)

- **Syscall coverage**: 160+ syscalls, actively growing. Supports **binary compatibility mode** — run unmodified Linux ELFs via elfloader, targeting Linux ABI compliance
- **AWS tooling**: Non-existent. AMI packaging was proposed ([kraftkit#1468](https://github.com/unikraft/kraftkit/issues/1468)) but closed Feb 2026 with "this will only make sense in the distant future." The older `plat-aws` path uses Xen images with legacy EC2 tools
- **Maturity**: Linux Foundation project, VC-backed (Vercel's fund). Active development, large academic community. `kraft` CLI supports OCI packaging and their own Unikraft Cloud platform
- **Go compatibility**: Works via binary compat mode. Requires `-buildmode=pie -static-pie` linking with `CGO_ENABLED=1` and libc in the rootfs. Different build flags than Nanos

### UKL (Unikernel Linux)

- **Syscall coverage**: 100% — it IS the Linux kernel with the application linked into kernel space
- **AWS tooling**: None. No CLI, no image builder, no ecosystem
- **Maturity**: Research project (Red Hat Research + Boston University). Kernel patch, not mainlined. Not production-ready
- **Verdict**: Interesting academically, not usable

### Terraform Provider for Ops

The [nanovms/terraform-provider-ops](https://github.com/nanovms/terraform-provider-ops) exists but is abandoned — v0.0.4 from June 2021, 18 commits total. Do not use.

### Summary

| | Syscall Coverage | AWS Tooling | Production Maturity |
|---|---|---|---|
| **Nanos** | ~200, undocumented | Best (`ops` CLI) | Decent |
| **Unikraft** | 160+, binary compat | None (AMI deferred) | Growing fast |
| **UKL** | 100% (is Linux) | None | Research only |

Nanos has the pipeline, Unikraft has the kernel. Nobody has both.

## Deployment Architecture

### AMI-Based Deploys via ASG Instance Refresh

ASG [Instance Refresh](https://docs.aws.amazon.com/autoscaling/ec2/userguide/asg-instance-refresh.html) provides rolling deployments for AMI updates:

- Update the launch template to point to a new AMI
- Trigger an instance refresh with configurable checkpoint percentages
- ASG replaces instances in batches, respecting health checks
- Auto-rollback if ALB health checks fail on new instances

Example Terraform config:

```hcl
data "aws_ami" "unikernel" {
  most_recent = true
  owners      = ["self"]
  filter {
    name   = "name"
    values = ["unikernel-demo-*"]
  }
}

resource "aws_launch_template" "app" {
  name_prefix   = "unikernel-demo-"
  image_id      = data.aws_ami.unikernel.id
  instance_type = "t3.micro"
}

resource "aws_autoscaling_group" "app" {
  launch_template {
    id      = aws_launch_template.app.id
    version = aws_launch_template.app.latest_version
  }

  instance_refresh {
    strategy = "Rolling"
    preferences {
      min_healthy_percentage = 90
      instance_warmup        = 30
      checkpoint_percentages = [10, 100]
      checkpoint_delay       = 60
    }
  }
}
```

The `data.aws_ami` lookup finds the latest AMI by name pattern on every `terraform plan`. When the AMI changes, Terraform updates the launch template, which triggers the instance refresh.

### CI/CD Pipeline

Recommended split: **app repo builds the AMI, infra repo manages the infrastructure.**

**App repo (GitHub Actions, on release tag):**
1. Build Go binary (`script/build`)
2. Create AMI via `ops image create` (named with version, e.g., `unikernel-demo-v1.2.3`)
3. Trigger Terraform Cloud run via API

**Infra repo (Terraform Cloud):**
- Manages VPC, subnets, security groups, ALB/NLB, launch template, ASG
- `data.aws_ami` picks up the latest AMI on plan
- Dev/Stage: auto-apply
- Prod: plan only, manual confirm in TFC UI

**Triggering Terraform Cloud from GHA:**

There is no official HashiCorp GitHub Action for triggering TFC runs. Options:
- TFC API (`curl` to `/api/v2/runs`) — simplest, most transparent
- `hashicorp/setup-terraform` action + `terraform apply` with cloud block — delegates run to TFC
- Third-party actions exist but are unmaintained

The API call is ~5 lines and doesn't depend on third-party actions:

```yaml
- name: Trigger Terraform Cloud
  run: |
    curl -s --request POST \
      --url "https://app.terraform.io/api/v2/runs" \
      --header "Authorization: Bearer ${{ secrets.TFC_API_TOKEN }}" \
      --header "Content-Type: application/vnd.api+json" \
      --data '{
        "data": {
          "type": "runs",
          "attributes": {
            "message": "New AMI: unikernel-demo-${{ github.ref_name }}",
            "auto-apply": false
          },
          "relationships": {
            "workspace": {
              "data": { "type": "workspaces", "id": "ws-XXXXX" }
            }
          }
        }
      }'
```

Set `auto-apply` per environment workspace.

### Syscall Risk Mitigation

The main risk with unikernels is a missing syscall crashing the service at runtime due to a third-party library invoking something unexpected.

Mitigations:
- **strace in CI**: Run the binary under `strace` on a Linux GHA runner, collect the syscall set, compare against known-good baseline
- **Unikernel e2e tests**: Boot the image and run the full HTTP test suite as a CI gate (already implemented in `script/test-unikernel`)
- **Checkpoint-based instance refresh**: Roll out to 10% of instances first, pause, validate metrics, then continue
- **Dependency auditing**: Check for `os/exec` usage in transitive dependencies

The strace approach is more reliable than trying to diff against Nanos' syscall list, since no clean versioned manifest exists.

## Decision

**Not pursuing unikernels for production use at this time.**

The technology is promising but the ecosystem is too immature:

- **Nanos** has working AWS tooling but unknown syscall coverage creates unacceptable risk for production services with third-party dependencies. Debugging and observability story is weak.
- **Unikraft** has better syscall coverage and binary compatibility but no AMI pipeline, and that's explicitly deferred to "the distant future."
- **No unikernel project** offers the combination of Linux-grade syscall compatibility AND production-ready cloud tooling.

The architecture research (ALB/NLB -> ASG, instance refresh deploys, TFC integration) remains valid if/when the unikernel tooling matures. The same patterns work equally well with minimal VM images (e.g., Firecracker + Alpine) or even lightweight AMIs built with Packer.

### What Would Change This

- Unikraft shipping AMI support in `kraft`
- Nanos publishing a versioned syscall manifest and reaching >250 syscalls
- A unikernel project achieving full Linux ABI compatibility with production tooling
- Our workloads being simple enough (pure stdlib, no third-party deps) to confidently stay within Nanos' syscall surface
