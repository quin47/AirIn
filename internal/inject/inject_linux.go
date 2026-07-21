//go:build linux

package inject

import (
	"fmt"
	"log"
	"os"
	"os/exec"
	"strings"
	"time"
)

type injector struct {
	baseInjector
	useClipboard bool
}

func NewInjector() (Injector, error) {
	inj := &injector{}

	if os.Getenv("WAYLAND_DISPLAY") != "" {
		if _, err := exec.LookPath("wtype"); err == nil {
			log.Println("inject: 使用 wtype (Wayland)")
			return inj, nil
		}
		if _, err := exec.LookPath("ydotool"); err == nil {
			log.Println("inject: 使用 ydotool (Wayland)")
			return inj, nil
		}
	}

	if os.Getenv("DISPLAY") != "" {
		if _, err := exec.LookPath("xdotool"); err == nil {
			log.Println("inject: 使用 xdotool (X11)")
			return inj, nil
		}
	}

	if _, err := exec.LookPath("xclip"); err == nil {
		if _, err := exec.LookPath("xdotool"); err == nil {
			inj.useClipboard = true
			log.Println("inject: 使用 xclip + xdotool 剪贴板方案")
			return inj, nil
		}
	}

	return &noopInjector{}, fmt.Errorf("未找到文本注入工具 (xdotool/wtype/ydotool)")
}

func init() {
	checkFunc = func() DepInfo {
		var info DepInfo
		for _, exe := range []string{"xdotool", "wtype", "ydotool", "xclip"} {
			if _, err := exec.LookPath(exe); err == nil {
				info.Found = append(info.Found, exe)
			} else {
				info.Miss = append(info.Miss, exe)
			}
		}
		return info
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
	if text == "" {
		return nil
	}

	if inj.useClipboard {
		return inj.typeViaClipboard(text)
	}

	return inj.typeViaXdotool(text)
}

func (inj *injector) backspaces(n int) error {
	if n <= 0 {
		return nil
	}
	keys := strings.Repeat("BackSpace ", n)
	keys = strings.TrimSpace(keys)
	return exec.Command("xdotool", "key", keys).Run()
}

func (inj *injector) typeViaXdotool(text string) error {
	if os.Getenv("WAYLAND_DISPLAY") != "" {
		if _, err := exec.LookPath("wtype"); err == nil {
			return exec.Command("wtype", "--", text).Run()
		}
		if _, err := exec.LookPath("ydotool"); err == nil {
			return exec.Command("ydotool", "type", text).Run()
		}
	}
	return exec.Command("xdotool", "type", "--", text).Run()
}

func (inj *injector) typeViaClipboard(text string) error {
	save := exec.Command("xclip", "-selection", "clipboard", "-out")
	oldData, _ := save.Output()

	echo := exec.Command("xclip", "-selection", "clipboard", "-in")
	echo.Stdin = strings.NewReader(text)
	if err := echo.Run(); err != nil {
		return fmt.Errorf("xclip copy: %w", err)
	}

	time.Sleep(30 * time.Millisecond)

	if err := exec.Command("xdotool", "key", "ctrl+v").Run(); err != nil {
		return fmt.Errorf("xdotool paste: %w", err)
	}

	if len(oldData) > 0 {
		restore := exec.Command("xclip", "-selection", "clipboard", "-in")
		restore.Stdin = strings.NewReader(string(oldData))
		_ = restore.Run()
	}

	return nil
}
