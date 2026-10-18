package main

// The Jot shortcut is stored the way the Swift app stored it: a Carbon virtual key code and Carbon modifier bits
// (core.HotKeySpec, default ⌃⌥J = {38, 6144}). Wails registers global shortcuts with the same Carbon API but takes an
// accelerator string ("ctrl+option+j"), so this file translates between the two.

import (
	"strings"

	"github.com/dasariprashant0/gloamlog/internal/core"
)

// Carbon modifier bits (HIToolbox/Events.h).
const (
	carbonCmd     = 0x0100
	carbonShift   = 0x0200
	carbonOption  = 0x0800
	carbonControl = 0x1000
)

// carbonKeyNames is Wails' macKeyCodes table (pkg/application/global_shortcut_darwin.go) the other way round: kVK_* code
// to the accelerator key name.
var carbonKeyNames = map[uint32]string{
	0: "a", 1: "s", 2: "d", 3: "f", 4: "h", 5: "g", 6: "z", 7: "x", 8: "c", 9: "v", 11: "b", 12: "q", 13: "w", 14: "e",
	15: "r", 16: "y", 17: "t", 31: "o", 32: "u", 34: "i", 35: "p", 37: "l", 38: "j", 40: "k", 45: "n", 46: "m",
	18: "1", 19: "2", 20: "3", 21: "4", 22: "6", 23: "5", 25: "9", 26: "7", 28: "8", 29: "0",
	24: "=", 27: "-", 30: "]", 33: "[", 39: "'", 41: ";", 42: "\\", 43: ",", 44: "/", 47: ".", 50: "`",
	36: "return", 48: "tab", 49: "space", 51: "backspace", 117: "delete", 53: "escape", 115: "home", 116: "page up",
	119: "end", 121: "page down", 123: "left", 124: "right", 125: "down", 126: "up",
	122: "f1", 120: "f2", 99: "f3", 118: "f4", 96: "f5", 97: "f6", 98: "f7", 100: "f8", 101: "f9", 109: "f10",
	103: "f11", 111: "f12", 105: "f13", 107: "f14", 113: "f15", 106: "f16", 64: "f17", 79: "f18", 80: "f19", 90: "f20",
}

// accelerator turns a stored shortcut into the Wails form. ok is false for "off" (nil), an unknown key, or a shortcut
// without ⌘ or ⌃ (the recorder never saves one: a bare letter would fire while typing everywhere).
func accelerator(spec *core.HotKeySpec) (string, bool) {
	if spec == nil {
		return "", false
	}
	key, known := carbonKeyNames[spec.KeyCode]
	if !known || spec.CarbonModifiers&(carbonCmd|carbonControl) == 0 {
		return "", false
	}
	var parts []string
	for _, m := range []struct {
		bit  uint32
		name string
	}{{carbonControl, "ctrl"}, {carbonOption, "option"}, {carbonShift, "shift"}, {carbonCmd, "cmd"}} {
		if spec.CarbonModifiers&m.bit != 0 {
			parts = append(parts, m.name)
		}
	}
	return strings.Join(append(parts, key), "+"), true
}
