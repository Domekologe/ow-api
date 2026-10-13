package cache

import "testing"

// TestMakeKeySplitKeyRoundTrip is the regression test for the key collision the
// old fmt.Sprintf-based key builder allowed: a tag containing ':' used to
// produce a key the scraper then split incorrectly.
func TestMakeKeySplitKeyRoundTrip(t *testing.T) {
	tests := []struct {
		platform string
		tag      string
	}{
		{"pc", "Player-1234"},
		{"console", "Player#1234"},
		{"pc", "weird:tag:with:colons"},
		{"pc", "tag with spaces"},
		{"pc", "Ünïcode-9999"},
		{"pc", "tag:profile"}, // Must not be mistaken for the profile suffix.
	}

	for _, tt := range tests {
		key := makeKey(tt.platform, tt.tag)

		platform, tag, isProfile, ok := SplitKey(key)
		if !ok {
			t.Errorf("SplitKey(%q) failed to parse", key)
			continue
		}
		if platform != tt.platform || tag != tt.tag {
			t.Errorf("SplitKey(%q) = (%q, %q), want (%q, %q)", key, platform, tag, tt.platform, tt.tag)
		}
		if isProfile {
			t.Errorf("SplitKey(%q) reported a profile entry, want false", key)
		}
	}
}

func TestSplitKeyDetectsProfileSuffix(t *testing.T) {
	key := makeKey("pc", "Player-1234") + profileSuffix

	platform, tag, isProfile, ok := SplitKey(key)
	if !ok {
		t.Fatalf("SplitKey(%q) failed to parse", key)
	}
	if !isProfile {
		t.Error("isProfile = false, want true for a key with the profile suffix")
	}
	if platform != "pc" || tag != "Player-1234" {
		t.Errorf("SplitKey = (%q, %q), want (\"pc\", \"Player-1234\")", platform, tag)
	}
}

func TestSplitKeyRejectsForeignKeys(t *testing.T) {
	invalid := []string{
		"",
		"some:other:key",
		"ow:stats:",
		"ow:stats:pconly",
		"session:abc",
	}

	for _, key := range invalid {
		if _, _, _, ok := SplitKey(key); ok {
			t.Errorf("SplitKey(%q) accepted a key it does not own", key)
		}
	}
}

func TestMakeKeyIsNamespaced(t *testing.T) {
	key := makeKey("pc", "Player-1234")
	if len(key) <= len(keyPrefix) || key[:len(keyPrefix)] != keyPrefix {
		t.Fatalf("makeKey produced %q, want it prefixed with %q", key, keyPrefix)
	}
}
