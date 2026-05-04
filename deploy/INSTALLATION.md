# Carbide REST Installation Guide

## Overview

This is a **prescriptive, BYO-Kubernetes bring-up guide** for the Carbide REST cloud components. It encodes the **order of operations**, the **exact manifest paths** from this repository, and what you must configure for your environment.

> **Experimental:** This software is a preview release. Features, APIs, and configurations may change without notice. Thoroughly test in non-critical environments before production use.

### Deployment topology

Carbide REST can be deployed in two ways:

- **Co-located:** The REST layer and [NVIDIA Infra Controller Core](https://github.com/NVIDIA/ncx-infra-controller-core) run together in the same datacenter cluster.
- **Cloud-hosted:** The REST layer runs anywhere (cloud, remote DC) and Site Agents running at each datacenter connect back to it. Multiple NVIDIA Infra Controller Core instances in different datacenters can each connect through their own Site Agent.

This guide covers the cloud-hosted topology — deploying the REST control plane components on a Kubernetes cluster that site agents will connect to from remote sites.

All manifests live under `deploy/kustomize/` with the following structure:

```
deploy/kustomize/
├── base/                 # Reusable base manifests (not applied directly)
│   ├── api/              # carbide-rest-api
│   ├── cert-manager/     # carbide-rest-cert-manager (internal PKI service)
│   ├── cert-manager-io/  # cert-manager.io ClusterIssuer
│   ├── common/           # Shared secrets and certs (carbide-rest namespace)
│   ├── db/               # Database migration job
│   ├── keycloak/         # Keycloak identity provider
│   ├── mock-core/        # carbide-rest-mock-core (dev/test only)
│   ├── postgres/         # PostgreSQL database
│   ├── site-agent/       # carbide-rest-site-agent
│   ├── site-manager/     # carbide-rest-site-manager + Site CRD
│   ├── temporal-helm/    # Temporal TLS certs and namespace resources
│   └── workflow/         # carbide-rest-cloud-worker + carbide-rest-site-worker
└── overlays/             # Environment-specific overlays (sets image registry/tag)
    ├── api/
    ├── cert-manager/
    ├── db/
    ├── mock-core/
    ├── site-agent/
    ├── site-manager/
    └── workflow/
```

**Namespaces used:**

| Namespace | Contents |
|---|---|
| `carbide-rest` | All Carbide REST workloads |
| `postgres` | PostgreSQL |
| `temporal` | Temporal workflow engine |

---

## Prerequisites

- Kubernetes cluster (v1.27+)
- `kubectl` configured with cluster-admin access
- `helm` (v3) — for the vendored Temporal chart at `temporal-helm/temporal/`
- [cert-manager](https://cert-manager.io/docs/installation/) installed in the cluster (v1.13+)
- Container images built and pushed to a registry accessible from the cluster — see [Building and Pushing Images](#building-and-pushing-images)

---

## Order of Operations

```
1.  Create namespaces
2.  Create CA signing secret            ← prerequisite for carbide-rest-cert-manager
3.  Deploy PostgreSQL
4.  Deploy Keycloak                     ← depends on PostgreSQL
5.  Deploy carbide-rest-cert-manager    ← depends on CA signing secret
6.  Apply cert-manager.io ClusterIssuer ← depends on carbide-rest-cert-manager
7.  Apply common secrets & certs        ← depends on ClusterIssuer
8.  Deploy Temporal                     ← depends on Temporal TLS certs
9.  Run DB migrations                   ← depends on PostgreSQL
10. Deploy carbide-rest-site-manager    ← depends on ClusterIssuer, Site CRD
11. Deploy carbide-rest-api             ← depends on all of the above
12. Deploy carbide-rest-workflow        ← depends on all of the above
13. Deploy carbide-rest-site-agent      ← depends on all of the above
```

---

## Step 1 — Create Namespaces

```bash
kubectl create namespace carbide-rest
kubectl apply -f deploy/kustomize/base/postgres/namespace.yaml
kubectl apply -f deploy/kustomize/base/temporal-helm/namespace.yaml
```

**Files:**
- `deploy/kustomize/base/postgres/namespace.yaml` — creates `postgres` namespace
- `deploy/kustomize/base/temporal-helm/namespace.yaml` — creates `temporal` namespace

---

## Step 2 — Create the CA Signing Secret

### What it is

Before we begin with the installation, we need a root CA (certificate + private key) provided as a Kubernetes Secret named `ca-signing-secret` in the `carbide-rest` namespace.

The cert-manager.io `ClusterIssuer` references this secret to issue certificates for all other components. It is also used by `carbide-rest-cert-manager`, which is the internal PKI service for Carbide REST working in conjunction with cert-manager.io to dynamically dispense mTLS certificate for all connecting Site Agents.

The CA certificate is the trust anchor for the entire deployment. Every TLS certificate issued to Carbide REST workloads — `site-manager` HTTPS cert, `site-agent` gRPC/Temporal client certs — traces back to this CA.

### Required secret shape

```
Secret name: ca-signing-secret  (type: kubernetes.io/tls)
Namespaces:  `carbide-rest`  and  `cert-manager`
Keys:
  tls.crt  →  PEM-encoded root CA certificate
  tls.key  →  PEM-encoded root CA private key
```

The secret must exist in both `carbide-rest` (for `carbide-rest-cert-manager`) and `cert-manager` (for the cert-manager.io `ClusterIssuer`).

### Option A — Use the helper script (recommended)

A `gen-site-ca.sh` script is provided at `scripts/gen-site-ca.sh`. It generates a self-signed RSA 4096 root CA and creates `ca-signing-secret` in both namespaces in one step:

```bash
# Apply directly to the cluster
./scripts/gen-site-ca.sh

# Apply to a non-default namespace
./scripts/gen-site-ca.sh --namespace my-carbide-ns

# Write cert files to disk without running kubectl (apply manually later)
./scripts/gen-site-ca.sh --output-dir /tmp/carbide-ca

# See all options
./scripts/gen-site-ca.sh --help
```

The script creates a CA with proper `v3_ca` extensions (basicConstraints, keyUsage) and applies it to both namespaces using `kubectl create secret tls --dry-run=client | kubectl apply`.

### Option B — Bring your own CA

If you have an existing PKI (HSM, enterprise CA, etc.), create the secret directly from your PEM files:

```bash
kubectl create secret tls ca-signing-secret \
  --cert=/path/to/ca.crt \
  --key=/path/to/ca.key \
  -n carbide-rest --dry-run=client -o yaml | kubectl apply -f -

kubectl create secret tls ca-signing-secret \
  --cert=/path/to/ca.crt \
  --key=/path/to/ca.key \
  -n cert-manager --dry-run=client -o yaml | kubectl apply -f -
```

---

## Step 3 — Deploy PostgreSQL

### What it is

A single-replica PostgreSQL 14 StatefulSet that hosts all databases for the Carbide REST stack. This is provided as a **reference deployment** — if you already operate a PostgreSQL instance, skip this step entirely and go straight to Step 9 (DB migrations). You will need to manually create the databases and users listed below on your existing instance before running migrations.

### Manifests

| File | Contents |
|---|---|
| `base/postgres/namespace.yaml` | `postgres` namespace |
| `base/postgres/admin-creds.yaml` | Secret `admin-creds` — postgres superuser password |
| `base/postgres/init-configmap.yaml` | ConfigMap `postgres-init` — SQL init script |
| `base/postgres/statefulset.yaml` | StatefulSet `postgres` — `postgres:14.4-alpine`, 1Gi PVC |
| `base/postgres/service.yaml` | ClusterIP Service on port 5432 — DNS: `postgres.postgres` |
| `base/postgres/adminer.yaml` | Optional Adminer web UI |

### Databases created at init time

| Database | User | Used by |
|---|---|---|
| `forge` | `forge` | carbide-rest-api, workflow workers |
| `keycloak` | `keycloak` | Keycloak |
| `temporal` | `temporal` | Temporal |
| `temporal_visibility` | `temporal` | Temporal |

### Credentials to change for production

- `base/postgres/admin-creds.yaml` — change `password: "postgres"` to a strong password
- Per-database passwords are embedded in `init-configmap.yaml` and must be kept in sync with `base/common/db-creds.yaml` and `base/temporal-helm/db-creds.yaml`

### Apply

```bash
kubectl apply -k deploy/kustomize/base/postgres
kubectl rollout status statefulset/postgres -n postgres
```

---

## Step 4 — Deploy Keycloak

### What it is

Keycloak is the **reference OIDC identity provider** for the Carbide REST API. It handles authentication and issues JWTs that the API validates on every request. It is pre-loaded with the `carbide-dev` realm via an imported realm ConfigMap, which includes the `carbide-api` client, realm roles, and a set of pre-seeded dev users.

Users of Carbide can also bring their own OpenID/OAuth JWT Provider, see [Auth docs](https://github.com/NVIDIA/infra-controller-rest/tree/main/auth) for more details.

### Manifests

| File | Contents |
|---|---|
| `base/keycloak/deployment.yaml` | Deployment `keycloak` — `quay.io/keycloak/keycloak:24.0` |
| `base/keycloak/realm-configmap.yaml` | ConfigMap `keycloak-realm` — `carbide-dev` realm JSON |
| `base/keycloak/service.yaml` | ClusterIP Service on port 8082 — DNS: `keycloak.carbide-rest` |

### Pre-configured realm

The `carbide-dev` realm includes:

- **Client:** `carbide-api` with client secret `carbide-local-secret`
- **Realm roles:** `admin`, `user`, `test-org:FORGE_PROVIDER_ADMIN`, `test-org:FORGE_TENANT_ADMIN`, `test-org:FORGE_PROVIDER_VIEWER`
- **Pre-seeded dev users:**

  | Username | Password | Roles |
  |---|---|---|
  | `testuser` | `testpassword` | `user`, `test-org:FORGE_TENANT_ADMIN` |
  | `admin` | `adminpassword` | `admin`, `user`, `test-org:FORGE_PROVIDER_ADMIN`, `test-org:FORGE_TENANT_ADMIN` |
  | `provider` | `providerpassword` | `user`, `test-org:FORGE_PROVIDER_ADMIN` |

### Configuration to change for production

- Replace `start-dev` in `deployment.yaml` args with `start` and configure proper TLS
- Remove or change the pre-seeded user passwords
- Update the `keycloak-client-secret` in `base/common/keycloak-client-secret.yaml` — must match the `secret` field in the realm JSON

### Apply

```bash
kubectl apply -k deploy/kustomize/base/keycloak -n carbide-rest
```

---

## Step 5 — Deploy `carbide-rest-cert-manager`

### What it is

`carbide-rest-cert-manager` is the internal PKI microservice (also referred as `credsmgr`). It uses native Go PKI to vend mTLS certificates for components over HTTPS, primarily for dynamic/external entities e.g. Site Agents. When the `site-manager` receives a new site registration, it calls `carbide-rest-cert-manager` service to issue the client certificates `site-agent` will use to authenticate. It exposes two ports:

- **8000** (HTTPS) — certificate issuance API
- **8001** (HTTP) — health and liveness endpoint

### Manifests

| File | Contents |
|---|---|
| `base/cert-manager/deployment.yaml` | Deployment `carbide-rest-cert-manager` — mounts `ca-signing-secret` |
| `base/cert-manager/service.yaml` | ClusterIP Service — ports 8000 (https) and 8001 (http) |
| `base/cert-manager/rbac.yaml` | ServiceAccount + Role/RoleBinding — needs read/write access to Secrets and ConfigMaps |

### CLI flags (set in `deployment.yaml`)

| Flag | Default | Description |
|---|---|---|
| `--ca-cert-file` | `/etc/pki/ca/tls.crt` | Path to CA cert (from `ca-signing-secret`) |
| `--ca-key-file` | `/etc/pki/ca/tls.key` | Path to CA key (from `ca-signing-secret`) |
| `--ca-common-name` | `Carbide Local Dev CA` | CN stamped on the CA |
| `--ca-organization` | `NVIDIA` | Organization stamped on the CA |
| `--tls-port` | `8000` | HTTPS listen port |
| `--insecure-port` | `8001` | HTTP health port |
| `--ca-base-dns` | `carbide.local` | DNS suffix used in issued certs |

### Apply

Using the overlay (sets image registry and tag):

```bash
# Edit deploy/kustomize/overlays/cert-manager/kustomization.yaml to set your image
kubectl kustomize --load-restrictor LoadRestrictionsNone \
  deploy/kustomize/overlays/cert-manager | kubectl apply -f -
```

```bash
kubectl rollout status deployment/carbide-rest-cert-manager -n carbide-rest
```

---

## Step 6 — Apply cert-manager.io ClusterIssuer

### What it is

A cert-manager.io `ClusterIssuer` named `carbide-rest-ca-issuer` that uses `ca-signing-secret` to sign certificates cluster-wide. All `Certificate` resources created by subsequent steps reference this issuer — Temporal TLS certs, site-manager TLS, site-agent gRPC certs, and Temporal client certs all flow through it. The ClusterIssuer is used for generating mTLS certs for static/well known in cluster services.

### Manifests

| File | Contents |
|---|---|
| `base/cert-manager-io/cluster-issuer.yaml` | `ClusterIssuer` `carbide-rest-ca-issuer` — references `ca-signing-secret` |

> **Note:** cert-manager.io reads the CA secret for a `ClusterIssuer` from the `cert-manager` controller namespace. The helper script in Step 2 (`gen-site-ca.sh`) creates `ca-signing-secret` in both `carbide-rest` and `cert-manager` automatically. If you created the secret manually, ensure it exists in both namespaces before applying this step.

### Apply

```bash
kubectl apply -k deploy/kustomize/base/cert-manager-io
kubectl get clusterissuer carbide-rest-ca-issuer
# READY column should show True
```

---

## Step 7 — Apply Common Secrets and Certificates

### What it is

The `common/` base provides all shared secrets and cert-manager `Certificate` resources consumed by `carbide-rest-api` and the workflow workers in the `carbide-rest` namespace. These must exist before those workloads are deployed.

### Manifests

| File | Secret name | Contents |
|---|---|---|
| `base/common/db-creds.yaml` | `db-creds` | `password: forge` — DB password for the `forge` user |
| `base/common/keycloak-client-secret.yaml` | `keycloak-client-secret` | `keycloak-client-secret: carbide-local-secret` — Keycloak OIDC client secret |
| `base/common/temporal-encryption-key.yaml` | `temporal-encryption-key` | `temporal-encryption-key: local-dev` — Temporal payload encryption key |
| `base/common/image-pull-secret.yaml` | `image-pull-secret` | Docker registry credentials — placeholder for public/open images, replace for private registries |
| `base/common/temporal-client-cloud-cert.yaml` | `temporal-client-cloud-certs` | cert-manager `Certificate` — TLS client cert for Temporal, used by API and workflow workers |

### `temporal-client-cloud-cert` Certificate

This cert-manager `Certificate` is issued by `carbide-rest-ca-issuer` and stored in the secret `temporal-client-cloud-certs`. It covers the following DNS names, allowing the API and both workers to authenticate to Temporal as the same logical client identity:

```
temporal-client, carbide-rest-api, cloud-worker, site-worker
```

Duration: 90 days, auto-renewed 15 days before expiry.

### Values to change for production

| Secret | Key | Change to |
|---|---|---|
| `db-creds` | `password` | Real `forge` DB password |
| `keycloak-client-secret` | `keycloak-client-secret` | Real Keycloak client secret (must match the value in the realm JSON) |
| `temporal-encryption-key` | `temporal-encryption-key` | A randomly generated 32+ byte key — **must be the same across API and all workers** |
| `image-pull-secret` | `.dockerconfigjson` | Base64-encoded Docker config for your container registry |

### Apply

```bash
kubectl apply -k deploy/kustomize/base/common
```

---

## Step 8 — Deploy Temporal

### What it is

Temporal is the durable workflow engine that coordinates all async and long-running operations in Carbide REST. The `cloud-worker` and `site-worker` services connect to it to poll and execute workflow tasks. `carbide-rest-api` schedules temporal workflows for `cloud-worker` and `site-agent` to execute. Temporal itself is deployed via the Helm chart vendored at `temporal-helm/temporal/`.

### Versions used

| Component | Image | Version |
|---|---|---|
| Temporal server | `temporalio/server` | `1.26.2` |
| Admin tools | `temporalio/admin-tools` | `1.26.2` |
| Temporal UI | `temporalio/ui` | `2.26.2` |

Helm chart version: `0.35.0` (appVersion `1.22.6` in Chart.yaml — the image tags in `values-kind.yaml` override this to `1.26.2`)

### Prerequisites in the cluster before installing Temporal

The following resources must exist in the `temporal` namespace before the Helm chart is installed, because the chart mounts them as volumes:

```bash
# Apply Temporal namespace, db-creds Secret, and TLS Certificate resources
kubectl apply -k deploy/kustomize/base/temporal-helm
```

Wait for cert-manager to issue all three certificate secrets:

```bash
kubectl get secret server-interservice-certs server-cloud-certs server-site-certs -n temporal
```

### TLS certificates applied by `base/temporal-helm/certificates.yaml`

Three `Certificate` resources are created in the `temporal` namespace by cert-manager, all issued by `carbide-rest-ca-issuer`:

| Certificate | Secret | Purpose |
|---|---|---|
| `server-interservice-cert` | `server-interservice-certs` | mTLS for Temporal internode communication (frontend ↔ history ↔ matching ↔ worker) |
| `server-cloud-cert` | `server-cloud-certs` | TLS endpoint for `cloud` namespace clients — DNS: `cloud.temporal-frontend.*` |
| `server-site-cert` | `server-site-certs` | TLS endpoint for `site` namespace clients — DNS: `site.temporal-frontend.*` |

These secrets are mounted into the Temporal server pods by the Helm values.

### Helm install

The Helm chart and our values files are vendored in the repository:

```bash
helm install temporal temporal-helm/temporal \
  --namespace temporal \
  --values temporal-helm/temporal/values-kind.yaml
```

`values-kind.yaml` is the reference values file for a local/kind cluster. It configures:

- PostgreSQL persistence (`postgres.postgres.svc.cluster.local`, databases `temporal` and `temporal_visibility`)
- mTLS on all internode and frontend communication using the cert secrets above
- Frontend host overrides for `cloud.server.temporal.local` and `site.server.temporal.local` so that the cloud and site client namespaces use their dedicated server certificates
- Schema setup and update jobs enabled (`schema.setup.enabled: true`, `schema.update.enabled: true`)

For a production deployment, copy `values-kind.yaml` and adjust resource limits, replica counts, and any environment-specific settings.

### Create Temporal namespaces

After Temporal is running, create the `cloud` and `site` namespaces that the workflow workers register to:

The `temporal-admintools` pod has the TLS environment variables pre-configured via the Helm values, so no TLS flags are needed on the CLI commands themselves. You do need to pass `--address` since the pod's default is `localhost:7233`:

```bash
kubectl exec -it -n temporal deployment/temporal-admintools -- \
  temporal operator namespace create cloud \
  --address temporal-frontend.temporal:7233

kubectl exec -it -n temporal deployment/temporal-admintools -- \
  temporal operator namespace create site \
  --address temporal-frontend.temporal:7233
```

---

## Step 9 — Run Database Migrations

### What it is

A Kubernetes `Job` that runs the Carbide REST database schema migrations against the `forge` PostgreSQL database. It uses an init container to wait for PostgreSQL to be ready before running, and will retry up to 30 times to handle cases where PostgreSQL is still starting.

### Manifests

| File | Contents |
|---|---|
| `base/db/job.yaml` | Job `carbide-rest-db-migration` — init container waits for `postgres.postgres:5432`, then runs migrations using the `carbide-rest-db` image |

### Configuration

| Env var | Value | Source |
|---|---|---|
| `PGHOST` | `postgres.postgres` | Manifest |
| `PGPORT` | `5432` | Manifest |
| `PGDATABASE` | `forge` | Manifest |
| `PGUSER` | `forge` | Manifest |
| `PGPASSWORD` | From Secret | `db-creds` → `password` |

### Apply

```bash
# Edit deploy/kustomize/overlays/db/kustomization.yaml to set your image
kubectl kustomize --load-restrictor LoadRestrictionsNone \
  deploy/kustomize/overlays/db | kubectl apply -f -

kubectl wait --for=condition=complete job/carbide-rest-db-migration -n carbide-rest --timeout=120s
```

---

## Step 10 — Deploy `carbide-rest-site-manager`

### What it is

`carbide-rest-site-manager` manages the full lifecycle of remote sites. It is the control-plane component that:

- Exposes an HTTPS API on port **8100** that site agents call during bootstrap to obtain their Temporal client certificates and registration credentials.
- Creates and manages `Site` custom resources in the `carbide-rest` namespace, one per registered site.
- Calls `carbide-rest-cert-manager` to issue certificates for newly registering sites.
- Tracks each site's bootstrap state (`AwaitHandshake` → `HandshakeComplete` → `RegistrationComplete`).

### Manifests

| File | Contents |
|---|---|
| `base/site-manager/site-crd.yaml` | CRD `sites.forge.nvidia.io` — the `Site` custom resource |
| `base/site-manager/deployment.yaml` | Deployment `carbide-rest-site-manager` |
| `base/site-manager/certificate.yaml` | cert-manager `Certificate` `site-manager-tls` — TLS cert for the HTTPS server |
| `base/site-manager/rbac.yaml` | ServiceAccount + Role/RoleBinding + ClusterRole/ClusterRoleBinding |
| `base/site-manager/service.yaml` | ClusterIP Service on port 8100 — DNS: `carbide-rest-site-manager.carbide-rest` |

### Site CRD (`sites.forge.nvidia.io`)

```yaml
spec:
  uuid:       # Unique site identifier (UUID)
  sitename:   # Human-readable site name
  provider:   # Infrastructure provider name
  fcorg:      # Organization identifier
status:
  bootstrapstate:     # AwaitHandshake | HandshakeComplete | RegistrationComplete
  controlplanestatus: # Status string
  otp:
    passcode:   # One-time passcode for site-agent bootstrap
    timestamp:  # OTP expiry
```

### CLI flags (set in `deployment.yaml`)

| Flag | Value | Description |
|---|---|---|
| `--listen-port` | `8100` | HTTPS listen port |
| `--creds-manager-url` | `https://carbide-rest-cert-manager.carbide-rest:8000` | URL to carbide-rest-cert-manager |
| `--tls-cert-path` | `/etc/tls/tls.crt` | TLS cert path (from `site-manager-tls` secret) |
| `--tls-key-path` | `/etc/tls/tls.key` | TLS key path (from `site-manager-tls` secret) |
| `--namespace` | `carbide-rest` | Kubernetes namespace to watch for Site CRs |

### Apply

Apply the CRD first, then the rest:

```bash
kubectl apply -f deploy/kustomize/base/site-manager/site-crd.yaml

# Edit deploy/kustomize/overlays/site-manager/kustomization.yaml to set your image
kubectl kustomize --load-restrictor LoadRestrictionsNone \
  deploy/kustomize/overlays/site-manager | kubectl apply -f -

kubectl rollout status deployment/carbide-rest-site-manager -n carbide-rest
```

---

## Step 11 — Deploy `carbide-rest-api`

### What it is

The main Carbide REST API server. It is the northbound interface for all Carbide operations — managing sites, hardware inventory, machine validation, and OS imaging. It authenticates requests via Keycloak JWTs, persists state to PostgreSQL, and dispatches long-running operations to Temporal workflows. It exposes:

- Port **8388** (HTTP) — REST API, versioned at `/v2`
- Port **9360** (HTTP) — Prometheus metrics

### Manifests

| File | Contents |
|---|---|
| `base/api/deployment.yaml` | Deployment `carbide-rest-api` |
| `base/api/configmap.yaml` | ConfigMap `carbide-rest-api-config` — full application `config.yaml` |
| `base/api/service.yaml` | ClusterIP Service on `:8388` + NodePort `30388` for external access |

### Application configuration (`base/api/configmap.yaml`)

Key sections in `config.yaml`:

```yaml
api:
  name: carbide
  route:
    version: v2

db:
  host: postgres.postgres
  port: 5432
  name: forge
  user: forge # Password comes from secret `db-creds`

temporal:
  host: temporal-frontend.temporal
  port: 7233
  serverName: server.temporal.local
  namespace: cloud # `site` for Site Worker
  queue: cloud # `site` for Site Worker
  tls:
    enabled: true
    certPath: /var/secrets/temporal/certs/tls.crt
    keyPath: /var/secrets/temporal/certs/tls.key
    caPath: /var/secrets/temporal/certs/ca.crt
  encryptionKeyPath: /var/secrets/temporal/encryption-key

siteManager:
  enabled: true
  svcEndpoint: "https://carbide-rest-site-manager:8100/v1/site"

keycloak:
  enabled: true
  baseURL: http://keycloak:8082
  externalBaseURL: http://localhost:8082   # browser-facing URL for OIDC redirects
  realm: carbide-dev
  clientID: carbide-api
  clientSecretPath: /var/secrets/keycloak/client-secret
```

### Secrets mounted at runtime

| Secret | Mount path | Description |
|---|---|---|
| `keycloak-client-secret` | `/var/secrets/keycloak/client-secret` | Keycloak OIDC client secret |
| `temporal-encryption-key` | `/var/secrets/temporal/encryption-key` | Temporal payload encryption key |
| `temporal-client-cloud-certs` | `/var/secrets/temporal/certs/` | Temporal mTLS client certs (`tls.crt`, `tls.key`, `ca.crt`) |

### Apply

```bash
# Edit deploy/kustomize/overlays/api/kustomization.yaml to set your image
kubectl kustomize --load-restrictor LoadRestrictionsNone \
  deploy/kustomize/overlays/api | kubectl apply -f -

kubectl rollout status deployment/carbide-rest-api -n carbide-rest
```

The API is reachable at `http://<node-ip>:30388` via NodePort, or at `carbide-rest-api.carbide-rest:8388` within the cluster.

---

## Step 12 — Deploy `carbide-rest-workflow`

### What it is

Two Temporal worker deployments that execute the workflow and activity logic for Carbide REST. They share one image (`carbide-rest-workflow`) but listen on different Temporal namespaces and queues:

- **`carbide-rest-cloud-worker`** — handles system workflows in Temporal namespace: `cloud` and queue: `cloud`. This includes Site health monitoring, Site Agent mTLS cert renewal workflows.
- **`carbide-rest-site-worker`** — handles Site workflows in Temporal namespace `site`, queue `site`. This processes data sent from Site Agents e.g. object inventory.

Both workers connect to PostgreSQL for state persistence and to Temporal over mTLS.

### Manifests

| File | Contents |
|---|---|
| `base/workflow/deployment.yaml` | Two Deployments: `carbide-rest-cloud-worker` and `carbide-rest-site-worker` |
| `base/workflow/configmap.yaml` | ConfigMap `carbide-rest-workflow-config` — shared `config.yaml` |

### Application configuration (`base/workflow/configmap.yaml`)

```yaml
db:
  host: postgres.postgres
  port: 5432
  name: forge
  user: forge # Password comes from secret `db-creds`

temporal:
  host: temporal-frontend.temporal
  port: 7233
  serverName: server.temporal.local
  namespace: cloud   # overridden per-deployment via TEMPORAL_NAMESPACE env var
  queue: cloud       # overridden per-deployment via TEMPORAL_QUEUE env var
  tls:
    enabled: true
    certPath: /var/secrets/temporal/certs/tls.crt
    keyPath: /var/secrets/temporal/certs/tls.key
    caPath: /var/secrets/temporal/certs/ca.crt
  encryptionKeyPath: /var/secrets/temporal/encryption-key
```

Each deployment sets `TEMPORAL_NAMESPACE` and `TEMPORAL_QUEUE` environment variables that override the config file values at runtime.

### Secrets mounted at runtime

| Secret | Description |
|---|---|
| `temporal-encryption-key` | Must match the key used by the API — same key decrypts the same payloads |
| `temporal-client-cloud-certs` | Same cert as the API; Temporal authorizes by client cert CN |

### Apply

```bash
# Edit deploy/kustomize/overlays/workflow/kustomization.yaml to set your image
kubectl kustomize --load-restrictor LoadRestrictionsNone \
  deploy/kustomize/overlays/workflow | kubectl apply -f -

kubectl rollout status deployment/carbide-rest-cloud-worker -n carbide-rest
kubectl rollout status deployment/carbide-rest-site-worker -n carbide-rest
```

---

## Step 13 — Deploy `carbide-rest-site-agent`

### What it is

The site agent (formerly Elektra) is the component that runs at a remote site and bridges it back to the Carbide REST control plane. It connects to the Carbide core gRPC API to collect hardware inventory, and connects to Temporal (on a per-site namespace and queue matching the site UUID) to receive and execute site-specific workflow tasks like OS imaging and machine configuration.

The site agent bootstrap flow is:

1. On first start it reads `site-registration` secret for `site-uuid`, `otp`, and `creds-url`.
2. It calls `carbide-rest-site-manager` at `creds-url` with the OTP to fetch its Temporal client certificates.
3. The received certs are written back into the `temporal-client-site-agent-certs` secret.
4. The agent then connects to Temporal using those certs and starts polling its site-specific namespace and queue.

### Manifests

| File | Contents |
|---|---|
| `base/site-agent/statefulset.yaml` | StatefulSet `carbide-rest-site-agent` |
| `base/site-agent/configmap.yaml` | ConfigMap `carbide-rest-site-agent-config` — env vars |
| `base/site-agent/certificate.yaml` | cert-manager `Certificate` `core-grpc-client-site-agent-certs` — SPIFFE gRPC client cert |
| `base/site-agent/site-registration-secret.yaml` | Secret `site-registration` — bootstrap credentials |
| `base/site-agent/temporal-client-site-agent-certs.yaml` | Secret `temporal-client-site-agent-certs` — placeholder, populated by bootstrap |
| `base/site-agent/rbac.yaml` | ServiceAccount + Role/RoleBinding — needs access to Secrets and CertificateRequests |
| `base/site-agent/service.yaml` | ClusterIP Service on ports 8080 (http) and 2112 (metrics) |

### Key environment variables (`base/site-agent/configmap.yaml`)

| Variable | Default | Description |
|---|---|---|
| `CARBIDE_ADDRESS` | `carbide-rest-mock-core:11079` | Carbide/Forge gRPC endpoint — **set this to your [NVIDIA Infra Controller Core](https://github.com/NVIDIA/ncx-infra-controller-core) address in production** |
| `CLUSTER_ID` | `00000000-0000-4000-8000-000000000001` | Site UUID — **must match a registered site** |
| `TEMPORAL_HOST` | `temporal-frontend.temporal` | Temporal frontend host |
| `TEMPORAL_PORT` | `7233` | Temporal frontend port |
| `TEMPORAL_SERVER` | `interservice.server.temporal.local` | Temporal TLS server name |
| `TEMPORAL_PUBLISH_NAMESPACE` | `site` | Temporal namespace for publishing (site-side workflows) |
| `TEMPORAL_SUBSCRIBE_NAMESPACE` | `00000000-0000-4000-8000-000000000001` | Per-site Temporal namespace — **must match site UUID** |
| `TEMPORAL_SUBSCRIBE_QUEUE` | `00000000-0000-4000-8000-000000000001` | Per-site Temporal queue — **must match site UUID** |
| `TEMPORAL_INVENTORY_SCHEDULE` | `@every 3m` | How often the agent reports hardware inventory |
| `TEMPORAL_CERT_PATH` | `/etc/temporal-certs` | Path to mounted Temporal TLS certs |

### Secrets mounted at runtime

| Secret | Mount path | Description |
|---|---|---|
| `site-registration` | `/etc/sitereg` | `site-uuid`, `otp`, `creds-url`, `cacert` for bootstrap |
| `core-grpc-client-site-agent-certs` | `/etc/carbide` | SPIFFE cert for gRPC to Carbide core (optional, issued by cert-manager) |
| `temporal-client-site-agent-certs` | `/etc/temporal-certs` | Temporal mTLS certs: `otp`, `cacertificate`, `certificate`, `key` — populated during bootstrap |

### SPIFFE gRPC certificate

The `certificate.yaml` resource issues a cert-manager `Certificate` with SPIFFE URI:
```
spiffe://carbide.local/carbide-rest/sa/carbide-rest-site-agent
```
This is the client identity the site agent presents when connecting to the Carbide core gRPC API.

### Configuring a site for bootstrap

After registering a site through the API, patch the `site-registration` secret with the real site UUID, OTP (from the `Site` CR status), and CA cert:

```bash
SITE_UUID=<your-site-uuid>
OTP=<otp-from-site-cr-status>
CA_B64=$(kubectl get secret ca-signing-secret -n carbide-rest -o jsonpath='{.data.tls\.crt}')

kubectl patch secret site-registration -n carbide-rest --type='json' -p="[
  {\"op\": \"replace\", \"path\": \"/data/site-uuid\", \"value\": \"$(echo -n $SITE_UUID | base64)\"},
  {\"op\": \"replace\", \"path\": \"/data/otp\", \"value\": \"$(echo -n $OTP | base64)\"},
  {\"op\": \"replace\", \"path\": \"/data/cacert\", \"value\": \"$CA_B64\"}
]"

# Also update CLUSTER_ID in the configmap to match SITE_UUID
kubectl patch configmap carbide-rest-site-agent-config -n carbide-rest --type='json' -p="[
  {\"op\": \"replace\", \"path\": \"/data/CLUSTER_ID\", \"value\": \"$SITE_UUID\"},
  {\"op\": \"replace\", \"path\": \"/data/TEMPORAL_SUBSCRIBE_NAMESPACE\", \"value\": \"$SITE_UUID\"},
  {\"op\": \"replace\", \"path\": \"/data/TEMPORAL_SUBSCRIBE_QUEUE\", \"value\": \"site\"}
]"

kubectl rollout restart statefulset/carbide-rest-site-agent -n carbide-rest
```

### Apply

```bash
# Edit deploy/kustomize/overlays/site-agent/kustomization.yaml to set your image
kubectl kustomize --load-restrictor LoadRestrictionsNone \
  deploy/kustomize/overlays/site-agent | kubectl apply -f -
```

---

## Building and Pushing Images

Before deploying, build and push the images to a registry accessible from your cluster.

### Build all images

```bash
make docker-build
```

By default images are tagged `localhost:5000/<name>:latest`. Override for your registry:

```bash
make docker-build IMAGE_REGISTRY=my-registry.example.com/carbide IMAGE_TAG=v1.0.0
```

### Available images

| Image | Description |
|---|---|
| `carbide-rest-api` | Main REST API server (port 8388) |
| `carbide-rest-workflow` | Temporal workflow workers (cloud-worker and site-worker) |
| `carbide-rest-site-manager` | Site lifecycle manager |
| `carbide-rest-site-agent` | On-site agent |
| `carbide-rest-db` | Database migrations (runs to completion) |
| `carbide-rest-cert-manager` | Internal PKI certificate manager |

### Authenticate and push

**AWS ECR:**
```bash
aws ecr get-login-password --region us-east-1 \
  | docker login --username AWS --password-stdin 123456789.dkr.ecr.us-east-1.amazonaws.com
```

**Google Artifact Registry:**
```bash
gcloud auth configure-docker
```

**Azure Container Registry:**
```bash
az acr login --name myregistry
```

**Push after building:**
```bash
REGISTRY=my-registry.example.com/carbide
TAG=v1.0.0

make docker-build IMAGE_REGISTRY=$REGISTRY IMAGE_TAG=$TAG

for image in carbide-rest-api carbide-rest-workflow carbide-rest-site-manager \
             carbide-rest-site-agent carbide-rest-db carbide-rest-cert-manager; do
    docker push "$REGISTRY/$image:$TAG"
done
```

---

## Image Registry Configuration

Each overlay in `deploy/kustomize/overlays/` has an `images:` stanza that must be updated to point to your registry before applying:

```yaml
# Example: deploy/kustomize/overlays/api/kustomization.yaml
images:
  - name: carbide-rest-api
    newName: <your-registry>/carbide-rest-api   # ← update this
    newTag: <your-tag>                          # ← update this
```

For a private registry that requires authentication, replace the `image-pull-secret`:

```bash
kubectl create secret docker-registry image-pull-secret \
  --namespace carbide-rest \
  --docker-server=<your-registry> \
  --docker-username=<username> \
  --docker-password=<password> \
  --dry-run=client -o yaml | kubectl apply -f -
```

---

## Applying Overlays

Each overlay in `deploy/kustomize/overlays/` deploys a single component and assumes its dependencies already exist. The general apply pattern is:

```bash
kubectl kustomize --load-restrictor LoadRestrictionsNone \
  deploy/kustomize/overlays/<component> | kubectl apply -f -
```

| Overlay | What it deploys |
|---|---|
| `overlays/cert-manager` | `carbide-rest-cert-manager` Deployment + Service + RBAC |
| `overlays/api` | `carbide-rest-api` Deployment + Services + ConfigMap |
| `overlays/workflow` | `carbide-rest-cloud-worker` + `carbide-rest-site-worker` Deployments + ConfigMap |
| `overlays/site-manager` | `carbide-rest-site-manager` Deployment + Service + Certificate + RBAC |
| `overlays/site-agent` | `carbide-rest-site-agent` StatefulSet + Service + Certificates + RBAC |
| `overlays/db` | `carbide-rest-db-migration` Job |

---

## Interacting with a Deployed Cluster

### CLI (`carbidecli`)

`carbidecli` is a command-line client that wraps the full REST API. It handles environment selection, Keycloak login, and token refresh automatically.

```bash
make carbide-cli             # build and install to $GOPATH/bin
carbidecli init              # generate ~/.carbide/config.yaml
```

Create a config per environment (`~/.carbide/config.yaml`, `~/.carbide/config.staging.yaml`, `~/.carbide/config.prod.yaml`), then use the interactive TUI:

```bash
carbidecli tui
```

Or run commands directly for scripting:

```bash
carbidecli --config ~/.carbide/config.yaml site list
```

See [cli/README.md](cli/README.md) for the full configuration reference and command list.

### Getting an access token

```bash
TOKEN=$(curl -s -X POST "http://<keycloak-host>/realms/carbide-dev/protocol/openid-connect/token" \
  -H "Content-Type: application/x-www-form-urlencoded" \
  -d "client_id=carbide-api" \
  -d "client_secret=<keycloak-client-secret>" \
  -d "grant_type=password" \
  -d "username=admin@example.com" \
  -d "password=adminpassword" | jq -r .access_token)
```

### Example API calls

```bash
# Health check
curl -s http://<api-host>:8388/healthz -H "Authorization: Bearer $TOKEN" | jq .

# Get current tenant
curl -s "http://<api-host>:8388/v2/org/<org>/carbide/tenant/current" \
  -H "Authorization: Bearer $TOKEN" | jq .

# List sites
curl -s "http://<api-host>:8388/v2/org/<org>/carbide/site" \
  -H "Authorization: Bearer $TOKEN" | jq .
```

---

## Secrets Reference

| Secret | Namespace | Created by | Required by |
|---|---|---|---|
| `ca-signing-secret` | `carbide-rest` | Operator (Step 2) | `carbide-rest-cert-manager`, `carbide-rest-ca-issuer` |
| `image-pull-secret` | `carbide-rest` | `base/common/image-pull-secret.yaml` | All workload pods |
| `db-creds` | `carbide-rest` | `base/common/db-creds.yaml` | `carbide-rest-db-migration`, `carbide-rest-api`, workflow workers |
| `keycloak-client-secret` | `carbide-rest` | `base/common/keycloak-client-secret.yaml` | `carbide-rest-api` |
| `temporal-encryption-key` | `carbide-rest` | `base/common/temporal-encryption-key.yaml` | `carbide-rest-api`, workflow workers |
| `temporal-client-cloud-certs` | `carbide-rest` | cert-manager via `base/common/temporal-client-cloud-cert.yaml` | `carbide-rest-api`, workflow workers |
| `site-manager-tls` | `carbide-rest` | cert-manager via `base/site-manager/certificate.yaml` | `carbide-rest-site-manager` |
| `core-grpc-client-site-agent-certs` | `carbide-rest` | cert-manager via `base/site-agent/certificate.yaml` | `carbide-rest-site-agent` |
| `temporal-client-site-agent-certs` | `carbide-rest` | Populated by site-agent bootstrap | `carbide-rest-site-agent` |
| `site-registration` | `carbide-rest` | `base/site-agent/site-registration-secret.yaml` + operator patch | `carbide-rest-site-agent` |
| `admin-creds` | `postgres` | `base/postgres/admin-creds.yaml` | `postgres` StatefulSet |
| `db-creds` | `temporal` | `base/temporal-helm/db-creds.yaml` | Temporal Helm chart |
| `server-interservice-certs` | `temporal` | cert-manager via `base/temporal-helm/certificates.yaml` | Temporal Helm chart |
| `server-cloud-certs` | `temporal` | cert-manager via `base/temporal-helm/certificates.yaml` | Temporal Helm chart |
| `server-site-certs` | `temporal` | cert-manager via `base/temporal-helm/certificates.yaml` | Temporal Helm chart |
