# DockOff

DockOff 为 openEuler 22.03 LTS-SP4 的 x86_64 和 aarch64 系统构建 Docker 离线安装包。在匹配的联网构建机上，它收集 Docker Engine 28.5.2、containerd.io 2.1.5、Compose v2.40.3 及所需依赖的 RPM，连同目标架构的 Go 安装器打成一个 `tar.gz`。隔离目标机上的安装器检查系统、归档完整性、RPM 签名和离线依赖后，从本地安装 RPM、启动 Docker 并验证 Engine 和 Compose，全程不访问网络。业务镜像单独传输，安装后可用 `docker load` 导入。

[English](README.md)

## Quickstart（快速开始）

准备一台联网的 openEuler 22.03 LTS-SP4 x86_64 构建机，安装 Go 1.24+、DNF、`rpm` 和 `rpmkeys`。建议使用干净系统，因为 DNF 会根据构建机已安装的软件解析依赖。`prepare` 需要 root 权限，并会将已校验的签名公钥导入构建机的 RPM 信任库。

1. 编译 DockOff 并准备 x86_64 目标：

   ```sh
   go build -o dockoff ./cmd/dockoff
   sudo ./dockoff prepare \
     --target openeuler-22.03-lts-sp4-amd64 \
     --output ./staging/openeuler-amd64
   ```

2. 将 staging 目录打包。记录 `bundle` 输出的 SHA-256，以便传输后核对：

   ```sh
   ./dockoff bundle \
     --staging ./staging/openeuler-amd64 \
     --output ./dockoff-docker.tar.gz
   ```

3. 将归档复制到匹配的目标机，用 `sha256sum` 与记录值核对，再解压并以 root 安装：

   ```sh
   sha256sum dockoff-docker.tar.gz
   mkdir package
   tar -xzf dockoff-docker.tar.gz -C package
   cd package
   sudo ./install-openeuler-22.03-lts-sp4-amd64 install --bundle . --check
   sudo ./install-openeuler-22.03-lts-sp4-amd64 install --bundle .
   ```

`--check` 只检查系统、完整性、签名、冲突和离线依赖，不安装 RPM 或更改 Docker 服务。如需包含 aarch64，在原生 aarch64 openEuler SP4 构建机上使用目标 `openeuler-22.03-lts-sp4-arm64` 执行 `prepare`，将其 staging 目录传到打包机，并给 `bundle` 增加一个 `--staging`。运行 `./dockoff targets` 可列出内置目标 ID。

## 支持目标

| Target ID | 操作系统 | 架构 | Engine | Compose |
| --- | --- | --- | --- | --- |
| `openeuler-22.03-lts-sp4-amd64` | openEuler 22.03 (LTS-SP4) | x86_64 | 28.5.2 | 2.40.3 |
| `openeuler-22.03-lts-sp4-arm64` | openEuler 22.03 (LTS-SP4) | aarch64 | 28.5.2 | 2.40.3 |

x86_64 目标已验证安装和容器运行。aarch64 已有 profile 和安装器交叉编译支持，尚未在 ARM 实机验证安装。每个系统版本和架构都有独立的目标 profile；安装器要求目标机与归档中的目标精确匹配。

## 配置与校验

`prepare` 支持用 `--docker-repo <HTTPS URL>` 指定 Docker RPM 镜像，用 `--docker-key <文件>` 指定 Docker 签名公钥。公钥必须匹配目标 profile 中固定的 SHA-256。`internal/targets/profiles/` 还定义系统匹配条件、软件版本、仓库和可信公钥。

安装器禁用 DNF 软件源和插件，仅使用归档中的 RPM。安装前检查文件散列、RPM 签名与元数据、软件冲突、磁盘空间和 systemd；日志写入 `/var/log/dockoff/`。归档提供 SHA-256 校验值，但没有项目级签名，因此传输后应核对 `bundle` 输出的归档散列。

## 新增目标

新增系统版本或架构时，需要独立的 JSON profile，并在匹配的构建机和目标机上验证。profile 字段、操作流程及验证边界见[详细设计](docs/详细设计.md)。
