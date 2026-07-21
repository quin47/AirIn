//go:build darwin

package inject

/*
#cgo LDFLAGS: -framework CoreGraphics -framework CoreFoundation
#include <CoreGraphics/CoreGraphics.h>
#include <CoreFoundation/CoreFoundation.h>

static void CGEventPostToPid(CGEventTapLocation tap, int pid, CGEventRef event) {
	CGEventPostToPid((pid_t)pid, event);
	CFRelease(event);
}

static CGEventRef createKeyboardEvent(CGEventSourceRef src, CGKeyCode code, bool down) {
	return CGEventCreateKeyboardEvent(src, code, down);
}

static void setUnicodeString(CGEventRef event, int len, const unsigned short *chars) {
	UniCharCount c = (UniCharCount)len;
	CGEventKeyboardSetUnicodeString(event, c, (const UniChar*)chars);
}

static void tapPostEvent(CGEventRef event) {
	CGEventPost(kCGSessionEventTap, event);
	CFRelease(event);
}
*/
import "C"
import (
	"log"
	"time"
)

type injector struct {
	baseInjector
}

func NewInjector() (Injector, error) {
	log.Println("inject: 使用 CGEvent (macOS)")
	return &injector{}, nil
}

func init() {
	checkFunc = func() DepInfo {
		return DepInfo{Found: []string{"CoreGraphics (built-in)"}}
	}
}

func (inj *injector) Inject(text string, definite bool) error {
	return inj.handleStream(text, definite, inj.typeText, inj.backspaces)
}

func (inj *injector) Commit() error {
	inj.mu.Lock()
	defer inj.mu.Unlock()
	inj.lastText = ""
	return nil
}

func (inj *injector) Cancel() error {
	inj.mu.Lock()
	defer inj.mu.Unlock()
	if inj.lastText != "" {
		n := countRunes(inj.lastText)
		inj.lastText = ""
		inj.mu.Unlock()
		err := inj.backspaces(n)
		inj.mu.Lock()
		return err
	}
	return nil
}

func (inj *injector) typeText(text string) error {
	runes := []rune(text)
	for _, r := range runes {
		inj.postUnicode(r)
		time.Sleep(2 * time.Millisecond)
	}
	return nil
}

func (inj *injector) backspaces(n int) error {
	for i := 0; i < n; i++ {
		inj.postKey(51, true) // kVK_Delete
		inj.postKey(51, false)
		time.Sleep(2 * time.Millisecond)
	}
	return nil
}

func (inj *injector) postKey(code uint16, down bool) {
	src := C.CGEventSourceCreate(C.kCGEventSourceStateHIDSystemState)
	if src == nil {
		return
	}
	defer C.CFRelease(C.CFTypeRef(src))

	event := C.createKeyboardEvent(src, C.CGKeyCode(code), C.bool(down))
	if event == nil {
		return
	}
	C.tapPostEvent(event)
}

func (inj *injector) postUnicode(r rune) {
	src := C.CGEventSourceCreate(C.kCGEventSourceStateHIDSystemState)
	if src == nil {
		return
	}
	defer C.CFRelease(C.CFTypeRef(src))

	event := C.createKeyboardEvent(src, 0, true)
	if event == nil {
		return
	}

	chars := []C.ushort{C.ushort(r)}
	C.setUnicodeString(event, C.int(len(chars)), &chars[0])
	C.tapPostEvent(event)

	event = C.createKeyboardEvent(src, 0, false)
	if event != nil {
		C.tapPostEvent(event)
	}
}
