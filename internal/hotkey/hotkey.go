package hotkey

import (
	"airin/internal/config"
	"log"
	"strings"
)

type Listener struct {
	cfg      *config.Config
	onToggle func()

	platform *platformListener
	stopCh   chan struct{}
	keyDown  map[string]bool
}

func New(cfg *config.Config, onToggle func()) *Listener {
	return &Listener{
		cfg:      cfg,
		onToggle: onToggle,
		stopCh:   make(chan struct{}),
		keyDown:  make(map[string]bool),
	}
}

func (l *Listener) Start() error {
	l.platform = newPlatformListener(l.onKeyDown, l.onKeyUp)
	return l.platform.start()
}

func (l *Listener) Stop() {
	close(l.stopCh)
	if l.platform != nil {
		l.platform.stop()
	}
}

func (l *Listener) onKeyDown(key string) {
	l.keyDown[key] = true
	l.checkCombo()
}

func (l *Listener) onKeyUp(key string) {
	delete(l.keyDown, key)
}

func (l *Listener) checkCombo() {
	hk := l.cfg.GetHotkey()

	allModifiersDown := true
	for _, mod := range hk.Modifiers {
		if !l.keyDown[mod] {
			allModifiersDown = false
			break
		}
	}

	if allModifiersDown && l.keyDown[hk.Key] {
		log.Printf("触发快捷键: %s", l.cfg.HotkeyString())
		l.keyDown = make(map[string]bool)
		if l.onToggle != nil {
			go l.onToggle()
		}
	}
}

func NormalizeKey(key string) string {
	key = strings.ToLower(strings.TrimSpace(key))
	aliases := map[string]string{
		"ctrl":      "ctrl",
		"control":   "ctrl",
		"alt":       "alt",
		"option":    "alt",
		"shift":     "shift",
		"super":     "super",
		"cmd":       "super",
		"command":   "super",
		"windows":   "super",
		"win":       "super",
		"meta":      "super",
		"space":     "space",
		" ":         "space",
		"return":    "enter",
		"tab":       "tab",
		"escape":    "escape",
		"esc":       "escape",
		"backspace": "backspace",
		"delete":    "delete",
		"up":        "up",
		"down":      "down",
		"left":      "left",
		"right":     "right",
		"f1":        "f1",
		"f2":        "f2",
		"f3":        "f3",
		"f4":        "f4",
		"f5":        "f5",
		"f6":        "f6",
		"f7":        "f7",
		"f8":        "f8",
		"f9":        "f9",
		"f10":       "f10",
		"f11":       "f11",
		"f12":       "f12",
	}
	if alias, ok := aliases[key]; ok {
		return alias
	}
	return key
}
