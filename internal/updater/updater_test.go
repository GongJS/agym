package updater

import "testing"

func TestIsNewer(t *testing.T) {
	cases := []struct {
		latest  string
		current string
		want    bool
	}{
		{"v0.2.3", "v0.2.2", true},
		{"0.3.0", "v0.2.9", true},
		{"v1.0.0", "v0.9.9", true},
		{"v0.2.2", "v0.2.2", false},
		{"v0.2.1", "v0.2.2", false},
		{"v0.2.3", "dev", false},
		{"v0.2.3", "", false},
	}
	for _, tc := range cases {
		if got := IsNewer(tc.latest, tc.current); got != tc.want {
			t.Errorf("IsNewer(%q, %q) = %v, want %v", tc.latest, tc.current, got, tc.want)
		}
	}
}

func TestReleaseAssetNames(t *testing.T) {
	arch, bin, err := releaseAssetNames("v0.2.3", "darwin", "arm64")
	if err != nil || arch != "agym_0.2.3_darwin_arm64.tar.gz" || bin != "agym" {
		t.Fatalf("unexpected darwin asset: arch=%q bin=%q err=%v", arch, bin, err)
	}
	arch, bin, err = releaseAssetNames("v0.2.3", "windows", "amd64")
	if err != nil || arch != "agym_0.2.3_windows_amd64.zip" || bin != "agym.exe" {
		t.Fatalf("unexpected windows asset: arch=%q bin=%q err=%v", arch, bin, err)
	}
}
