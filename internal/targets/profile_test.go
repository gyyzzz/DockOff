package targets

import "testing"

func TestEmbeddedProfilesAreIndependentTargets(t *testing.T) {
	profiles, err := All()
	if err != nil {
		t.Fatal(err)
	}
	if len(profiles) != 2 {
		t.Fatalf("got %d profiles, want 2", len(profiles))
	}
	for _, profile := range profiles {
		if _, err := Lookup(profile.ID); err != nil {
			t.Errorf("cannot look up %s: %v", profile.ID, err)
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
