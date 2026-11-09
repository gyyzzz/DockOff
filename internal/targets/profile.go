package targets

import (
	"embed"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io/fs"
	"net/url"
	"strings"
)

//go:embed profiles/*.json
var files embed.FS

type Profile struct {
	ID             string            `json:"id"`
	OSID           string            `json:"os_id"`
	OSVersion      string            `json:"os_version"`
	GoArch         string            `json:"go_arch"`
	NativeArch     string            `json:"native_arch"`
	PackageFormat  string            `json:"package_format"`
	PackageManager string            `json:"package_manager"`
	Service        string            `json:"service"`
	EngineVersion  string            `json:"engine_version"`
	ComposeVersion string            `json:"compose_version"`
	Repositories   Repositories      `json:"repositories"`
	Packages       []PackagePin      `json:"packages"`
	Keys           map[string]string `json:"key_sha256"`
}

type Repositories struct {
	OS        string `json:"os"`
	Updates   string `json:"updates"`
	Docker    string `json:"docker"`
	OSKey     string `json:"os_key_url,omitempty"`
	DockerKey string `json:"docker_key_url,omitempty"`
}

type PackagePin struct {
	Name    string `json:"name"`
	Epoch   string `json:"epoch"`
	Version string `json:"version"`
	Release string `json:"release"`
}

func (p PackagePin) EVR() string { return p.Epoch + ":" + p.Version + "-" + p.Release }

func All() ([]Profile, error) {
	entries, err := fs.ReadDir(files, "profiles")
	if err != nil {
		return nil, err
	}
	profiles := make([]Profile, 0, len(entries))
	ids := map[string]bool{}
	identities := map[string]bool{}
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".json") {
			continue
		}
		data, err := files.ReadFile("profiles/" + entry.Name())
		if err != nil {
			return nil, err
		}
		var profile Profile
		if err := json.Unmarshal(data, &profile); err != nil {
			return nil, fmt.Errorf("parse target profile %s: %w", entry.Name(), err)
		}
		if err := profile.Validate(); err != nil {
			return nil, fmt.Errorf("invalid target profile %s: %w", entry.Name(), err)
		}
		if ids[profile.ID] {
			return nil, fmt.Errorf("duplicate target ID %q", profile.ID)
		}
		ids[profile.ID] = true
		identity := strings.ToLower(profile.OSID) + "\x00" + profile.OSVersion + "\x00" + profile.GoArch
		if identities[identity] {
			return nil, fmt.Errorf("duplicate target identity %s %s %s", profile.OSID, profile.OSVersion, profile.GoArch)
		}
		identities[identity] = true
		profiles = append(profiles, profile)
	}
	return profiles, nil
}

func Lookup(id string) (Profile, error) {
	profiles, err := All()
	if err != nil {
		return Profile{}, err
	}
	for _, profile := range profiles {
		if profile.ID == id {
			return profile, nil
		}
	}
	return Profile{}, fmt.Errorf("unsupported target %q", id)
}

func Detect(osID, osVersion, goArch string) (Profile, error) {
	profiles, err := All()
	if err != nil {
		return Profile{}, err
	}
	for _, profile := range profiles {
		if strings.EqualFold(osID, profile.OSID) && osVersion == profile.OSVersion && goArch == profile.GoArch {
			return profile, nil
		}
	}
	return Profile{}, fmt.Errorf("unsupported target: os=%s version=%s arch=%s", osID, osVersion, goArch)
}

func (p Profile) Validate() error {
	if p.ID == "" || p.OSID == "" || p.OSVersion == "" || p.GoArch == "" || p.NativeArch == "" {
		return fmt.Errorf("missing target identity fields")
	}
	if p.PackageFormat != "rpm" || p.PackageManager != "dnf" {
		return fmt.Errorf("unsupported package backend %s/%s", p.PackageFormat, p.PackageManager)
	}
	archMap := map[string]string{"amd64": "x86_64", "arm64": "aarch64"}
	if archMap[p.GoArch] != p.NativeArch {
		return fmt.Errorf("unsupported Go/RPM architecture pair %s/%s", p.GoArch, p.NativeArch)
	}
	if p.Service == "" || p.EngineVersion == "" || p.ComposeVersion == "" {
		return fmt.Errorf("missing service or component version")
	}
	if p.Repositories.OS == "" || p.Repositories.Updates == "" || p.Repositories.Docker == "" {
		return fmt.Errorf("missing repository URL")
	}
	for _, address := range []string{p.Repositories.OS, p.Repositories.Updates, p.Repositories.Docker} {
		u, err := url.Parse(address)
		if err != nil || u.Scheme != "https" || u.Host == "" {
			return fmt.Errorf("repository must be an HTTPS URL: %q", address)
		}
	}
	for _, source := range []string{p.Repositories.OSKey, p.Repositories.DockerKey} {
		if source != "" {
			u, err := url.Parse(source)
			if err != nil || u.Scheme != "https" || u.Host == "" {
				return fmt.Errorf("key source must be an HTTPS URL: %q", source)
			}
		}
	}
	if len(p.Packages) == 0 || len(p.Keys) == 0 {
		return fmt.Errorf("missing package pins or signing keys")
	}
	seen := map[string]bool{}
	for _, pkg := range p.Packages {
		if pkg.Name == "" || pkg.Version == "" || pkg.Release == "" || seen[pkg.Name] {
			return fmt.Errorf("invalid or duplicate package pin %q", pkg.Name)
		}
		seen[pkg.Name] = true
	}
	for name, sum := range p.Keys {
		if name == "" || strings.ContainsAny(name, "/\\\r\n") || len(sum) != 64 {
			return fmt.Errorf("invalid signing key entry %q", name)
		}
		if _, err := hex.DecodeString(sum); err != nil {
			return fmt.Errorf("invalid SHA-256 for signing key %q", name)
		}
	}
	if _, ok := p.Keys["docker.asc"]; !ok || len(p.Keys) != 2 {
		return fmt.Errorf("expected Docker and one OS signing key")
	}
	return nil
}
