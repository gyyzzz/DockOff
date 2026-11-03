package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strings"

	"github.com/dockoff/dockoff/internal/targets"
)

const bundleFormat = 1

type manifest struct {
	Format  int                      `json:"format"`
	Files   map[string]string        `json:"files"`
	Targets map[string]targetPayload `json:"targets"`
}

type targetPayload struct {
	Packages []rpmInfo `json:"packages"`
	Sources  []string  `json:"sources"`
}

type rpmInfo struct {
	File    string `json:"file"`
	Name    string `json:"name"`
	Epoch   string `json:"epoch"`
	Version string `json:"version"`
	Release string `json:"release"`
	Arch    string `json:"arch"`
}

type stringList []string

func (s *stringList) String() string { return strings.Join(*s, ",") }
func (s *stringList) Set(value string) error {
	if strings.TrimSpace(value) == "" {
		return fmt.Errorf("staging path cannot be empty")
	}
	*s = append(*s, value)
	return nil
}

func (p rpmInfo) evr() string { return p.Epoch + ":" + p.Version + "-" + p.Release }

func command(name string, args ...string) error {
	fmt.Println("+", name, strings.Join(args, " "))
	cmd := exec.Command(name, args...)
	cmd.Env = envWith("LC_ALL", "C")
	cmd.Stdin = os.Stdin
	cmd.Stdout, cmd.Stderr = io.MultiWriter(os.Stdout, taskLog), io.MultiWriter(os.Stderr, taskLog)
	return cmd.Run()
}

func output(name string, args ...string) (string, error) {
	cmd := exec.Command(name, args...)
	cmd.Env = envWith("LC_ALL", "C")
	var stderr strings.Builder
	cmd.Stderr = &stderr
	b, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("%s: %w: %s", name, err, stderr.String())
	}
	return strings.TrimSpace(string(b)), nil
}

func envWith(key, value string) []string {
	prefix := key + "="
	env := make([]string, 0, len(os.Environ())+1)
	for _, item := range os.Environ() {
		if !strings.HasPrefix(item, prefix) {
			env = append(env, item)
		}
	}
	return append(env, prefix+value)
}

func readOSRelease() (map[string]string, error) {
	b, err := os.ReadFile("/etc/os-release")
	if err != nil {
		return nil, err
	}
	values := make(map[string]string)
	for _, line := range strings.Split(string(b), "\n") {
		if k, v, ok := strings.Cut(line, "="); ok {
			values[k] = strings.Trim(strings.TrimSpace(v), "\"'")
		}
	}
	return values, nil
}

func detectHostProfile() (targets.Profile, error) {
	if runtime.GOOS != "linux" {
		return targets.Profile{}, fmt.Errorf("target installation requires Linux; current OS is %s", runtime.GOOS)
	}
	values, err := readOSRelease()
	if err != nil {
		return targets.Profile{}, err
	}
	return targets.Detect(values["ID"], values["VERSION"], runtime.GOARCH)
}

func checkBuilderHost(profile targets.Profile) error {
	if runtime.GOOS != "linux" || runtime.GOARCH != profile.GoArch {
		return fmt.Errorf("target %s requires native Linux/%s; current host is %s/%s", profile.ID, profile.GoArch, runtime.GOOS, runtime.GOARCH)
	}
	values, err := readOSRelease()
	if err != nil {
		return err
	}
	_, err = targets.Detect(values["ID"], values["VERSION"], runtime.GOARCH)
	if err != nil {
		return fmt.Errorf("prepare must run on the target OS and version: %w", err)
	}
	if !strings.EqualFold(values["ID"], profile.OSID) || values["VERSION"] != profile.OSVersion {
		return fmt.Errorf("builder host does not match target %s", profile.ID)
	}
	return nil
}

func hashFile(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	h := sha256.New()
	if _, err = io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

func copyFile(src, dst string, mode os.FileMode) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	if err = os.MkdirAll(filepath.Dir(dst), 0755); err != nil {
		return err
	}
	out, err := os.OpenFile(dst, os.O_CREATE|os.O_EXCL|os.O_WRONLY, mode)
	if err != nil {
		return err
	}
	_, copyErr := io.Copy(out, in)
	closeErr := out.Close()
	if copyErr != nil {
		return copyErr
	}
	return closeErr
}

func inspectRPM(path string) (rpmInfo, error) {
	s, err := output("rpm", "-qp", "--qf", "%{NAME}\t%{EPOCHNUM}\t%{VERSION}\t%{RELEASE}\t%{ARCH}", path)
	if err != nil {
		return rpmInfo{}, err
	}
	p := strings.Split(s, "\t")
	if len(p) != 5 {
		return rpmInfo{}, fmt.Errorf("invalid RPM metadata: %s", path)
	}
	return rpmInfo{File: filepath.Base(path), Name: p[0], Epoch: p[1], Version: p[2], Release: p[3], Arch: p[4]}, nil
}

