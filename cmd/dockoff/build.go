package main

import (
	"archive/tar"
	"compress/gzip"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/dockoff/dockoff/internal/targets"
)

func runPrepare(args []string) error {
	fs := newFlags("prepare")
	targetID := fs.String("target", "", "target profile ID (see dockoff targets)")
	repo := fs.String("docker-repo", "", "optional Docker RPM repository mirror URL")
	key := fs.String("docker-key", "", "optional Docker RPM signing key file; otherwise fetched from Docker")
	osKey := fs.String("os-key", "", "optional OS RPM signing key file; otherwise fetched from profile URL")
	dest := fs.String("output", "", "new staging directory")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if err := required(*targetID, *dest); err != nil {
		return err
	}
	profile, err := targets.Lookup(*targetID)
	if err != nil {
		return err
	}
	if err := checkBuilderHost(profile); err != nil {
		return err
	}
	if os.Geteuid() != 0 {
		return fmt.Errorf("prepare requires root for DNF")
	}
	dockerRepo := profile.Repositories.Docker
	if *repo != "" {
		dockerRepo = *repo
	}
	if err := validHTTPS(dockerRepo); err != nil {
		return err
	}
	for _, name := range []string{"dnf", "rpm", "rpmkeys"} {
		if _, err := exec.LookPath(name); err != nil {
			return err
		}
	}
	destination, err := filepath.Abs(*dest)
	if err != nil {
		return err
	}
	if _, err := os.Lstat(destination); !os.IsNotExist(err) {
		return fmt.Errorf("output must not already exist: %s", destination)
	}
	if err := os.MkdirAll(filepath.Dir(destination), 0755); err != nil {
		return err
	}
	root, err := os.MkdirTemp(filepath.Dir(destination), ".dockoff-prepare-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(root)
	sources := []string{profile.Repositories.OS, profile.Repositories.Updates, strings.TrimRight(dockerRepo, "/") + "/"}
	keyDir := filepath.Join(root, "keys", profile.ID)
	if err := os.MkdirAll(keyDir, 0755); err != nil {
		return err
	}
	dockerKey := filepath.Join(keyDir, "docker.asc")
	if *key != "" {
		err = copyFile(*key, dockerKey, 0644)
	} else {
		keyURL := profile.Repositories.DockerKey
		if keyURL == "" {
			keyURL = "https://download.docker.com/linux/centos/gpg"
		}
		err = fetch(keyURL, dockerKey)
	}
	if err != nil {
		return fmt.Errorf("Docker signing key: %w", err)
	}
	osKeyName := ""
	for name := range profile.Keys {
		if name != "docker.asc" {
			osKeyName = name
		}
	}
	osKeyPath := filepath.Join(keyDir, osKeyName)
	if *osKey != "" {
		err = copyFile(*osKey, osKeyPath, 0644)
	} else if profile.Repositories.OSKey != "" {
		err = fetch(profile.Repositories.OSKey, osKeyPath)
	} else {
		return fmt.Errorf("target %s requires --os-key", profile.ID)
	}
	if err != nil {
		return err
	}
	for name, sum := range profile.Keys {
		actual, err := hashFile(filepath.Join(keyDir, name))
		if err != nil {
			return err
		}
		if actual != sum {
			return fmt.Errorf("unrecognized %s signing key", name)
		}
	}
	repository := filepath.Join(root, "rpms", profile.ID)
	if err := os.MkdirAll(repository, 0755); err != nil {
		return err
	}
	repoDir := filepath.Join(root, "repos.d")
	if err := os.MkdirAll(repoDir, 0755); err != nil {
		return err
	}
	installRoot := filepath.Join(root, "installroot")
	if err := os.MkdirAll(installRoot, 0755); err != nil {
		return err
	}
	if err := command("rpm", "--root", installRoot, "--initdb"); err != nil {
		return err
	}
	for name := range profile.Keys {
		if err := command("rpm", "--root", installRoot, "--import", filepath.Join(keyDir, name)); err != nil {
			return err
		}
	}
	config := filepath.Join(root, "build.conf")
	if err := writeBuildConfig(config, repoDir, sources); err != nil {
		return err
	}
	cmdArgs := []string{"--config=" + config, "--installroot=" + installRoot, "--releasever=/", "--setopt=install_weak_deps=False", "--assumeyes", "--downloadonly", "--downloaddir=" + repository, "install"}
	for _, pin := range profile.Packages {
		cmdArgs = append(cmdArgs, pin.Name+"-"+pin.EVR()+"."+profile.NativeArch)
	}
	if err := command("dnf", cmdArgs...); err != nil {
		return err
	}
	packages, err := inspectDirectory(repository, profile)
	if err != nil {
		return err
	}
	if err := verifySignatures(root, packages, profile); err != nil {
		return err
	}
	if err := os.Remove(config); err != nil {
		return err
	}
	files, err := inventory(root)
	if err != nil {
		return err
	}
	m := manifest{Format: bundleFormat, Files: files, Targets: map[string]targetPayload{profile.ID: {Packages: packages, Sources: sources}}}
	if err := writeJSON(filepath.Join(root, "manifest.json"), m); err != nil {
		return err
	}
	if err := os.Rename(root, destination); err != nil {
		return err
	}
	fmt.Println("Prepared", destination)
	return nil
}

func validHTTPS(address string) error {
	u, err := url.Parse(address)
	if err != nil || u.Scheme != "https" || u.Host == "" || u.User != nil || strings.ContainsAny(address, "\r\n") {
		return fmt.Errorf("repository must be an HTTPS URL without credentials: %q", address)
	}
	return nil
}

func fetch(address, path string) error {
	if err := validHTTPS(address); err != nil {
		return err
	}
	client := &http.Client{Timeout: 60 * time.Second, CheckRedirect: func(req *http.Request, via []*http.Request) error {
		if len(via) >= 10 {
			return fmt.Errorf("too many redirects")
		}
		return validHTTPS(req.URL.String())
	}}
	var last error
	for attempt := 0; attempt < 3; attempt++ {
		resp, err := client.Get(address)
		if err != nil {
			last = err
			continue
		}
		data, readErr := io.ReadAll(io.LimitReader(resp.Body, 65537))
		resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			last = fmt.Errorf("%s returned HTTP %d", address, resp.StatusCode)
			continue
		}
		if readErr != nil {
			last = readErr
			continue
		}
		if len(data) > 65536 {
			return fmt.Errorf("signing key response exceeds 64 KiB")
		}
		return os.WriteFile(path, data, 0644)
	}
	return last
}

