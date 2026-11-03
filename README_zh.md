# DockOff

DockOff 在联网构建机上收集 Docker RPM 并生成单个离线 `tar.gz`；隔离目标机由 Go 安装器完成检查、安装、启动和验证。安装过程不访问网络。业务镜像不打入安装包，可在安装 Docker 后单独传输并用 `docker load` 导入。

## 支持目标

每个“发行版 + 版本 + 架构”组合都是独立支持目标。当前内置目标：

| Target ID | 操作系统 | 架构 | Docker Engine | Compose |
| --- | --- | --- | --- | --- |
| `openeuler-22.03-lts-sp4-amd64` | openEuler 22.03 (LTS-SP4) | x86_64 | 28.5.2 | 2.40.3 |
| `openeuler-22.03-lts-sp4-arm64` | openEuler 22.03 (LTS-SP4) | aarch64 | 28.5.2 | 2.40.3 |

x86_64 目标已在 openEuler SP4 上验证安装和容器运行。ARM64 目前只支持构建配置和安装器交叉编译，尚未做 ARM 实机验证。

运行 `dockoff targets` 查看内置目标。

## 构建离线包

`prepare` 必须在目标操作系统和目标架构一致的联网构建机上运行。建议使用干净的 openEuler SP4 构建机，因为 DNF 会根据构建机当前已安装的软件解析依赖。构建机需要 Go、DNF、`rpm` 和 `rpmkeys`，并以 root 执行；准备过程会把已校验的签名公钥导入构建机 RPM 信任库。

```sh
go build -o dockoff ./cmd/dockoff

sudo ./dockoff prepare \
  --target openeuler-22.03-lts-sp4-amd64 \
  --output ./staging/openeuler-amd64
```

ARM64 的 staging 应在原生 ARM64 openEuler SP4 构建机生成：

```sh
sudo ./dockoff prepare \
  --target openeuler-22.03-lts-sp4-arm64 \
  --output ./staging/openeuler-arm64
```

如需使用 Docker RPM 镜像，可传 `--docker-repo <HTTPS地址>`；`--docker-key <公钥文件>` 可指定 Docker 官方签名公钥。公钥内容必须匹配 profile 中固定的 SHA-256。目标 profile、组件版本、RPM 架构、仓库和公钥散列均在 `internal/targets/profiles/` 中配置。

一个或多个 staging 目录可以合并为一个归档：

```sh
./dockoff bundle \
  --staging ./staging/openeuler-amd64 \
  --staging ./staging/openeuler-arm64 \
  --output ./dockoff-docker.tar.gz
```

每个 `--staging` 对应一个目标，可以重复传入。`bundle` 根据 profile 中的 Go 架构交叉编译安装器，并打印归档 SHA-256。只打包提供的目标。

## 目标机安装

将归档复制到目标机并解压，然后以 root 执行归档中的 `install-<target-id>`。例如：

```sh
mkdir package
tar -xzf dockoff-docker.tar.gz -C package
cd package
sudo ./install-openeuler-22.03-lts-sp4-amd64 install --bundle .
```

安装器根据目标机 `/etc/os-release` 和自身架构识别 profile，并要求归档中存在完全匹配的目标。`--check` 可只执行环境、清单、签名、冲突和离线依赖预检，不安装 RPM 或启动服务：

```sh
sudo ./install-openeuler-22.03-lts-sp4-amd64 install --bundle . --check
```

安装器禁用全部 DNF 软件源和插件，只从归档中的 RPM 安装。它会校验文件 SHA-256、RPM 签名、包元数据、systemd、磁盘空间和已安装软件冲突，随后启动 Docker 并检查 Engine、Compose 和 daemon。日志写入 `/var/log/dockoff/`。

## 新增支持目标

新增发行版、发行版版本或架构时，为每个新组合增加独立 JSON profile，不复用其他目标的版本锁、仓库地址或 RPM 架构设置。profile 需要定义唯一 `id`、`os_id`、`os_version`、`go_arch`、`native_arch`、包管理器、服务名、组件版本、仓库 URL、RPM pin 和签名公钥 SHA-256。随后在对应联网系统上准备该目标的 staging，验证依赖闭包、签名和离线预检，再将 staging 传给 `bundle`。安装器只有在目标机信息精确匹配 profile 时才会安装。

完整的扩展步骤、清单格式和校验边界见 [详细设计](docs/详细设计.md)。
