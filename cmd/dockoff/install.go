package main

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/dockoff/dockoff/internal/targets"
)

var taskLog io.Writer = io.Discard

func report(format string, args ...any) {
	fmt.Fprintf(io.MultiWriter(os.Stdout, taskLog), format+"\n", args...)
}

func runInstall(args []string) (result error) {
	fs := newFlags("install")
	bundle := fs.String("bundle", ".", "extracted bundle directory")
	check := fs.Bool("check", false, "verify host, integrity, signatures and dependencies without installing")
	if err := fs.Parse(args); err != nil {
		return err
	}
	profile, err := detectHostProfile()
	if err != nil {
		return err
	}
	if os.Geteuid() != 0 {
		return fmt.Errorf("install requires root")
	}
	unlock, err := installLock()
	if err != nil {
		return err
	}
	defer unlock()
	if err := os.MkdirAll("/var/log/dockoff", 0700); err != nil {
		return err
	}
	logPath := filepath.Join("/var/log/dockoff", time.Now().Format("20060102-150405.000000000")+".log")
	log, err := os.OpenFile(logPath, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		return err
	}
	taskLog = log
	defer func() {
		if result != nil {
			report("FAILED: %v", result)
		}
		taskLog = io.Discard
		log.Close()
	}()
	report("Log: %s", logPath)
	root, err := filepath.Abs(*bundle)
	if err != nil {
		return err
	}
	if strings.ContainsAny(root, "\r\n$") {
		return fmt.Errorf("bundle path cannot contain newlines or dollar signs")
	}
	for _, tool := range []string{"rpm", "rpmkeys", profile.PackageManager, "systemctl"} {
		if _, err := exec.LookPath(tool); err != nil {
			return err
		}
	}
	if _, err := os.Stat("/run/systemd/system"); err != nil {
		return fmt.Errorf("requires running systemd: %w", err)
	}
	m, err := readManifest(filepath.Join(root, "manifest.json"))
	if err != nil {
		return err
	}
	payload, ok := m.Targets[profile.ID]
	if !ok {
		return fmt.Errorf("bundle does not contain target %s", profile.ID)
	}
	packages := payload.Packages
	report("Checking bundle integrity for target %s", profile.ID)
	if err := verifyBundle(root, m); err != nil {
		return err
	}
	if err := verifySignatures(root, packages, profile); err != nil {
		return err
	}
	repository := filepath.Join(root, "rpms", profile.ID)
	for _, p := range packages {
		actual, err := inspectRPM(filepath.Join(repository, p.File))
		if err != nil {
			return err
		}
		if actual != p {
			return fmt.Errorf("RPM metadata differs from manifest: %s", p.File)
		}
	}
	if err := checkConflicts(packages); err != nil {
		return err
	}
	for _, path := range []string{"/", "/usr", "/var"} {
		if err := checkSpace(path, 1<<30); err != nil {
			return err
		}
	}
	work, err := os.MkdirTemp("", "dockoff-install-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(work)
	config := filepath.Join(work, "dnf.conf")
	if err := os.MkdirAll(filepath.Join(work, "empty-repos"), 0700); err != nil {
		return err
	}
	if err := writeInstallConfig(config, work); err != nil {
		return err
	}
	localRPMs := make([]string, 0, len(packages))
	for _, p := range packages {
		localRPMs = append(localRPMs, filepath.Join(repository, p.File))
	}
	// DNF's assume-no transaction resolves dependencies from only these local
	// RPM paths. Its expected "Operation aborted" is a successful dry run.
	preflight := append([]string{"--config=" + config, "--noplugins", "--disablerepo=*", "--setopt=localpkg_gpgcheck=0", "--assumeno", "install"}, localRPMs...)
	if err := preflightInstall(preflight); err != nil {
		return fmt.Errorf("offline dependency preflight failed: %w", err)
	}
	if *check {
		report("Checks passed; no RPM installation or service changes performed")
		return nil
	}
	report("Installing bundled RPM files locally")
	for name := range profile.Keys {
		if err := command("rpm", "--import", filepath.Join(root, "keys", profile.ID, name)); err != nil {
			return err
		}
	}
	install := append([]string{"--config=" + config, "--noplugins", "--disablerepo=*", "--setopt=localpkg_gpgcheck=1", "--setopt=install_weak_deps=False", "--assumeyes", "install"}, localRPMs...)
	if err := command("dnf", install...); err != nil {
		return fmt.Errorf("RPM transaction failed; inspect %s and dnf history: %w", logPath, err)
	}
	if err := command("systemctl", "enable", "--now", profile.Service); err != nil {
		return err
	}
	if err := command("systemctl", "is-active", profile.Service); err != nil {
		return err
	}
	if err := command("systemctl", "is-enabled", profile.Service); err != nil {
		return err
	}
	for _, pin := range profile.Packages {
		name, version := pin.Name, pin.EVR()
		installed, err := output("rpm", "-q", "--qf", "%{EPOCHNUM}:%{VERSION}-%{RELEASE}", name)
		if err != nil || installed != version {
			return fmt.Errorf("installed %s version mismatch: %q (%v)", name, installed, err)
		}
	}
	engine, err := output("docker", "version", "--format", "{{.Server.Version}}")
	if err != nil || engine != profile.EngineVersion {
		return fmt.Errorf("Engine verification failed: %q (%v)", engine, err)
	}
	compose, err := output("docker", "compose", "version", "--short")
	if err != nil || strings.TrimPrefix(compose, "v") != profile.ComposeVersion {
		return fmt.Errorf("Compose verification failed: %q (%v)", compose, err)
	}
	if _, err := output("docker", "info"); err != nil {
		return fmt.Errorf("Docker info verification failed: %w", err)
	}
	report("Installed: Docker %s; Compose %s. No images were downloaded.", engine, compose)
	return nil
}

func preflightInstall(args []string) error {
	cmd := exec.Command("dnf", args...)
	cmd.Env = envWith("LC_ALL", "C")
	var output strings.Builder
	cmd.Stdout, cmd.Stderr = &output, &output
	err := cmd.Run()
	if err == nil {
		return nil
	}
	if strings.Contains(output.String(), "Operation aborted.") && strings.Contains(output.String(), "Dependencies resolved.") {
		fmt.Print(output.String())
		fmt.Fprint(taskLog, output.String())
		return nil
	}
	return fmt.Errorf("dnf preflight: %w: %s", err, output.String())
}

func verifySignatures(root string, packages []rpmInfo, profile targets.Profile) error {
	db, err := os.MkdirTemp("", "dockoff-rpmdb-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(db)
	if _, err := output("rpm", "--dbpath", db, "--initdb"); err != nil {
		return err
	}
	for name, expected := range profile.Keys {
		path := filepath.Join(root, "keys", profile.ID, name)
		sum, err := hashFile(path)
		if err != nil {
			return err
		}
		if sum != expected {
			return fmt.Errorf("unrecognized signing key %s", name)
		}
		if _, err := output("rpmkeys", "--dbpath", db, "--import", path); err != nil {
			return err
		}
	}
	args := []string{"--dbpath", db, "--checksig"}
	for _, p := range packages {
		args = append(args, filepath.Join(root, "rpms", profile.ID, p.File))
	}
	checked, err := output("rpmkeys", args...)
	if err != nil {
		return err
	}
	lines := strings.Split(checked, "\n")
	if len(lines) != len(packages) {
		return fmt.Errorf("unexpected RPM signature check output")
	}
	for _, line := range lines {
		// rpmkeys may return success for unsigned packages; require a signature too.
		if !strings.HasSuffix(line, ": digests signatures OK") {
			return fmt.Errorf("RPM signature not verified: %s", line)
		}
	}
	return nil
}

func checkConflicts(packages []rpmInfo) error {
	text, err := output("rpm", "-qa", "--qf", "%{NAME}\t%{EPOCHNUM}:%{VERSION}-%{RELEASE}\t%{ARCH}\n")
	if err != nil {
		return err
	}
	conflicts := map[string]bool{"docker-engine": true, "docker": true, "docker-common": true, "docker-latest": true, "docker-latest-logrotate": true, "docker-logrotate": true, "docker-selinux": true, "podman-docker": true, "containerd": true, "runc": true, "moby-engine": true, "moby-cli": true, "docker-compose": true, "docker-ce-rootless-extras": true}
	expected := map[string]rpmInfo{}
	for _, p := range packages {
		expected[p.Name] = p
	}
	for _, line := range strings.Split(text, "\n") {
		fields := strings.Split(line, "\t")
		if len(fields) != 3 {
			continue
		}
		if conflicts[fields[0]] {
			return fmt.Errorf("conflicting installed package %s; remove or migrate it explicitly first", fields[0])
		}
		if p, ok := expected[fields[0]]; ok && (fields[1] != p.evr() || fields[2] != p.Arch) {
			return fmt.Errorf("existing %s is %s.%s; automatic upgrades/replacements are not supported", fields[0], fields[1], fields[2])
		}
	}
	return nil
}

func writeInstallConfig(path, work string) error {
	text := fmt.Sprintf("[main]\nreposdir=%s\nplugins=0\ngpgcheck=1\nlocalpkg_gpgcheck=1\ninstall_weak_deps=False\nmetadata_expire=-1\ncachedir=%s\npersistdir=%s\nlogdir=%s\nkeepcache=True\n", filepath.Join(work, "empty-repos"), filepath.Join(work, "cache"), filepath.Join(work, "persist"), work)
	return os.WriteFile(path, []byte(text), 0600)
}
