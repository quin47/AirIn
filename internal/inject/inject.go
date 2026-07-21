package inject

import (
	"log"
	"strings"
	"sync"
)

type Injector interface {
	Inject(text string, definite bool) error
	Commit() error
	Cancel() error
}

const backspaceRepeat = 20

type baseInjector struct {
	lastText  string
	mu        sync.Mutex
}

func (b *baseInjector) handleStream(text string, definite bool, doType func(string) error, doBackspace func(int) error) error {
	b.mu.Lock()
	defer b.mu.Unlock()

	if definite {
		if text != "" && text != b.lastText {
			if b.lastText != "" {
				bs := countRunes(b.lastText)
				if err := doBackspace(bs); err != nil {
					return err
				}
			}
			if err := doType(text); err != nil {
				return err
			}
		}
		b.lastText = ""
		return nil
	}

	if text == b.lastText {
		return nil
	}

	delta, isSubset := computeDelta(b.lastText, text)
	if isSubset {
		if err := doType(delta); err != nil {
			return err
		}
	} else {
		if b.lastText != "" {
			bs := countRunes(b.lastText)
			if err := doBackspace(bs); err != nil {
				return err
			}
		}
		if err := doType(text); err != nil {
			return err
		}
	}

	b.lastText = text
	return nil
}

func countRunes(s string) int {
	n := 0
	for range s {
		n++
	}
	return n
}

func computeDelta(old, new string) (string, bool) {
	if old == "" {
		return new, true
	}
	if strings.HasPrefix(new, old) {
		return new[len(old):], true
	}

	commonLen := 0
	oldRunes := []rune(old)
	newRunes := []rune(new)
	for i := 0; i < len(oldRunes) && i < len(newRunes); i++ {
		if oldRunes[i] == newRunes[i] {
			commonLen++
		} else {
			break
		}
	}

	if commonLen > 0 {
		_ = string(newRunes[commonLen:])
		return "", false
	}

	return "", false
}

type DepInfo struct {
	Found []string
	Miss  []string
}

var checkFunc func() DepInfo

func CheckDeps() DepInfo {
	if checkFunc == nil {
		return DepInfo{Miss: []string{"(unsupported platform)"}}
	}
	return checkFunc()
}

type noopInjector struct{}

func (n *noopInjector) Inject(text string, definite bool) error {
	log.Printf("inject (noop): text=%q definite=%v", text, definite)
	return nil
}

func (n *noopInjector) Commit() error  { return nil }
func (n *noopInjector) Cancel() error  { return nil }
