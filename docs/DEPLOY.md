# Deploying to Google Cloud Run (GitHub Actions + Workload Identity Federation)

Everything provider-specific is a **variable or secret**; nothing is hardcoded. Commands below are a starting point: verify flags against the current `gcloud` documentation, they change.

## 0. Shape of the deployment
- One Cloud Run service per environment (`staging`, `production`), ideally in separate GCP projects with separate databases and secrets.
- **min = max = 1 instance, CPU always allocated** (`deploy/cloudrun.service.yaml`). Pollers, alert evaluation and WebSockets run outside requests and keep state in memory, so the service must not scale to zero or run several copies (until Redis/Pub-Sub fan-out exists). Expect an always-on instance cost.
- Request timeout 3600 s (WebSockets live up to 60 min; clients reconnect and catch up with `GET /me/feed`).
- Startup probe `/readyz` (database reachable, migrations applied at start), liveness probe `/healthz`.
- Database: managed Postgres via `DATABASE_URL` (see `docs/DATABASE.md`). The Supabase pooler works over IPv4.

## 1. One-time GCP setup (per project)
```bash
PROJECT=my-project; REGION=asia-south1; REPO=tradepulse
gcloud config set project $PROJECT
gcloud services enable run.googleapis.com artifactregistry.googleapis.com secretmanager.googleapis.com iamcredentials.googleapis.com
gcloud artifacts repositories create $REPO --repository-format=docker --location=$REGION

# Service accounts: one to deploy (used by GitHub), one for the running service
gcloud iam service-accounts create tradepulse-deployer
gcloud iam service-accounts create tradepulse-runtime
DEPLOYER=tradepulse-deployer@$PROJECT.iam.gserviceaccount.com
RUNTIME=tradepulse-runtime@$PROJECT.iam.gserviceaccount.com
gcloud projects add-iam-policy-binding $PROJECT --member=serviceAccount:$DEPLOYER --role=roles/run.admin
gcloud projects add-iam-policy-binding $PROJECT --member=serviceAccount:$DEPLOYER --role=roles/artifactregistry.writer
gcloud projects add-iam-policy-binding $PROJECT --member=serviceAccount:$DEPLOYER --role=roles/secretmanager.viewer
gcloud iam service-accounts add-iam-policy-binding $RUNTIME --member=serviceAccount:$DEPLOYER --role=roles/iam.serviceAccountUser
```
**Workload Identity Federation** (GitHub OIDC, no keys): create a pool and an OIDC provider for `https://token.actions.githubusercontent.com`, restrict it with an attribute condition to **your repository**, and allow it to impersonate the deployer:
```bash
gcloud iam workload-identity-pools create github --location=global
gcloud iam workload-identity-pools providers create-oidc github-actions --location=global --workload-identity-pool=github \
  --issuer-uri=https://token.actions.githubusercontent.com \
  --attribute-mapping="google.subject=assertion.sub,attribute.repository=assertion.repository" \
  --attribute-condition="assertion.repository=='YOUR_ORG/YOUR_REPO'"
POOL=$(gcloud iam workload-identity-pools describe github --location=global --format='value(name)')
gcloud iam service-accounts add-iam-policy-binding $DEPLOYER --role=roles/iam.workloadIdentityUser \
  --member="principalSet://iam.googleapis.com/$POOL/attribute.repository/YOUR_ORG/YOUR_REPO"
```
The provider resource name (`projects/NUMBER/locations/global/workloadIdentityPools/github/providers/github-actions`) goes into the GitHub variable below.

## 2. Secrets (Secret Manager) per environment
Naming convention `tradepulse-<environment>-<name>`:

| Secret name suffix | Env var | Required | Notes |
|---|---|---|---|
| `jwt-secret` | `JWT_SECRET` | yes | `openssl rand -base64 48` |
| `database-url` | `DATABASE_URL` | yes | pooler URL (Supabase transaction mode) |
| `database-migration-url` | `DATABASE_MIGRATION_URL` | recommended | direct/session URL for migrations |
| `webhook-signing-key` | `WEBHOOK_SIGNING_KEY` | for webhooks/channels | `openssl rand -base64 48` |
| `x-bearer-token` | `X_BEARER_TOKEN` | optional | enables X sources |
| `metrics-token` | `METRICS_TOKEN` | recommended | enables `GET /metrics` |
| `bootstrap-admin-email`, `bootstrap-admin-password` | `BOOTSTRAP_ADMIN_*` | first deploy only | delete the secrets after the admin exists |
```bash
printf '%s' "$(openssl rand -base64 48)" | gcloud secrets create tradepulse-staging-jwt-secret --data-file=-
gcloud secrets add-iam-policy-binding tradepulse-staging-jwt-secret --member=serviceAccount:$RUNTIME --role=roles/secretmanager.secretAccessor
```
Repeat for each secret (the runtime account needs `secretAccessor` on each). Optional secrets are wired into the service only if they exist when you deploy.

## 3. GitHub configuration
Create two **environments**, `staging` and `production` (add required reviewers to `production`). In each, set **variables**:

| Variable | Example |
|---|---|
| `GCP_PROJECT_ID` | `my-project` |
| `GCP_REGION` | `asia-south1` |
| `GCP_ARTIFACT_REPO` | `tradepulse` |
| `CLOUD_RUN_SERVICE` | `tradepulse-staging` |
| `GCP_WORKLOAD_IDENTITY_PROVIDER` | `projects/123/locations/global/workloadIdentityPools/github/providers/github-actions` |
| `GCP_DEPLOY_SERVICE_ACCOUNT` | `tradepulse-deployer@my-project.iam.gserviceaccount.com` |
| `GCP_RUNTIME_SERVICE_ACCOUNT` | `tradepulse-runtime@my-project.iam.gserviceaccount.com` |
| `ALLOWED_ORIGINS` | `https://app.example.com` (frontend origins; may be empty before the frontend exists) |
| `TRUSTED_PROXY_HOPS` | `1` (Cloud Run direct; `2` behind an external HTTPS load balancer) |

No secrets are stored in GitHub: authentication is OIDC.

## 4. Deploy
Actions -> **deploy** -> Run workflow -> choose the environment. The workflow builds and pushes the image, renders `deploy/cloudrun.service.yaml`, runs `gcloud run services replace`, allows public invocation (the API is public; access control is in the app) and smoke-tests `/readyz`.
First deploy: create the admin with the bootstrap secrets, log in, **then delete the bootstrap secrets** and redeploy.
Custom domain: map it in Cloud Run (managed TLS). `TRUST_PROXY=true` is already set in the template.

## 5. Verify and roll back
- `curl https://SERVICE_URL/readyz`; `/docs` must be **404** in production; `/metrics` 401 without the token.
- Logs: Cloud Logging (JSON, `request` lines carry status and duration; no tokens or credentials).
- **Rollback:** `gcloud run services update-traffic SERVICE --region REGION --to-revisions=PREVIOUS_REVISION=100`. Database migrations are forward-only; keep them backward compatible for one release.
- During a rollout two revisions briefly overlap: ingestion is idempotent (database-level dedupe) so this is safe; WebSocket clients reconnect.

## 6. Cost and limits to know
Always-on CPU instance + managed Postgres + X API credits (capped by `X_MONTHLY_BUDGET_USD`). Set a GCP budget alert. Single instance is the design for 10 to 50 members; see `docs/LOADTEST.md` for measured limits.
