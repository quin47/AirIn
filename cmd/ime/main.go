package main

import (
	"airin/internal/audio"
	"airin/internal/config"
	"airin/internal/hotkey"
	"airin/internal/inject"
	"airin/internal/state"
	"airin/internal/asr"
	"airin/internal/tray"
	"context"
	"log"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"
)

func main() {
	log.SetFlags(log.LstdFlags | log.Lshortfile)
	log.Println("AirIn 启动中...")

	checkDeps()

	cfg, err := config.Load()
	if err != nil {
		log.Printf("加载配置失败: %v, 使用默认配置", err)
		cfg = config.Default()
	}

	sm := state.New()
	if cfg.IsConfigured() {
		sm.SetConfigured(true)
	}

	injector, err := inject.NewInjector()
	if err != nil {
		log.Printf("文本注入初始化失败: %v", err)
	}
	_ = injector

	engine := &recEngine{
		cfg:      cfg,
		sm:       sm,
		injector: injector,
	}

	trayMgr := tray.New(cfg, sm)
	trayMgr.SetCallbacks(engine.start, engine.stop)

	hkListener := hotkey.New(cfg, func() {
		log.Println("快捷键触发")
		trayMgr.TriggerStartStop()
	})

	if err := hkListener.Start(); err != nil {
		log.Printf("热键监听启动失败: %v", err)
	}
	defer hkListener.Stop()

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		<-sigCh
		log.Println("收到退出信号")
		engine.stop()
		trayMgr.Quit()
	}()

	if err := trayMgr.Run(); err != nil {
		log.Fatalf("托盘启动失败: %v", err)
	}
}

type recEngine struct {
	cfg      *config.Config
	sm       *state.StateMachine
	injector inject.Injector

	mu       sync.Mutex
	cancel   context.CancelFunc
	capture  audio.Capture
	asrCli   asr.Client
}

func (e *recEngine) start() {
	e.mu.Lock()
	defer e.mu.Unlock()

	if e.cancel != nil {
		log.Println("已在录音中")
		return
	}

	log.Println("===== 开始录音 =====")

	capture, err := audio.NewCapture(audio.DefaultConfig())
	if err != nil {
		log.Printf("音频采集初始化失败: %v", err)
		e.sm.Transition(state.Idle)
		return
	}
	e.capture = capture

	asrCfg := asr.DefaultConfig()
	asrCfg.Token = e.cfg.GetAPIKey()
	asrCfg.AppID = e.cfg.GetAppID()
	asrCfg.Cluster = e.cfg.GetCluster()
	asrCfg.AccessKey = e.cfg.GetAPIKey()
	asrCfg.SecretKey = e.cfg.GetSecretKey()
	if asrCfg.UID == "" {
		asrCfg.UID = "airin-user"
	}

	asrCli := asr.NewClient(asrCfg)
	e.asrCli = asrCli

	ctx, cancel := context.WithCancel(context.Background())
	e.cancel = cancel

	if err := asrCli.Connect(ctx); err != nil {
		log.Printf("ASR 连接失败: %v", err)
		cancel()
		e.asrCli = nil
		e.capture.Stop()
		e.capture = nil
		e.cancel = nil
		e.sm.Transition(state.Idle)
		return
	}

	audioStream, err := capture.Start()
	if err != nil {
		log.Printf("音频采集启动失败: %v", err)
		asrCli.Close()
		cancel()
		e.asrCli = nil
		e.capture = nil
		e.cancel = nil
		e.sm.Transition(state.Idle)
		return
	}

	firstResult := false

	var wg sync.WaitGroup

	wg.Add(1)
	go func() {
		defer wg.Done()
		for chunk := range audioStream {
			if err := asrCli.SendAudio(chunk); err != nil {
				log.Printf("发送音频失败: %v", err)
				return
			}
		}
		asrCli.SendEnd()
	}()

	wg.Add(1)
	go func() {
		defer wg.Done()
		for result := range asrCli.Results() {
			if !firstResult {
				firstResult = true
				e.sm.Transition(state.Transcribing)
			}
			if e.injector != nil {
				if err := e.injector.Inject(result.Text, result.Definite); err != nil {
					log.Printf("文本注入失败: %v", err)
				}
			} else {
				log.Printf("ASR: %q (definite=%v)", result.Text, result.Definite)
			}
		}
	}()

	go func() {
		for err := range asrCli.Errors() {
			log.Printf("ASR 错误: %v", err)
		}
	}()

	go func() {
		wg.Wait()
		log.Println("===== 录音流程结束 =====")
		if e.injector != nil {
			e.injector.Commit()
		}
		asrCli.Close()
		e.mu.Lock()
		e.cancel = nil
		e.capture = nil
		e.asrCli = nil
		if e.sm.Get() == state.Listening || e.sm.Get() == state.Transcribing {
			e.sm.Transition(state.Idle)
		}
		e.mu.Unlock()
	}()
}

func (e *recEngine) stop() {
	e.mu.Lock()
	defer e.mu.Unlock()

	if e.cancel == nil {
		return
	}

	log.Println("===== 停止录音 =====")

	// Force state to Idle first so the UI updates immediately
	if e.sm.Get() == state.Listening || e.sm.Get() == state.Transcribing {
		e.sm.Transition(state.Idle)
	}

	e.cancel()

	if e.capture != nil {
		e.capture.Stop()
	}

	if e.injector != nil {
		time.Sleep(100 * time.Millisecond)
		e.injector.Commit()
	}
}

func checkDeps() {
	log.Println("===== 依赖检查 =====")

	audioInfo := audio.CheckDeps()
	for _, exe := range audioInfo.Found {
		log.Printf("  [ok]   音频: %s", exe)
	}
	for _, exe := range audioInfo.Miss {
		log.Printf("  [miss] 音频: %s", exe)
	}

	injectInfo := inject.CheckDeps()
	for _, exe := range injectInfo.Found {
		log.Printf("  [ok]   注入: %s", exe)
	}
	for _, exe := range injectInfo.Miss {
		log.Printf("  [miss] 注入: %s", exe)
	}

	log.Println("======================")
}
