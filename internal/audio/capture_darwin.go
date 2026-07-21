//go:build darwin

package audio

import (
	"encoding/binary"
	"fmt"
	"io"
	"log"
	"os/exec"
	"sync"
	"sync/atomic"
)

type capture struct {
	cfg     Config
	cmd     *exec.Cmd
	stdout  io.ReadCloser
	stopCh  chan struct{}
	wg      sync.WaitGroup
	running atomic.Bool
}

func NewCapture(cfg Config) (Capture, error) {
	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	return &capture{cfg: cfg, stopCh: make(chan struct{})}, nil
}

func findRecordCmd() (string, []string) {
	if _, err := exec.LookPath("ffmpeg"); err == nil {
		return "ffmpeg", []string{
			"-f", "avfoundation",
			"-i", ":0",
			"-acodec", "pcm_s16le",
			"-ar", "16000",
			"-ac", "1",
			"-f", "s16le",
			"-loglevel", "quiet",
			"pipe:1",
		}
	}
	if _, err := exec.LookPath("sox"); err == nil {
		return "sox", []string{
			"-d",
			"-r", "16000",
			"-c", "1",
			"-b", "16",
			"-e", "signed-integer",
			"-t", "raw",
			"-q",
			"-",
		}
	}
	return "", nil
}

func init() {
	checkFunc = func() DepInfo {
		var info DepInfo
		for _, exe := range []string{"ffmpeg", "sox"} {
			if _, err := exec.LookPath(exe); err == nil {
				info.Found = append(info.Found, exe)
			} else {
				info.Miss = append(info.Miss, exe)
			}
		}
		return info
	}
}

func (c *capture) Devices() ([]Device, error) {
	return []Device{{Name: "default", MaxChannels: 1}}, nil
}

func (c *capture) Start() (<-chan []int16, error) {
	if c.running.Swap(true) {
		return nil, fmt.Errorf("capture already running")
	}

	exe, args := findRecordCmd()
	if exe == "" {
		return nil, fmt.Errorf("no audio capture tool found (tried: ffmpeg, sox); install via brew")
	}

	c.cmd = exec.Command(exe, args...)
	var err error
	c.stdout, err = c.cmd.StdoutPipe()
	if err != nil {
		c.running.Store(false)
		return nil, fmt.Errorf("stdout pipe: %w", err)
	}

	if err := c.cmd.Start(); err != nil {
		c.running.Store(false)
		return nil, fmt.Errorf("start %s: %w", exe, err)
	}

	sampleRate := c.cfg.SampleRate
	channels := c.cfg.Channels
	chunkSize := sampleRate * channels / 10

	out := make(chan []int16, 8)

	c.wg.Add(1)
	go func() {
		defer c.wg.Done()
		defer close(out)

		log.Printf("audio: 录音已启动 (%s, %dHz, %dch)", exe, sampleRate, channels)

		buf := make([]byte, chunkSize*2)

		for {
			select {
			case <-c.stopCh:
				return
			default:
			}

			n, err := io.ReadFull(c.stdout, buf)
			if err != nil {
				if err != io.EOF && err != io.ErrUnexpectedEOF {
					log.Printf("audio: 读取错误: %v", err)
				}
				return
			}

			samples := make([]int16, n/2)
			for i := range samples {
				samples[i] = int16(binary.LittleEndian.Uint16(buf[i*2:]))
			}

			select {
			case out <- samples:
			case <-c.stopCh:
				return
			}
		}
	}()

	return out, nil
}

func (c *capture) Stop() error {
	if !c.running.Swap(false) {
		return nil
	}

	close(c.stopCh)

	if c.cmd != nil && c.cmd.Process != nil {
		c.cmd.Process.Kill()
	}
	c.wg.Wait()

	if c.stdout != nil {
		c.stdout.Close()
	}

	log.Println("audio: 录音已停止")
	return nil
}
