//go:build !linux && !darwin

package hotkey

import "log"

type platformListener struct {
	onKeyDown func(string)
	onKeyUp   func(string)
}

func newPlatformListener(onDown, onUp func(string)) *platformListener {
	return &platformListener{onKeyDown: onDown, onKeyUp: onUp}
}

func (p *platformListener) start() error {
	log.Println("hotkey: 平台不支持, 快捷键不可用")
	return nil
}

func (p *platformListener) stop() {}
