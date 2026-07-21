//go:build darwin

package hotkey

import (
	"log"
	"strings"

	hook "github.com/robotn/gohook"
)

type platformListener struct {
	onKeyDown func(string)
	onKeyUp   func(string)
	evChan    chan hook.Event
}

const (
	macCtrlCode  = 59
	macAltCode   = 58
	macShiftCode = 56
	macCmdCode   = 55
)

func newPlatformListener(onDown, onUp func(string)) *platformListener {
	return &platformListener{
		onKeyDown: onDown,
		onKeyUp:   onUp,
	}
}

func (p *platformListener) start() error {
	p.evChan = hook.Start()
	go p.loop()
	log.Println("hotkey: listening for global key events (macOS)")
	return nil
}

func (p *platformListener) stop() {
	hook.End()
}

func (p *platformListener) loop() {
	for ev := range p.evChan {
		if ev.Kind == hook.KeyDown || ev.Kind == hook.KeyHold {
			p.onKeyDown(resolveKey(ev))
		} else if ev.Kind == hook.KeyUp {
			p.onKeyUp(resolveKey(ev))
		}
	}
}

func resolveKey(ev hook.Event) string {
	switch ev.Rawcode {
	case macCtrlCode:
		return "ctrl"
	case macAltCode:
		return "alt"
	case macShiftCode:
		return "shift"
	case macCmdCode:
		return "super"
	}

	name := strings.ToLower(hook.RawcodetoKeychar(int(ev.Rawcode)))
	if name == "" {
		return ""
	}
	return name
}
