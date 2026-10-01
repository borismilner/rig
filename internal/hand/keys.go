package hand

import (
	"fmt"
	"strings"

	"github.com/jezek/xgb/xproto"
)

// Key combinations, carried from AgentBox's internal/hotkey with only the
// parser: rig has no global hotkey grab, and the hand presses keys through
// this spelling.

const (
	modShift = uint16(xproto.ModMaskShift)
	modCtrl  = uint16(xproto.ModMaskControl)
	modAlt   = uint16(xproto.ModMask1)
	modSuper = uint16(xproto.ModMask4)
)

// namedKeys are the keys supported by name; anything else is a single
// character (a-z, 0-9, punctuation) mapped to its Latin-1 keysym.
var namedKeys = map[string]xproto.Keysym{
	"grave": 0x0060, "backtick": 0x0060, "`": 0x0060,
	"space": 0x0020, "escape": 0xff1b, "esc": 0xff1b,
	"return": 0xff0d, "enter": 0xff0d, "tab": 0xff09,
	"comma": 0x002c, "period": 0x002e, "slash": 0x002f,
	"semicolon": 0x003b, "apostrophe": 0x0027,
	"bracketleft": 0x005b, "bracketright": 0x005d,
	"minus": 0x002d, "equal": 0x003d, "backslash": 0x005c,
	"up": 0xff52, "down": 0xff54, "left": 0xff51, "right": 0xff53,
	"home": 0xff50, "end": 0xff57,
	"pageup": 0xff55, "prior": 0xff55, "pagedown": 0xff56, "next": 0xff56,
	"backspace": 0xff08, "delete": 0xffff, "del": 0xffff, "insert": 0xff63,
}

// parseKeys turns "ctrl+alt+t" into a modifier mask and a keysym. Order and
// case do not matter.
func parseKeys(spec string) (uint16, xproto.Keysym, error) {
	var mods uint16
	key := ""
	for _, raw := range strings.Split(spec, "+") {
		p := strings.ToLower(strings.TrimSpace(raw))
		switch p {
		case "":
			continue
		case "super", "meta", "mod4", "win", "cmd":
			mods |= modSuper
		case "ctrl", "control":
			mods |= modCtrl
		case "alt", "mod1", "option":
			mods |= modAlt
		case "shift":
			mods |= modShift
		default:
			if key != "" {
				return 0, 0, fmt.Errorf("key %q names two keys (%q and %q)", spec, key, p)
			}
			key = p
		}
	}
	if key == "" {
		return 0, 0, fmt.Errorf("key %q names no key", spec)
	}
	if ks, ok := namedKeys[key]; ok {
		return mods, ks, nil
	}
	if len(key) > 1 && key[0] == 'f' {
		var n int
		if _, err := fmt.Sscanf(key, "f%d", &n); err == nil && n >= 1 && n <= 24 {
			return mods, xproto.Keysym(0xffbd + n), nil // F1 is keysym 0xffbe
		}
	}
	if r := []rune(key); len(r) == 1 && r[0] < 0x100 {
		return mods, xproto.Keysym(r[0]), nil
	}
	return 0, 0, fmt.Errorf("key %q: unknown key %q", spec, key)
}
