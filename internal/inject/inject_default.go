//go:build !linux && !darwin

package inject

import "log"

func NewInjector() (Injector, error) {
	log.Println("inject: 当前平台未实现，使用 noop")
	return &noopInjector{}, nil
}

func init() {
	checkFunc = func() DepInfo {
		return DepInfo{Miss: []string{"(unsupported platform)"}}
	}
}
