package tray

import (
	"fmt"
	"airin/internal/config"
	"airin/internal/state"
	"log"

	"github.com/godbus/dbus/v5"
	"github.com/godbus/dbus/v5/introspect"
	"github.com/godbus/dbus/v5/prop"
)

const (
	sniWatcherPath  = "/org/kde/StatusNotifierWatcher"
	sniWatcherIface = "org.kde.StatusNotifierWatcher"
	sniItemIface    = "org.kde.StatusNotifierItem"
	sniItemPath     = "/StatusNotifierItem"
)

type Manager struct {
	cfg     *config.Config
	sm      *state.StateMachine
	onStart func()
	onStop  func()
	conn    *dbus.Conn
	props   *prop.Properties
	quitCh  chan struct{}
}

func New(cfg *config.Config, sm *state.StateMachine) *Manager {
	return &Manager{
		cfg:    cfg,
		sm:     sm,
		quitCh: make(chan struct{}),
	}
}

func (m *Manager) SetCallbacks(onStart, onStop func()) {
	m.onStart = onStart
	m.onStop = onStop
}

func (m *Manager) Run() error {
	var err error
	m.conn, err = dbus.ConnectSessionBus()
	if err != nil {
		return fmt.Errorf("connect session bus: %w", err)
	}

	reply, err := m.conn.RequestName("org.kde.StatusNotifierItem-airin", dbus.NameFlagDoNotQueue)
	if err != nil || reply != dbus.RequestNameReplyPrimaryOwner {
		m.conn.Close()
		return fmt.Errorf("request name: %v / %v", err, reply)
	}

	m.setupProps()

	if err := m.conn.Export(m, sniItemPath, sniItemIface); err != nil {
		m.conn.Close()
		return fmt.Errorf("export SNI: %w", err)
	}

	if err := m.conn.Export(introspect.NewIntrospectable(m.introspect()), sniItemPath,
		"org.freedesktop.DBus.Introspectable"); err != nil {
		m.conn.Close()
		return fmt.Errorf("export introspect: %w", err)
	}

	m.registerWatcher()

	m.sm.OnChange(func(old, new state.Status) {
		m.refreshProps()
	})

	log.Println("tray: SNI 托盘已注册")
	<-m.quitCh
	return nil
}

func (m *Manager) setupProps() {
	iconData := generateSNIIconPixmap()

	m.props = prop.New(m.conn, sniItemPath, map[string]map[string]*prop.Prop{
		sniItemIface: {
			"Category":          {Value: "ApplicationStatus", Writable: false, Emit: prop.EmitTrue, Callback: nil},
			"Id":                {Value: "airin", Writable: false, Emit: prop.EmitTrue, Callback: nil},
			"Title":             {Value: "AirIn", Writable: false, Emit: prop.EmitTrue, Callback: nil},
			"Status":            {Value: "Active", Writable: false, Emit: prop.EmitTrue, Callback: nil},
			"WindowId":          {Value: int32(0), Writable: false, Emit: prop.EmitTrue, Callback: nil},
			"IconName":          {Value: "", Writable: false, Emit: prop.EmitTrue, Callback: nil},
			"IconPixmap":        {Value: iconData, Writable: false, Emit: prop.EmitTrue, Callback: nil},
			"OverlayIconName":   {Value: "", Writable: false, Emit: prop.EmitTrue, Callback: nil},
			"AttentionIconName": {Value: "", Writable: false, Emit: prop.EmitTrue, Callback: nil},
			"ToolTip":           {Value: buildTooltip("待配置"), Writable: false, Emit: prop.EmitTrue, Callback: nil},
			"ItemIsMenu":        {Value: false, Writable: false, Emit: prop.EmitTrue, Callback: nil},
		},
	})
}

func (m *Manager) refreshProps() {
	s := m.sm.Get()
	statusStr := "Passive"
	if s == state.Idle || s == state.Listening || s == state.Transcribing {
		statusStr = "Active"
	}
	m.props.SetMust(sniItemIface, "Status", dbus.MakeVariant(statusStr))
	m.props.SetMust(sniItemIface, "Title", dbus.MakeVariant("AirIn - "+s.String()))
	m.props.SetMust(sniItemIface, "ToolTip", dbus.MakeVariant(buildTooltip(s.String())))
}

