# BC-Linux Build Discovery

## 21.10 U4 x86_64

Read-only inspection identified `ID=bclinux`, `VERSION=21.10U4`, architecture `x86_64`, DNF 4.14, RPM 4.17, rpmkeys, and running systemd. The host had Docker Engine 18.09 and Compose 1.22 installed, which conflicted with the pinned Docker 28.5.2 package during dependency resolution. Its configured repositories used HTTP, while the profile requires HTTPS. No staging was produced for U4.

## 21.10 U5 x86_64

The matching build host identified `ID=bclinux`, `VERSION=21.10 U5`, architecture `x86_64`, DNF 4.14, RPM 4.17, rpmkeys, and systemd. It had no Docker packages and sufficient disk space. After manually correcting the host clock so TLS certificate validation worked, `prepare` completed using the BC-Linux release key and the HTTPS Docker mirror.

The verified staging directory was `/root/dockoff-staging/bclinux-21.10u5-amd64`. Its manifest contains one target and 132 RPMs, including Docker Engine/CLI `28.5.2`, containerd.io `2.1.5`, and Compose plugin `2.40.3`. No Docker installation was performed. The U5 target still needs `install --check`, actual installation, service verification, and container operation testing before it can be described as installation-supported.
