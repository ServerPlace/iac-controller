# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Commands

```bash
# Build
make build-local          # compile binary to bin/iac-controller
go build ./...            # quick syntax/type check

# Test
go test -race ./...       # run all tests
go test -race ./internal/scm/...  # run tests in a specific package

# Format + test (pre-commit)
make go-check             # go mod tidy + go fmt + go test -race

# Full local build
make all                  # go-check + build-local

# Docker
make docker-build         # build linux/amd64 image
make docker-push          # auth + build + push to GCP Artifact Registry

# Infrastructure
make tf-init              # terraform init
make tf-apply             # terraform apply (infra only)
make deploy               # docker-push + tf-apply (full deploy)
```

## Architecture

The controller is a GCP Cloud Run HTTP service that manages PR-based Terraform deployment workflows. It issues short-lived GCP credentials (via Service Account impersonation), enforces compliance rules before apply, and bridges SCM webhooks with Azure DevOps pipeline triggers.

### Startup flow

`cmd/controller/main.go` → `internal/config.Load()` → `internal/server.NewServer()` → `srv.ListenAndServe()`

`NewServer` wires all dependencies in `initDependencies()` then creates controllers in `initControllers()`. Routes are registered lazily in `setupRoutes()` (called from `ListenAndServe`). The `Server` struct (Atlantis pattern) holds every dependency as a field — no global state.

### Configuration

Config is loaded from a YAML file (default `/app/config/config.yaml`, override with `CONFIG_PATH`). Environment variables override YAML keys. Strings prefixed with `_secret://` are resolved at startup from GCP Secret Manager via `internal/secrets/resolver.go`.

`SCM_PROVIDER` must be `azure` or `github` — it controls which SCM client is instantiated. `LOCAL_DEV=true` skips OIDC auth on `/api/v1/*` routes (HMAC signing is still exercised).

### Request authentication layers

| Endpoint group | Auth |
|---|---|
| `GET /healthz` | none |
| `POST /v1/credentials` | OIDC (`allowed_invokers`) |
| `POST /admin/repositories` | OIDC (`allowed_admins`) |
| `POST /webhook/azure` | Basic Auth |
| `POST /api/v1/plans`, `/approve`, `/plans/close` | OIDC + HMAC per-repo |
| `POST /internal/async/run` | OIDC (Cloud Tasks SA) |

HMAC middleware (`internal/middleware/hmac_auth.go`) resolves the repo key from Firestore, signs request bodies with `JIT_SECRET_KEY`, and injects the `RepositoryMetadata` into context. OIDC middleware (`internal/middleware/auth.go`) validates Google OIDC tokens against `expected_audiences` and `allowed_azps`.

### Core domain: apply workflow

There are **two apply trigger models** — one active today, one planned:

**Current flow (user-initiated):** The user manually triggers the apply pipeline in Azure DevOps. The pipeline runs iac-runner, which calls `POST /v1/credentials` with `mode=apply`. Because no job was pre-created by the controller, `job_id` and `job_token` arrive empty, hitting the early-return path in `handleApply` (`credentials.go`) that issues the token directly after SHA and branch checks.

**Future flow (controller-initiated):** `POST /webhook/azure` → `internal/webhook/Handler.ServeHTTP` → detects `/apply` comment → `service.DeploymentService.RunApply()`:
1. Fetches PR via SCM client
2. Saves `Deployment` record (status: `VALIDATING`) to Firestore for audit
3. Evaluates compliance rules (`internal/compliance.Engine`)
4. On pass: saves `Job`, triggers ADO pipeline with `IV_JOB_ID` + `IV_JIT_SECRET` variables
5. Updates PR comment with result (compliance table + pipeline URL)

In this future flow, the pipeline receives `IV_JOB_ID` and `IV_JIT_SECRET` as Azure DevOps pipeline variables, and iac-runner passes them back in the credentials request. The controller then validates the JIT token against the pre-created job — closing the chain of custody. **Do not remove `DeploymentService.RunApply()`, the `PipelineOrchestrator` port, `internal/pipeline/azure`, or the Job/JIT token validation in `handleApply` — they are intentionally kept for this planned flow.**

The `POST /api/v1/approve` endpoint also belongs to this future flow: it lets the controller create a job+token pair on demand (before the pipeline runs), replacing the need for a webhook trigger.

### Ports and adapters

`internal/core/ports/interfaces.go` defines two interfaces:
- `Persistence` — implemented by `internal/storage/firestore.Adapter`
- `PipelineOrchestrator` — implemented by `internal/pipeline/azure.Adapter`

`internal/scm.Client` is the SCM interface; Azure and GitHub implementations live in `internal/scm/azure/` and `internal/scm/github/`. The mock (`internal/scm/mock_client.go`) is generated with `go:generate mockgen`.

### Async engine (Cloud Tasks)

`internal/async.Engine` provides coalesced, at-least-once task execution backed by Firestore leases and GCP Cloud Tasks. `Engine.Kick()` deduplicates — only one task per `(kind, key)` is queued. `Engine.RunOnce()` acquires a Firestore lease, dispatches to a registered handler (`async.Registry`), and handles `Done/Wait/Retry/Fail` outcomes.

Currently registered task kind: `merge_pr` (handler in `internal/server/controllers/tasks/`).

### Compliance rules (webhook-triggered apply)

Rules are enabled via `compliance.rules[]` in config. Each rule implements `compliance.Rule` and is registered in `internal/compliance/registry.go`. The built-in `approval` rule is in `internal/compliance/rules/approval.go`. Results with `SeverityStop` block the apply.

### Apply gates (user-initiated apply credential delivery)

`internal/compliance/apply_gate.go` — `ApplyGate` interface, `ApplyContext`, `ApplyEngine`.  
`internal/compliance/apply_gates.go` — all gate implementations + `newApplyGate` switch.

Gates run in `handleApply` before issuing the GCP token. They receive pre-fetched data via `ApplyContext` (no network calls inside gates — keeps them fast and testable).

| Gate ID | Default | Blocks when |
|---|---|---|
| `sha_stale` | **on** | pipeline SHA ≠ live PR HEAD |
| `branch_up_to_date` | **on** | source branch is behind target — guaranteed controller-side regardless of ADO policy config |
| `sha_backend_match` | **on** | pipeline SHA ≠ SHA registered at plan time |
| `branch_policies_passing` | **on** | delegates to ADO Policy Evaluations API — covers approvals, work items, comment resolution, and any other blocking policy; degrades to SeverityInfo (skip) on GitHub |

Configure via `apply_gates[]` in `config.yaml`. Absent → defaults apply (sha_stale + branch_up_to_date only). To add a gate, add a case in `newApplyGate` and a struct in `apply_gates.go` — no global registry, no blank imports.

On gate failure: posts a Markdown table comment to the PR (key `"apply-gate"`) and returns 403.  
Branch status fetch is **fail-closed**: SCM error → 403 before the engine runs.  
Deployment lookup is **best-effort**: not found → `sha_backend_match` degrades to SeverityInfo.

### Infrastructure (Terraform)

- `terraform/` — deploys the Cloud Run service, Firestore, Cloud Tasks queue, Artifact Registry, and IAM bindings
- `iac-permissions/terraform/` — grants the controller's service account the IAM permissions it needs on target projects
- Image tag strategy: `v1-<7-char-sha256-of-source-files>`

### Logging

zerolog is used throughout. The root logger is created in `main.go` with `severity` (GCP-compatible) and `time` field names. Per-request loggers are stored in `context.Context` via `pkg/log` helpers (`log.WithLogger`, `log.FromContext`). Background goroutines must propagate the logger explicitly into the background context.
