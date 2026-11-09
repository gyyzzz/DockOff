# DockOff

DockOff builds an offline Docker installation bundle for openEuler 22.03 LTS-SP4 on x86_64 or aarch64. On a matching connected builder, it collects RPMs for Docker Engine 28.5.2, containerd.io 2.1.5, Compose v2.40.3, and their required dependencies into a `tar.gz` with a target-specific Go installer. On the isolated host, that installer checks the host, bundle integrity, RPM signatures, and offline dependencies, then installs the RPMs, starts Docker, and verifies Engine and Compose without network access. Application images are transferred separately and can be imported with `docker load`.

[中文](README_zh.md)

## Quickstart

Use a connected openEuler 22.03 LTS-SP4 x86_64 builder with Go 1.24+, DNF, `rpm`, and `rpmkeys`. A clean builder is recommended because DNF resolves dependencies against its installed packages. `prepare` requires root and imports verified signing keys into the builder's RPM trust database.

1. Build DockOff and prepare the x86_64 target:

   ```sh
   go build -o dockoff ./cmd/dockoff
   sudo ./dockoff prepare \
     --target openeuler-22.03-lts-sp4-amd64 \
     --output ./staging/openeuler-amd64
   ```

2. Package the staging directory. Record the SHA-256 printed by `bundle` for transfer verification:

   ```sh
   ./dockoff bundle \
     --staging ./staging/openeuler-amd64 \
     --output ./dockoff-docker.tar.gz
   ```

3. Copy the archive to a matching target host, compare its `sha256sum` with the recorded value, then extract and install it as root:

   ```sh
   sha256sum dockoff-docker.tar.gz
   mkdir package
   tar -xzf dockoff-docker.tar.gz -C package
   cd package
   sudo ./install-openeuler-22.03-lts-sp4-amd64 install --bundle . --check
   sudo ./install-openeuler-22.03-lts-sp4-amd64 install --bundle .
   ```

`--check` runs the host, integrity, signature, conflict, and offline dependency checks without installing RPMs or changing the Docker service. To include aarch64, run `prepare` with target `openeuler-22.03-lts-sp4-arm64` on a native aarch64 openEuler SP4 builder, transfer its staging directory to the bundling host, and add another `--staging` to `bundle`. Run `./dockoff targets` to list embedded target IDs.

## Supported targets

| Target ID | Operating system | Architecture | Engine | Compose |
| --- | --- | --- | --- | --- |
| `openeuler-22.03-lts-sp4-amd64` | openEuler 22.03 (LTS-SP4) | x86_64 | 28.5.2 | 2.40.3 |
| `openeuler-22.03-lts-sp4-arm64` | openEuler 22.03 (LTS-SP4) | aarch64 | 28.5.2 | 2.40.3 |
| `bclinux-21.10u5-amd64` | BigCloud Enterprise Linux 21.10 U5 | x86_64 | 28.5.2 | 2.40.3 |

The openEuler x86_64 target has been tested for installation and container operation. BC-Linux 21.10 U5 has passed package staging validation; offline preflight and installation on that target remain untested. The aarch64 profile and installer cross-compilation are available, but installation on ARM hardware has not been tested. Each OS release and architecture has its own target profile; the installer requires an exact match with the host and bundle.

## Configuration and safeguards

`prepare` accepts `--docker-repo <HTTPS URL>` for a Docker RPM mirror and `--docker-key <file>` for the Docker signing key. The key must match the SHA-256 pinned in the target profile. Profiles in `internal/targets/profiles/` also define OS matching, package versions, repositories, and trusted keys.

The installer uses only bundled RPMs, with DNF repositories and plugins disabled. It verifies file hashes, RPM signatures and metadata, package conflicts, disk space, and systemd before installation; logs are written to `/var/log/dockoff/`. The bundle has a SHA-256 checksum but no project-level signature, so compare the printed archive hash after transfer.

## Adding targets

New OS release and architecture combinations require a separate JSON profile and validation on a matching builder and target host. See [the design document](docs/详细设计.md) for the profile fields, workflow, and validation limits.

For a guided BC-Linux target workflow, invoke the project skill `$bc-linux-target`. It collects build and target host identity, asks for component versions (defaulting to the existing openEuler pins), checks repository and signing-key inputs, validates a profile, and runs `prepare` only after confirming its root-side effects. Successful `prepare` is staging validation only; target offline preflight and installation still need separate validation.
