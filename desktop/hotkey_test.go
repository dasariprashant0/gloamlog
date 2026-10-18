package main

import (
	"testing"

	"github.com/dasariprashant0/gloamlog/internal/core"
)

func TestAcceleratorFromCarbonShortcut(t *testing.T) {
	for _, c := range []struct {
		name string
		spec *core.HotKeySpec
		want string
		ok   bool
	}{
		{"default ⌃⌥J", &core.HotKeySpec{KeyCode: 38, CarbonModifiers: carbonControl | carbonOption}, "ctrl+option+j", true},
		{"⇧⌘L", &core.HotKeySpec{KeyCode: 37, CarbonModifiers: carbonShift | carbonCmd}, "shift+cmd+l", true},
		{"⌘ space", &core.HotKeySpec{KeyCode: 49, CarbonModifiers: carbonCmd}, "cmd+space", true},
		{"F5 with control", &core.HotKeySpec{KeyCode: 96, CarbonModifiers: carbonControl}, "ctrl+f5", true},
		{"off", nil, "", false},
		{"no ⌘ or ⌃ (would fire while typing)", &core.HotKeySpec{KeyCode: 38, CarbonModifiers: carbonOption | carbonShift}, "", false},
		{"unknown key code", &core.HotKeySpec{KeyCode: 200, CarbonModifiers: carbonCmd}, "", false},
	} {
		got, ok := accelerator(c.spec)
		if got != c.want || ok != c.ok {
			t.Errorf("%s: accelerator = %q, %v; want %q, %v", c.name, got, ok, c.want, c.ok)
		}
	}
}
