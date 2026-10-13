package service

import "testing"

func TestValidatePlatform(t *testing.T) {
	tests := []struct {
		in   string
		want string
		ok   bool
	}{
		{"pc", "pc", true},
		{"PC", "pc", true},
		{" console ", "console", true},
		{"xbox", "", false},
		{"", "", false},
		{"pc/../admin", "", false},
	}

	for _, tt := range tests {
		got, ok := validatePlatform(tt.in)
		if ok != tt.ok || (ok && got != tt.want) {
			t.Errorf("validatePlatform(%q) = (%q, %v), want (%q, %v)", tt.in, got, ok, tt.want, tt.ok)
		}
	}
}

func TestValidateTagAccepts(t *testing.T) {
	valid := []string{
		"Player-1234",
		"Player#1234",
		"Player",
		"Some Gamertag",
		"Ünïcode-9999",
		"a_b.c-12345",
	}

	for _, tag := range valid {
		if _, ok := validateTag(tag); !ok {
			t.Errorf("validateTag(%q) rejected a legitimate tag", tag)
		}
	}
}

// TestValidateTagRejects covers the inputs that could manipulate the upstream URL.
func TestValidateTagRejects(t *testing.T) {
	invalid := []string{
		"",
		" ",
		"../../etc/passwd",
		"..",
		"player/../../admin",
		"player?query=1",
		"player&x=1",
		"player\x00null",
		"player\nInjected",
		"<script>alert(1)</script>",
		"http://evil.example/",
		"a", // Too short for the pattern.
		"ThisTagIsFarTooLongToBeARealBattleTagAndShouldBeRejectedOutrightXX",
	}

	for _, tag := range invalid {
		if _, ok := validateTag(tag); ok {
			t.Errorf("validateTag(%q) accepted a tag that should be rejected", tag)
		}
	}
}

func TestValidateTagLengthLimit(t *testing.T) {
	long := make([]byte, maxTagLength+1)
	for i := range long {
		long[i] = 'a'
	}
	if _, ok := validateTag(string(long)); ok {
		t.Fatal("validateTag accepted a tag longer than maxTagLength")
	}
}
