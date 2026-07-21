package main

import (
	"airin/internal/config"
	"airin/internal/hotkey"
	"airin/internal/state"
	"airin/internal/tray"
	"log"
	"os"
	"os/signal"
	"syscall"
)

func main() {
	log.SetFlags(log.LstdFlags | log.Lshortfile)
	log.Println("AirIn 启动中...")

	cfg, err := config.Load()
	if err != nil {
		log.Printf("加载配置失败: %v, 使用默认配置", err)
		cfg = config.Default()
	}

	sm := state.New()

	if cfg.IsConfigured() {
		sm.SetConfigured(true)
	} else {
		sm.SetConfigured(false)
	}

	trayMgr := tray.New(cfg, sm)

	trayMgr.SetCallbacks(
		func() { log.Println("开始录音") },
		func() { log.Println("停止录音") },
	)

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
		trayMgr.Quit()
	}()

	if err := trayMgr.Run(); err != nil {
		log.Fatalf("托盘启动失败: %v", err)
	}
}