func (m *Manager) registerWatcher() {
	obj := m.conn.Object(sniWatcherIface, sniWatcherPath)
	obj.Call("RegisterStatusNotifierItem", 0, "org.kde.StatusNotifierItem-airin")
}

func (m *Manager) introspect() *introspect.Node {
	return &introspect.Node{
		Name: sniItemPath,
		Interfaces: []introspect.Interface{
			introspect.IntrospectData,
			prop.IntrospectData,
			{
				Name: sniItemIface,
				Methods: []introspect.Method{
					{
						Name: "Activate",
						Args: []introspect.Arg{
							{Name: "x", Direction: "in", Type: "i"},
							{Name: "y", Direction: "in", Type: "i"},
						},
					},
					{
						Name: "SecondaryActivate",
						Args: []introspect.Arg{
							{Name: "x", Direction: "in", Type: "i"},
							{Name: "y", Direction: "in", Type: "i"},
						},
					},
					{
						Name: "ContextMenu",
						Args: []introspect.Arg{
							{Name: "x", Direction: "in", Type: "i"},
							{Name: "y", Direction: "in", Type: "i"},
							{Name: "menu", Direction: "out", Type: "o"},
						},
					},
					{
						Name: "Scroll",
						Args: []introspect.Arg{
							{Name: "delta", Direction: "in", Type: "i"},
							{Name: "orientation", Direction: "in", Type: "s"},
						},
					},
				},
				Properties: []introspect.Property{
					{Name: "Category", Type: "s", Access: "read"},
					{Name: "Id", Type: "s", Access: "read"},
					{Name: "Title", Type: "s", Access: "read"},
					{Name: "Status", Type: "s", Access: "read"},
					{Name: "WindowId", Type: "i", Access: "read"},
					{Name: "IconName", Type: "s", Access: "read"},
					{Name: "IconPixmap", Type: "a(iiay)", Access: "read"},
					{Name: "OverlayIconName", Type: "s", Access: "read"},
					{Name: "AttentionIconName", Type: "s", Access: "read"},
					{Name: "ToolTip", Type: "(sa(iiay)ss)", Access: "read"},
					{Name: "ItemIsMenu", Type: "b", Access: "read"},
				},
			},
		},
	}
}

func (m *Manager) Activate(x, y int32) *dbus.Error {
	log.Printf("tray: Activate (%d, %d)", x, y)
	go m.TriggerStartStop()
	return nil
}

func (m *Manager) SecondaryActivate(x, y int32) *dbus.Error {
	log.Printf("tray: SecondaryActivate (%d, %d)", x, y)
	go m.showConfigInfo()
	return nil
}

func (m *Manager) ContextMenu(x, y int32) (dbus.ObjectPath, *dbus.Error) {
	return "/no/menu", nil
}

func (m *Manager) Scroll(delta int32, orientation string) *dbus.Error {
	return nil
}

func (m *Manager) TriggerStartStop() {
	s := m.sm.Get()
	switch s {
	case state.Unconfigured:
		log.Println("未配置 API Key, 无法开始录音")
	case state.Idle:
		if m.onStart != nil {
			if err := m.sm.Transition(state.Listening); err != nil {
				log.Printf("状态转换失败: %v", err)
				return
			}
			m.onStart()
		}
	case state.Listening, state.Transcribing:
		if m.onStop != nil {
			m.sm.Transition(state.Idle)
			m.onStop()
		}
	}
}

func (m *Manager) Quit() {
	close(m.quitCh)
	if m.conn != nil {
		m.conn.Close()
	}
}

func (m *Manager) showConfigInfo() {
	fmt.Println()
	fmt.Println("========== AirIn 配置 ==========")
	fmt.Printf("  API Key: %s\n", maskKey(m.cfg.GetAPIKey()))
	fmt.Printf("  快捷键:  %s\n", m.cfg.HotkeyString())
	fmt.Printf("  状态:    %s\n", m.sm.Get().String())
	fmt.Println()
	fmt.Println("  配置文件: ~/.config/ime/config.json")
	fmt.Println("  格式示例:")
	fmt.Println(`  {
    "api_key": "your-api-key-here",
    "hotkey": {
      "modifiers": ["ctrl", "shift"],
      "key": "v"
    }
  }`)
	fmt.Println("=====================================")
	fmt.Println()
}

func maskKey(key string) string {
	if key == "" {
		return "(未设置)"
	}
	if len(key) <= 8 {
		return "***"
	}
	return key[:4] + "****" + key[len(key)-4:]
}
