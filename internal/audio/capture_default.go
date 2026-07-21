//go:build !linux && !darwin

package audio

import "fmt"

type capture struct{}

func NewCapture(cfg Config) (Capture, error) {
	return nil, fmt.Errorf("audio capture not implemented on this platform")
}

func (c *capture) Devices() ([]Device, error) {
	return nil, fmt.Errorf("audio capture not implemented on this platform")
}

func (c *capture) Start() (<-chan []int16, error) {
	return nil, fmt.Errorf("audio capture not implemented on this platform")
}

func (c *capture) Stop() error {
	return fmt.Errorf("audio capture not implemented on this platform")
}

func init() {
	checkFunc = func() DepInfo {
		return DepInfo{Miss: []string{"(unsupported platform)"}}
	}
}
