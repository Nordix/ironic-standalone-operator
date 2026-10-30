# Ironic Standalone Operator (IrSO)

Kubernetes operator that deploys and upgrades Ironic for Metal3 from an
`Ironic` custom resource. Project overview: [README.md](README.md).
Contribution rules: [CONTRIBUTING.md](CONTRIBUTING.md) and the
[Metal3 community guide](https://github.com/metal3-io/community/blob/main/CONTRIBUTING.md).

This file holds only what you cannot cheaply infer from the code. Where it
disagrees with the code or the Makefile, trust those and report the
mismatch.

## AI-assisted contributions

Metal3 follows the CNCF and
[Linux Foundation generative AI policy](https://www.linuxfoundation.org/legal/generative-ai).

- AI is a tool. The human who opens a PR is its author and is responsible
  for every line of it, including licensing and third-party content.
- Review is a conversation between humans. A PR where an agent or bot
  answers reviewers will be closed. Agents may draft changes; the
  contributor reads them, understands them, and replies personally.

- Keep changes surgical. The smallest complete change that solves the
  stated problem wins. Every added file, function, flag, dependency or
  test helper must trace to that problem; anything that does not is
  bloat and gets removed. Prefer reusing existing code over new code.
  Report unrelated findings instead of fixing them in the same PR.

## Repository map

Four Go modules (root, `api/`, `test/`, `hack/tools/`), wired together
with `replace` directives. Root `go test ./...` does not cover the
others.

- Reconcile loop: `internal/controller/ironic_controller.go`. Deployment
  logic (containers, secrets, upgrades, status): `pkg/ironic/`.
- Validation: `pkg/ironic/validation.go`, called by both the webhook and
  the controller.
- Supported Ironic versions and their image tags: `SupportedVersions` in
  `api/v1alpha1/ironic_types.go`. Version-dependent behavior:
  `pkg/ironic/version.go`.
- Feature gates: `api/v1alpha1/features.go`.

## Testing

- Targets: `make help` and [docs/testing.md](docs/testing.md). CI jobs:
  [Prow](https://github.com/metal3-io/project-infra/blob/main/prow/config/jobs/metal3-io/ironic-standalone-operator.yaml)
  and `.github/workflows/`.
- `CONTAINER_RUNTIME` defaults to `docker` in the Makefile and to `podman`
  in `hack/*.sh`. Set it to whichever is installed.
- Conventions are whatever the existing code, tests, and lint configs do.
  Read neighboring files and `_test.go` files before writing new ones.
- Before finishing, add or update tests for every behavior change, then
  run `make test`, `make lint` and the `hack/` checks for the file types
  you touched. All must pass.
- Never hand-edit generated output (`zz_generated.*.go`, the CRD, RBAC
  role and webhook manifests under `config/`, `docs/api.md`). Change the
  source and run `make generate manifests`. The `manifests` GitHub job
  fails on any diff.
- Do not run `test/prepare.sh`, `test/run.sh` or `test/local-ironic/`
  scripts unless the user asks. `prepare.sh` installs cert-manager and
  the MariaDB operator with Helm into the current kubeconfig context and
  rewrites the tracked `config/testing/manager_irso_config_patch.yaml`.
  The `local-ironic` scripts use `sudo` to create network interfaces and
  run Ironic on the host with `podman kube play`. CI runs the functional
  tests on every PR.

## Invariants

These apply to writing and to reviewing changes.

- **Ironic versions.** The operator must keep deploying every entry in
  `SupportedVersions`, and each maps to an ironic-image release branch.
  Changes to the environment or files passed to ironic-image containers
  must stay compatible with all of them, or be gated through
  `pkg/ironic/version.go`. A new ironic-image branch needs a new
  `SupportedVersions` entry and a version test in `test/suite_test.go`.
- **API compatibility.** There is one API version, `v1alpha1`. Never
  remove or rename a field, and never tighten validation on existing
  fields, without a deprecation path. Mark such PRs as breaking.
- **Validation.** Webhooks can be disabled (`--webhook-port=0`). Put
  spec validation in `pkg/ironic/validation.go`, never only in
  `internal/webhook/`.
- **RBAC.** Grant permissions with `+kubebuilder:rbac` markers on the
  controller, then `make manifests`.
- **Security.** Never weaken security to make something work: no
  disabled TLS verification, broadened RBAC, secrets in code, logs or
  fixtures, or skipped validation. Treat the `Ironic` spec and
  user-provided Secrets and ConfigMaps as untrusted input. Call out any
  change to authentication, TLS, RBAC, Secrets or webhooks in the PR
  description.
- **Tenancy and isolation.** One IrSO serves many namespaces, watching
  all of them or a `WATCH_NAMESPACE` list. Review these areas with extra
  care:
   - Cross-namespace access. An `Ironic` resource must never read or
     modify another namespace's Secrets, ConfigMaps or workloads.
     References resolve in the `Ironic` resource's own namespace, and the
     operator only sees user-provided Secrets and ConfigMaps labeled
     `environment.metal3.io/ironic-standalone-operator: "true"`
     (`pkg/secretutils/`).
   - Role isolation. Anyone who can create an `Ironic` gets pods on the
     host network with `NET_ADMIN` and `NET_RAW`, and with the
     `Overrides` feature gate, arbitrary extra containers in those pods.
     Treat changes to host networking, capabilities, overrides or the
     tenant-facing `ironic_editor_role.yaml` and `ironic_viewer_role.yaml`
     as privilege changes.
   - Multi-tenancy. Status, events, logs and metrics must not leak one
     namespace's credentials or certificates to another.
- **Pinning.** Pin new external references: GitHub Actions by commit SHA,
  container images by digest, downloaded binaries by checksum.

## External references

- [Metal3 Book, IrSO user guide](https://book.metal3.io/irso/introduction)
- [IrSO design proposals](https://github.com/metal3-io/metal3-docs/tree/main/design/ironic-standalone-operator)
- [Metal3 API design principles](https://github.com/metal3-io/metal3-docs/blob/main/design/api-design-principles.md)
