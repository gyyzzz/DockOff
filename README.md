# DockOff

DockOff collects Docker RPMs on a connected builder and creates one offline `tar.gz`. A Go installer on the isolated target checks the host, installs Docker Engine and Compose v2, starts the service, and verifies the result without network access. Business images are kept separate and can be loaded later with `docker load`.

## Supported targets

Each distribution, release, and architecture combination is an independent target:

| Target ID | Operating system | Architecture | Engine | Compose |
| --- | --- | --- | --- | --- |
| `openeuler-22.03-lts-sp4-amd64` | openEuler 22.03 (LTS-SP4) | x86_64 | 28.5.2 | 2.40.3 |
| `openeuler-22.03-lts-sp4-arm64` | openEuler 22.03 (LTS-SP4) | aarch64 | 28.5.2 | 2.40.3 |

The x86_64 target has been tested on openEuler SP4. ARM64 profile support and installer cross-compilation are present, but ARM hardware installation has not been tested. Run `dockoff targets` to list embedded targets.

## Build an offline bundle

Run `prepare` on a connected builder matching the target OS and architecture. Use a clean openEuler SP4 builder because DNF resolves dependencies against installed packages. The builder needs Go, DNF, `rpm`, and `rpmkeys`; run the command as root. Preparation imports the verified signing keys into the builder RPM trust database.

```sh
go build -o dockoff ./cmd/dockoff

sudo ./dockoff prepare \
  --target openeuler-22.03-lts-sp4-amd64 \
  --output ./staging/openeuler-amd64
```

Prepare ARM64 staging on a native ARM64 openEuler SP4 builder:

```sh
sudo ./dockoff prepare \
  --target openeuler-22.03-lts-sp4-arm64 \
  --output ./staging/openeuler-arm64
```

Use `--docker-repo <HTTPS URL>` for a repository mirror and `--docker-key <file>` to supply the Docker signing key. Its SHA-256 must match the target profile. Target IDs, OS matching, package pins, repositories, and trusted key hashes are defined in `internal/targets/profiles/`.

Pass one or more staging directories to `bundle`:

```sh
./dockoff bundle \
  --staging ./staging/openeuler-amd64 \
  --staging ./staging/openeuler-arm64 \
  --output ./dockoff-docker.tar.gz
```

Each `--staging` adds one target. `bundle` cross-compiles the installer from the profile's Go architecture and prints the archive SHA-256. Only supplied targets are included.

## Install

Copy the archive to the target and extract it. Run the matching `install-<target-id>` binary as root:

```sh
mkdir package
tar -xzf dockoff-docker.tar.gz -C package
cd package
sudo ./install-openeuler-22.03-lts-sp4-amd64 install --bundle .
```

The installer detects the target from `/etc/os-release` and its own architecture. The bundle must contain that exact target. Add `--check` to validate the host, manifest, signatures, package conflicts, and offline dependencies without installing RPMs or starting Docker.

The installer disables all DNF repositories and plugins and uses only bundled RPM files. It checks file hashes, RPM signatures and metadata, systemd, disk space, and installed package conflicts before installing Docker and starting its service. Logs are written to `/var/log/dockoff/`.

## Add a target

Add one JSON profile for each new distribution, release, and architecture combination. Give it a unique ID and define OS matching fields, Go and native RPM architectures, package backend, service name, component versions, repositories, RPM pins, and trusted key hashes. Prepare and validate its dependency closure on a matching connected builder, then pass its staging directory to `bundle`. The installer accepts a target only when the host matches that profile.

See [README_zh.md](README_zh.md) and [docs/详细设计.md](docs/详细设计.md) for the full workflow and design.
