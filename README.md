# Cloud Run IaC Controller (MVP)

This controller exposes a single endpoint used by CI pipelines to request short-lived GCP access tokens
(via Service Account impersonation).

## Endpoints

- `GET /healthz`
- `POST /v1/credentials` (Cloud Run IAM invoker required)

## Required env vars (set at deploy time)

- `GCP_PROJECT`
- `PLAN_SERVICE_ACCOUNT`  (email)
- `APPLY_SERVICE_ACCOUNT` (email)

## How to Deploy the Controller

Please read `DEPLOY.md` file.