func validatePackages(packages []rpmInfo, profile targets.Profile) error {
	locked := make(map[string]targets.PackagePin, len(profile.Packages))
	for _, pin := range profile.Packages {
		locked[pin.Name] = pin
	}
	seen, files := map[string]bool{}, map[string]bool{}
	for _, p := range packages {
		if p.File != filepath.Base(p.File) || !strings.HasSuffix(p.File, ".rpm") || strings.ContainsAny(p.File, "\\\r\n") {
			return fmt.Errorf("invalid RPM filename: %q", p.File)
		}
		if p.Name == "" || seen[p.Name] || files[p.File] {
			return fmt.Errorf("duplicate or empty RPM entry: %s", p.Name)
		}
		seen[p.Name], files[p.File] = true, true
		if p.Arch != profile.NativeArch && p.Arch != "noarch" {
			return fmt.Errorf("wrong architecture for %s: %s", p.Name, p.Arch)
		}
		if pin, ok := locked[p.Name]; ok && (p.evr() != pin.EVR() || p.Arch != profile.NativeArch) {
			return fmt.Errorf("%s must be %s.%s, found %s.%s", p.Name, pin.EVR(), profile.NativeArch, p.evr(), p.Arch)
		}
	}
	for name := range locked {
		if !seen[name] {
			return fmt.Errorf("missing package %s", name)
		}
	}
	return nil
}

func inspectDirectory(dir string, profile targets.Profile) ([]rpmInfo, error) {
	paths, err := filepath.Glob(filepath.Join(dir, "*.rpm"))
	if err != nil {
		return nil, err
	}
	packages := []rpmInfo{}
	for _, path := range paths {
		p, err := inspectRPM(path)
		if err != nil {
			return nil, err
		}
		packages = append(packages, p)
	}
	if err := validatePackages(packages, profile); err != nil {
		return nil, err
	}
	sort.Slice(packages, func(i, j int) bool { return packages[i].Name < packages[j].Name })
	return packages, nil
}

func writeJSON(path string, value any) error {
	b, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, append(b, '\n'), 0644)
}

func readManifest(path string) (manifest, error) {
	var m manifest
	b, err := os.ReadFile(path)
	if err != nil {
		return m, err
	}
	if err = json.Unmarshal(b, &m); err != nil {
		return m, err
	}
	if m.Format != bundleFormat || len(m.Targets) == 0 {
		return m, fmt.Errorf("unsupported or empty manifest")
	}
	for id, payload := range m.Targets {
		profile, err := targets.Lookup(id)
		if err != nil {
			return m, err
		}
		if err := validatePackages(payload.Packages, profile); err != nil {
			return m, err
		}
	}
	return m, nil
}

func inventory(root string) (map[string]string, error) {
	files := map[string]string{}
	err := filepath.WalkDir(root, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			return nil
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		if !info.Mode().IsRegular() {
			return fmt.Errorf("non-regular bundle member: %s", path)
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		rel = filepath.ToSlash(rel)
		if rel == "manifest.json" {
			return nil
		}
		files[rel], err = hashFile(path)
		return err
	})
	return files, err
}

func verifyBundle(root string, m manifest) error {
	// Compare both directions so altered or unlisted package files are rejected.
	actual, err := inventory(root)
	if err != nil {
		return err
	}
	if len(actual) != len(m.Files) {
		return fmt.Errorf("bundle file inventory differs from manifest")
	}
	for file, sum := range m.Files {
		if sum == "" || actual[file] != sum {
			return fmt.Errorf("checksum mismatch or missing file: %s", file)
		}
	}
	for id, payload := range m.Targets {
		profile, err := targets.Lookup(id)
		if err != nil {
			return err
		}
		for key, sum := range profile.Keys {
			if actual["keys/"+id+"/"+key] != sum {
				return fmt.Errorf("unrecognized signing key for %s: %s", id, key)
			}
		}
		prefix := "rpms/" + id + "/"
		listed := map[string]bool{}
		for _, p := range payload.Packages {
			file := prefix + p.File
			if actual[file] == "" {
				return fmt.Errorf("missing RPM: %s", file)
			}
			listed[file] = true
		}
		for file := range actual {
			if strings.HasPrefix(file, prefix) && strings.HasSuffix(file, ".rpm") && !listed[file] {
				return fmt.Errorf("unlisted RPM: %s", file)
			}
		}
	}
	return nil
}
