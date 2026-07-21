package state

import (
	"fmt"
	"sync"
)

type Status int

const (
	Unconfigured Status = iota
	Idle
	Listening
	Transcribing
)

var statusNames = map[Status]string{
	Unconfigured: "待配置",
	Idle:         "待命中",
	Listening:    "录音中",
	Transcribing: "识别中",
}

func (s Status) String() string {
	if name, ok := statusNames[s]; ok {
		return name
	}
	return fmt.Sprintf("未知状态(%d)", s)
}

type StateMachine struct {
	status Status
	mu     sync.RWMutex

	listeners []func(old, new Status)
	muListen  sync.RWMutex
}

func New() *StateMachine {
	return &StateMachine{
		status: Unconfigured,
	}
}

func (sm *StateMachine) Get() Status {
	sm.mu.RLock()
	defer sm.mu.RUnlock()
	return sm.status
}

func (sm *StateMachine) Transition(new Status) error {
	sm.mu.Lock()
	old := sm.status

	if old == Unconfigured && new != Unconfigured {
		return fmt.Errorf("无法从未配置状态直接切换到 %s", new)
	}

	allowed := map[Status][]Status{
		Unconfigured: {Unconfigured},
		Idle:         {Idle, Listening, Unconfigured},
		Listening:    {Listening, Transcribing, Idle},
		Transcribing: {Transcribing, Idle},
	}

	for _, a := range allowed[old] {
		if a == new {
			sm.status = new
			sm.mu.Unlock()
			sm.notify(old, new)
			return nil
		}
	}

	sm.mu.Unlock()
	return fmt.Errorf("不允许的状态转换: %s -> %s", old, new)
}

func (sm *StateMachine) SetConfigured(configured bool) {
	sm.mu.Lock()
	old := sm.status
	if !configured {
		if old != Unconfigured {
			sm.status = Unconfigured
			sm.mu.Unlock()
			sm.notify(old, Unconfigured)
			return
		}
	} else {
		if old == Unconfigured {
			sm.status = Idle
			sm.mu.Unlock()
			sm.notify(old, Idle)
			return
		}
	}
	sm.mu.Unlock()
}

func (sm *StateMachine) OnChange(fn func(old, new Status)) {
	sm.muListen.Lock()
	defer sm.muListen.Unlock()
	sm.listeners = append(sm.listeners, fn)
}

func (sm *StateMachine) notify(old, new Status) {
	sm.muListen.RLock()
	defer sm.muListen.RUnlock()
	for _, fn := range sm.listeners {
		fn(old, new)
	}
}