func writeBuildConfig(path, repoDir string, sources []string) error {
	var b strings.Builder
	fmt.Fprintf(&b, "[main]\nreposdir=%s\nplugins=1\ngpgcheck=1\ninstall_weak_deps=False\ntimeout=30\nretries=2\n", repoDir)
	for i, source := range sources {
		fmt.Fprintf(&b, "\n[build-%d]\nname=build-%d\nbaseurl=%s\nenabled=1\ngpgcheck=1\n", i, i, source)
	}
	return os.WriteFile(path, []byte(b.String()), 0600)
}

func runBundle(args []string) error {
	fs := newFlags("bundle")
	var stagingDirs stringList
	fs.Var(&stagingDirs, "staging", "prepared target staging directory (repeat for multiple targets)")
	dest := fs.String("output", "", "new tar.gz output path")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if err := required(*dest); err != nil {
		return err
	}
	if len(stagingDirs) == 0 {
		return fmt.Errorf("provide one or more --staging directories")
	}
	root, err := os.MkdirTemp("", "dockoff-bundle-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(root)
	m := manifest{Format: bundleFormat, Files: map[string]string{}, Targets: map[string]targetPayload{}}
	for _, dir := range stagingDirs {
		staged, err := readManifest(filepath.Join(dir, "manifest.json"))
		if err != nil {
			return err
		}
		if len(staged.Targets) != 1 {
			return fmt.Errorf("staging directory must contain exactly one target")
		}
		var targetID string
		var payload targetPayload
		for targetID, payload = range staged.Targets {
		}
		if _, exists := m.Targets[targetID]; exists {
			return fmt.Errorf("duplicate target staging: %s", targetID)
		}
		if err := verifyBundle(dir, staged); err != nil {
			return err
		}
		m.Targets[targetID] = payload
		for file := range staged.Files {
			if strings.HasPrefix(file, "keys/") {
				if _, err := os.Stat(filepath.Join(root, filepath.FromSlash(file))); err == nil {
					continue
				}
			}
			if err := copyFile(filepath.Join(dir, filepath.FromSlash(file)), filepath.Join(root, filepath.FromSlash(file)), 0644); err != nil {
				return err
			}
		}
		profile, err := targets.Lookup(targetID)
		if err != nil {
			return err
		}
		cmd := exec.Command("go", "build", "-trimpath", "-o", filepath.Join(root, "install-"+targetID), "./cmd/dockoff")
		for _, env := range os.Environ() {
			if !strings.HasPrefix(env, "GOOS=") && !strings.HasPrefix(env, "GOARCH=") && !strings.HasPrefix(env, "CGO_ENABLED=") {
				cmd.Env = append(cmd.Env, env)
			}
		}
		cmd.Env = append(cmd.Env, "GOOS=linux", "GOARCH="+profile.GoArch, "CGO_ENABLED=0")
		cmd.Stdout, cmd.Stderr = os.Stdout, os.Stderr
		if err := cmd.Run(); err != nil {
			return err
		}
	}
	if len(m.Targets) == 0 {
		return fmt.Errorf("no targets staged")
	}
	files, err := inventory(root)
	if err != nil {
		return err
	}
	m.Files = files
	if err := writeJSON(filepath.Join(root, "manifest.json"), m); err != nil {
		return err
	}
	if err := verifyBundle(root, m); err != nil {
		return err
	}
	if err := archiveDirectory(root, *dest); err != nil {
		return err
	}
	sum, err := hashFile(*dest)
	if err != nil {
		return err
	}
	fmt.Printf("Created %s\nSHA256 %s\n", *dest, sum)
	return nil
}

func archiveDirectory(dir, destination string) error {
	f, err := os.OpenFile(destination, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0644)
	if err != nil {
		return err
	}
	gz := gzip.NewWriter(f)
	tw := tar.NewWriter(gz)
	err = filepath.WalkDir(dir, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			return nil
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		if !info.Mode().IsRegular() {
			return fmt.Errorf("non-regular file: %s", path)
		}
		h, err := tar.FileInfoHeader(info, "")
		if err != nil {
			return err
		}
		h.Name, err = filepath.Rel(dir, path)
		if err != nil {
			return err
		}
		h.Name = filepath.ToSlash(h.Name)
		h.Uid, h.Gid, h.Uname, h.Gname = 0, 0, "", ""
		if err = tw.WriteHeader(h); err != nil {
			return err
		}
		in, err := os.Open(path)
		if err != nil {
			return err
		}
		_, copyErr := io.Copy(tw, in)
		closeErr := in.Close()
		if copyErr != nil {
			return copyErr
		}
		return closeErr
	})
	for _, closer := range []io.Closer{tw, gz, f} {
		if closeErr := closer.Close(); err == nil {
			err = closeErr
		}
	}
	if err != nil {
		os.Remove(destination)
	}
	return err
}
