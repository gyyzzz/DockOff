package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/dockoff/dockoff/internal/targets"
)

func validTestPackages() []rpmInfo {
	return []rpmInfo{
		{File: "docker-ce.rpm", Name: "docker-ce", Epoch: "3", Version: "28.5.2", Release: "1.el9", Arch: "x86_64"},
		{File: "docker-ce-cli.rpm", Name: "docker-ce-cli", Epoch: "1", Version: "28.5.2", Release: "1.el9", Arch: "x86_64"},
		{File: "containerd.io.rpm", Name: "containerd.io", Epoch: "0", Version: "2.1.5", Release: "1.el9", Arch: "x86_64"},
		{File: "docker-compose-plugin.rpm", Name: "docker-compose-plugin", Epoch: "0", Version: "2.40.3", Release: "1.el9", Arch: "x86_64"},
		{File: "container-selinux.rpm", Name: "container-selinux", Epoch: "2", Version: "2.138", Release: "5.oe2203sp4", Arch: "noarch"},
	}
}

func TestValidatePackages(t *testing.T) {
	profile, err := targets.Lookup("openeuler-22.03-lts-sp4-amd64")
	if err != nil {
		t.Fatal(err)
	}
	if err := validatePackages(validTestPackages(), profile); err != nil {
		t.Fatalf("valid package set rejected: %v", err)
	}
}

func TestValidatePackagesRejectsWrongArchitecture(t *testing.T) {
	packages := validTestPackages()
	packages[0].Arch = "aarch64"
	profile, _ := targets.Lookup("openeuler-22.03-lts-sp4-amd64")
	if err := validatePackages(packages, profile); err == nil {
		t.Fatal("expected architecture mismatch")
	}
}

func TestValidatePackagesRejectsWrongPinnedVersion(t *testing.T) {
	packages := validTestPackages()
	packages[0].Version = "28.5.1"
	profile, _ := targets.Lookup("openeuler-22.03-lts-sp4-amd64")
	if err := validatePackages(packages, profile); err == nil {
		t.Fatal("expected pinned version mismatch")
	}
}

func TestValidatePackagesRejectsMissingComponent(t *testing.T) {
	packages := validTestPackages()
	packages = append(packages[:1], packages[2:]...)
	profile, _ := targets.Lookup("openeuler-22.03-lts-sp4-amd64")
	if err := validatePackages(packages, profile); err == nil {
		t.Fatal("expected missing package error")
	}
}

func TestValidateInstalledPackagesAllowsDistributionReleasePackage(t *testing.T) {
	profile, err := targets.Lookup("bclinux-21.10u5-amd64")
	if err != nil {
		t.Fatal(err)
	}
	installed := "bclinux-release\t0:21.10U5-59.1.bclinux.21.10U5\tx86_64\n"
	if err := validateInstalledPackages(installed, profile); err != nil {
		t.Fatalf("distribution release package rejected: %v", err)
	}
}

func TestValidateInstalledPackagesRejectsOldDockerVersion(t *testing.T) {
	profile, err := targets.Lookup("bclinux-21.10u5-amd64")
	if err != nil {
		t.Fatal(err)
	}
	installed := "docker-ce\t3:26.1.4-1.el9\tx86_64\n"
	if err := validateInstalledPackages(installed, profile); err == nil {
		t.Fatal("expected old Docker version rejection")
	}
}

func TestValidateInstalledPackagesRejectsConflictingRuntime(t *testing.T) {
	profile, err := targets.Lookup("bclinux-21.10u5-amd64")
	if err != nil {
		t.Fatal(err)
	}
	installed := "podman-docker\t0:4.9.0-1\tx86_64\n"
	if err := validateInstalledPackages(installed, profile); err == nil {
		t.Fatal("expected conflicting runtime rejection")
	}
}

func TestInventoryRejectsSymlink(t *testing.T) {
	root := t.TempDir()
	if err := os.Symlink(filepath.Join(root, "missing"), filepath.Join(root, "link")); err != nil {
		t.Fatal(err)
	}
	if _, err := inventory(root); err == nil {
		t.Fatal("expected symlink rejection")
	}
}

func TestManifestRequiresKnownTarget(t *testing.T) {
	path := filepath.Join(t.TempDir(), "manifest.json")
	if err := os.WriteFile(path, []byte(`{"format":1,"files":{},"targets":{"unknown":{"packages":[],"sources":[]}}}`), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := readManifest(path); err == nil {
		t.Fatal("expected unknown target rejection")
	}
}

func TestPreflightAcceptsDNFsExpectedAbort(t *testing.T) {
	dir := t.TempDir()
	dnf := filepath.Join(dir, "dnf")
	if err := os.WriteFile(dnf, []byte("#!/bin/sh\nprintf 'Dependencies resolved.\\nOperation aborted.\\n'\nexit 1\n"), 0755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
	if err := preflightInstall(nil); err != nil {
		t.Fatalf("expected DNF assume-no result to be accepted: %v", err)
	}
}

func TestPreflightRejectsDNFFailure(t *testing.T) {
	dir := t.TempDir()
	dnf := filepath.Join(dir, "dnf")
	if err := os.WriteFile(dnf, []byte("#!/bin/sh\nprintf 'Cannot resolve package\\n'\nexit 1\n"), 0755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
	if err := preflightInstall(nil); err == nil {
		t.Fatal("expected DNF error")
	}
}
