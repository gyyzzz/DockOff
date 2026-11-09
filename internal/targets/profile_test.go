package targets

import "testing"

func TestEmbeddedProfilesAreIndependentTargets(t *testing.T) {
	profiles, err := All()
	if err != nil {
		t.Fatal(err)
	}
	if len(profiles) != 4 {
		t.Fatalf("got %d profiles, want 4", len(profiles))
	}
	for _, profile := range profiles {
		if _, err := Lookup(profile.ID); err != nil {
			t.Errorf("cannot look up %s: %v", profile.ID, err)
		}
	}
}

func TestDetectBCLinuxU5MatchesExactVersionAndArchitecture(t *testing.T) {
	profile, err := Detect("bclinux", "21.10 U5", "amd64")
	if err != nil {
		t.Fatal(err)
	}
	if profile.ID != "bclinux-21.10u5-amd64" {
		t.Fatalf("detected %s", profile.ID)
	}
	for _, tc := range []struct{ version, arch string }{
		{"21.10 U4", "amd64"},
		{"21.10 U5", "arm64"},
	} {
		if _, err := Detect("bclinux", tc.version, tc.arch); err == nil {
			t.Errorf("unexpected match for version=%s arch=%s", tc.version, tc.arch)
		}
	}
}

func TestDetectBCLinuxMatchesExactVersionAndArchitecture(t *testing.T) {
	profile, err := Detect("bclinux", "21.10U4", "amd64")
	if err != nil {
		t.Fatal(err)
	}
	if profile.ID != "bclinux-21.10u4-amd64" {
		t.Fatalf("detected %s", profile.ID)
	}
	for _, tc := range []struct{ version, arch string }{
		{"21.10U3", "amd64"},
		{"21.10U4", "arm64"},
	} {
		if _, err := Detect("bclinux", tc.version, tc.arch); err == nil {
			t.Errorf("unexpected match for version=%s arch=%s", tc.version, tc.arch)
		}
	}
}

func TestDetectMatchesOSVersionAndArchitecture(t *testing.T) {
	profile, err := Detect("OPENEULER", "22.03 (LTS-SP4)", "amd64")
	if err != nil {
		t.Fatal(err)
	}
	if profile.ID != "openeuler-22.03-lts-sp4-amd64" {
		t.Fatalf("detected %s", profile.ID)
	}
	for _, test := range []struct{ version, arch string }{
		{"22.03 (LTS-SP3)", "amd64"},
		{"22.03 (LTS-SP4)", "386"},
	} {
		if _, err := Detect("openEuler", test.version, test.arch); err == nil {
			t.Errorf("unexpected match for version=%s arch=%s", test.version, test.arch)
		}
	}
}
