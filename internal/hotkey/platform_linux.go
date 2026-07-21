//go:build linux

package hotkey

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"time"
	"unsafe"
)

const (
	evKey = 0x01

	keyRelease = 0
	keyPress   = 1
	keyRepeat  = 2
)

type inputEvent struct {
	Time  syscall.Timeval
	Type  uint16
	Code  uint16
	Value int32
}

type platformListener struct {
	onKeyDown func(string)
	onKeyUp   func(string)
	stopCh    chan struct{}

	fds      []*os.File
	keyState map[uint16]bool
}

var linuxKeyMap = map[uint16]string{
	29:  "ctrl",
	42:  "shift",
	54:  "shift",
	56:  "alt",
	100: "alt",
	97:  "ctrl",
	125: "super",
	126: "super",
	57:  "space",

	16: "q", 17: "w", 18: "e", 19: "r", 20: "t", 21: "y", 22: "u", 23: "i",
	24: "o", 25: "p", 30: "a", 31: "s", 32: "d", 33: "f", 34: "g", 35: "h",
	36: "j", 37: "k", 38: "l", 44: "z", 45: "x", 46: "c", 47: "v", 48: "b",
	49: "n", 50: "m",
}

var linuxCtrlKeyMap = map[uint16]string{
	29:  "ctrl",
	97:  "ctrl",
	42:  "shift",
	54:  "shift",
	56:  "alt",
	100: "alt",
	125: "super",
	126: "super",
}

func newPlatformListener(onDown, onUp func(string)) *platformListener {
	return &platformListener{
		onKeyDown: onDown,
		onKeyUp:   onUp,
		stopCh:    make(chan struct{}),
		keyState:  make(map[uint16]bool),
	}
}

func (p *platformListener) start() error {
	matches, _ := filepath.Glob("/dev/input/event*")
	if len(matches) == 0 {
		log.Println("hotkey: 未找到 /dev/input/event* 设备, 快捷键不可用")
		return nil
	}

	for _, path := range matches {
		name, err := getDeviceName(path)
		if err != nil {
			continue
		}
		name = strings.ToLower(name)
		if !strings.Contains(name, "keyboard") && !strings.Contains(name, "key") {
			continue
		}

		fd, err := os.OpenFile(path, os.O_RDONLY, 0)
		if err != nil {
			continue
		}
		p.fds = append(p.fds, fd)
		log.Printf("hotkey: 监听键盘设备 %s (%s)", path, name)
	}

	if len(p.fds) == 0 {
		log.Println("hotkey: 未找到键盘设备")
		return nil
	}

	go p.loop()
	return nil
}

func (p *platformListener) stop() {
	close(p.stopCh)
	for _, fd := range p.fds {
		fd.Close()
	}
}

func (p *platformListener) loop() {
	for _, fd := range p.fds {
		go p.readEvents(fd)
	}
	<-p.stopCh
}

func (p *platformListener) readEvents(fd *os.File) {
	event := inputEvent{}
	eventSize := int(unsafe.Sizeof(event))
	buf := make([]byte, eventSize)

	for {
		select {
		case <-p.stopCh:
			return
		default:
		}

		fd.SetReadDeadline(time.Now().Add(100 * time.Millisecond))
		n, err := fd.Read(buf)
		if err != nil {
			if !isTimeout(err) {
				return
			}
			continue
		}
		if n != eventSize {
			continue
		}

		event = *(*inputEvent)(unsafe.Pointer(&buf[0]))

		if event.Type != evKey {
			continue
		}

		keyName := p.resolveKey(event.Code)
		if keyName == "" {
			continue
		}

		switch event.Value {
		case keyPress:
			if !p.keyState[event.Code] {
				p.keyState[event.Code] = true
				p.onKeyDown(keyName)
			}
		case keyRelease:
			if p.keyState[event.Code] {
				p.keyState[event.Code] = false
				p.onKeyUp(keyName)
			}
		case keyRepeat:
			if !p.keyState[event.Code] {
				p.keyState[event.Code] = true
				p.onKeyDown(keyName)
			}
		}
	}
}

func (p *platformListener) resolveKey(code uint16) string {
	if name, ok := linuxKeyMap[code]; ok {
		return name
	}
	return ""
}

func getDeviceName(path string) (string, error) {
	fd, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer fd.Close()

	name := make([]byte, 256)
	_, _, errno := syscall.Syscall(syscall.SYS_IOCTL, fd.Fd(), uintptr(0x80ff>>16|(uintptr(unsafe.Sizeof(name))<<16)|0x06), uintptr(unsafe.Pointer(&name[0])))
	if errno != 0 {
		return "", fmt.Errorf("ioctl: %v", errno)
	}

	n := 0
	for n < len(name) && name[n] != 0 {
		n++
	}
	return string(name[:n]), nil
}

func isTimeout(err error) bool {
	if e, ok := err.(interface{ Timeout() bool }); ok {
		return e.Timeout()
	}
	return false
}
