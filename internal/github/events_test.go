package github

import "testing"

// TestEventPresetsValid ensures every preset references only supported event
// names and has a usable label/short key.
func TestEventPresetsValid(t *testing.T) {
	supported := make(map[string]bool, len(SupportedEvents))
	for _, e := range SupportedEvents {
		supported[e.Name] = true
	}

	for key, preset := range EventPresets {
		if key == "" {
			t.Error("preset with empty key")
		}
		if preset.Label == "" {
			t.Errorf("preset %q has empty label", key)
		}
		if len(preset.Events) == 0 {
			t.Errorf("preset %q has no events", key)
		}
		for _, evt := range preset.Events {
			if !supported[evt] {
				t.Errorf("preset %q references unsupported event %q", key, evt)
			}
		}
	}
}

// TestSupportedEventsShortsUnique ensures callback short codes don't collide.
func TestSupportedEventsShortsUnique(t *testing.T) {
	seen := make(map[string]string)
	for _, e := range SupportedEvents {
		if prev, ok := seen[e.Short]; ok {
			t.Errorf("short %q duplicated: %q and %q", e.Short, prev, e.Name)
		}
		seen[e.Short] = e.Name
	}
}
