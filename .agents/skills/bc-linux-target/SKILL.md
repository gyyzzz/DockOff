---
name: bc-linux-target
description: Add and validate DockOff support for a specific BC-Linux release and architecture, including profile creation and RPM staging on a matching connected builder.
---

# BC-Linux Target Preparation

Use this skill when adding a BC-Linux release and architecture. A target is one exact OS version and CPU architecture. Do not describe a target as installation-supported until offline preflight and actual installation have been validated.

## Collect and Confirm

Collect `/etc/os-release`, `uname -m`, DNF/RPM tool versions, systemd status, repositories, key information, and available disk space from the build host and target host. The build host must match the target profile's OS identity, version, and native architecture. Prefer a clean disposable connected builder because DNF resolves dependencies against the builder's installed package set.

Ask once for Docker Engine/CLI, containerd.io, and Compose plugin versions. For any unspecified components, propose the matching pins from the existing openEuler profile: Engine/CLI `28.5.2`, containerd.io `2.1.5`, Compose `2.40.3`. Confirm availability and compatibility in the BC-Linux repositories; do not silently choose substitutions.

Check that repository and key URLs use HTTPS. Verify signing key hashes and signatures, but treat trust as a user decision: confirm official key provenance and fingerprint through a trusted channel before adding a hash to a profile or importing it. Do not bypass TLS or signature checks.

## Profile and Build

Add one profile under `internal/targets/profiles/` for each exact release and architecture. Preserve exact `ID`, `VERSION`, Go architecture, RPM architecture, package EVRs, repository URLs, service name, and signing key hashes. Update tests for exact detection and rejection of mismatched versions or architectures.

Run `go test ./...`, `go vet ./...`, and build the CLI for the matching Linux architecture. On the matching connected builder, run `dockoff prepare --target <id> --os-key <trusted-key-file> --output <new-staging-directory>`. `prepare` downloads RPMs and verifies pins and signatures; it does not install Docker. It requires root and imports keys into an isolated temporary RPM database.

Report staging success separately from target install validation. Before claiming support, run `install --check` and actual installation on a matching target, then verify Docker service, component versions, and container operation. Stop and report blockers if host identity, TLS, signatures, dependencies, or disk space do not pass.
