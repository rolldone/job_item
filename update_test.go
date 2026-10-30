package main

import "testing"

func TestUpdateBinaryName(t *testing.T) {
	cases := []struct {
		osName string
		arch   string
		want   string
	}{
		{"linux", "amd64", "job_item_linux_amd64"},
		{"darwin", "arm64", "job_item_darwin_arm64"},
		{"windows", "amd64", "job_item_windows_amd64.exe"},
	}
	for _, tc := range cases {
		if got := updateBinaryName(tc.osName, tc.arch); got != tc.want {
			t.Errorf("updateBinaryName(%q, %q) = %q, want %q", tc.osName, tc.arch, got, tc.want)
		}
	}
}

func TestNormalizeUpdateOS(t *testing.T) {
	cases := map[string]string{
		"linux":   "linux",
		"darwin":  "darwin",
		"macos":   "darwin",
		"windows": "windows",
		"win":     "windows",
	}
	for input, expected := range cases {
		got, err := normalizeUpdateOS(input)
		if err != nil {
			t.Errorf("normalizeUpdateOS(%q): %v", input, err)
			continue
		}
		if got != expected {
			t.Errorf("normalizeUpdateOS(%q) = %q, want %q", input, got, expected)
		}
	}
	if _, err := normalizeUpdateOS("freebsd"); err == nil {
		t.Error("expected error for unsupported OS")
	}
}

func TestNormalizeUpdateArch(t *testing.T) {
	cases := map[string]string{
		"amd64":   "amd64",
		"x86_64":  "amd64",
		"x86":     "386",
		"386":     "386",
		"arm64":   "arm64",
		"aarch64": "arm64",
		"arm":     "arm",
	}
	for input, expected := range cases {
		got, err := normalizeUpdateArch(input)
		if err != nil {
			t.Errorf("normalizeUpdateArch(%q): %v", input, err)
			continue
		}
		if got != expected {
			t.Errorf("normalizeUpdateArch(%q) = %q, want %q", input, got, expected)
		}
	}
	if _, err := normalizeUpdateArch("mips"); err == nil {
		t.Error("expected error for unsupported arch")
	}
}
