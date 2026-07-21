package audio

import (
	"fmt"
)

type Device struct {
	Name     string
	MaxChannels int
}

type Capture interface {
	Start() (<-chan []int16, error)
	Stop() error
	Devices() ([]Device, error)
}

type Config struct {
	SampleRate int
	Channels   int
	BitDepth   int
	DeviceName string
}

func DefaultConfig() Config {
	return Config{
		SampleRate: 16000,
		Channels:   1,
		BitDepth:   16,
	}
}

func (c Config) FrameSize() int {
	return c.Channels * c.BitDepth / 8
}

func (c Config) Validate() error {
	if c.SampleRate <= 0 {
		return fmt.Errorf("invalid sample rate: %d", c.SampleRate)
	}
	if c.Channels <= 0 {
		return fmt.Errorf("invalid channels: %d", c.Channels)
	}
	if c.BitDepth != 16 {
		return fmt.Errorf("unsupported bit depth: %d (only 16-bit supported)", c.BitDepth)
	}
	return nil
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
