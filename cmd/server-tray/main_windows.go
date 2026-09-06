//go:build windows

package main

import (
	"context"
	_ "embed"
	"fmt"
	"sync"
	"time"

	"bb_erp_echo/internal/servertray"

	"github.com/getlantern/systray"
)

//go:embed icon.ico
var trayIcon []byte

func main() {
	release, acquired, err := servertray.AcquireSingleInstance()
	if err != nil {
		servertray.ShowInfo("博邦服务无法建立单实例保护，请查看启动日志。")
		_ = servertray.WriteBootstrapError(servertray.InitialInfo().LogDir, err)
		return
	}
	if !acquired {
		servertray.ShowInfo("博邦服务已经在运行。")
		return
	}
	defer release()

	application := &trayApplication{stopped: make(chan struct{})}
	systray.Run(application.ready, application.exit)
}

type trayApplication struct {
	mu            sync.Mutex
	controller    *servertray.Controller
	info          servertray.ServiceInfo
	addresses     []string
	addressItems  []*addressMenuItem
	serviceUsable bool

	statusItem    *systray.MenuItem
	versionItem   *systray.MenuItem
	addressesRoot *systray.MenuItem
	openItem      *systray.MenuItem
	checkItem     *systray.MenuItem
	restartItem   *systray.MenuItem
	logItem       *systray.MenuItem
	directoryItem *systray.MenuItem
	exitItem      *systray.MenuItem

	stopOnce sync.Once
	stopped  chan struct{}
}

type addressMenuItem struct {
	item *systray.MenuItem
	url  string
}

func (a *trayApplication) ready() {
	systray.SetIcon(trayIcon)
	systray.SetTooltip("博邦 ERP · 正在启动")
	a.info = servertray.InitialInfo()
	a.statusItem = systray.AddMenuItem("● 博邦服务正在启动", "当前服务状态")
	a.statusItem.Disable()
	a.versionItem = systray.AddMenuItem("版本："+a.info.Version, "服务端版本")
	a.versionItem.Disable()
	a.addressesRoot = systray.AddMenuItem("访问地址", "当前局域网访问地址")
	a.addAddressItem()
	a.addressesRoot.Disable()
	systray.AddSeparator()
	a.openItem = systray.AddMenuItem("打开管理页面", "使用默认浏览器打开管理页面")
	a.checkItem = systray.AddMenuItem("检查服务状态", "检查 /ready 就绪状态")
	a.restartItem = systray.AddMenuItem("重启服务", "优雅重启服务端")
	systray.AddSeparator()
	a.logItem = systray.AddMenuItem("打开日志目录", "打开服务端日志目录")
	a.directoryItem = systray.AddMenuItem("打开服务目录", "打开服务端程序所在目录")
	systray.AddSeparator()
	a.exitItem = systray.AddMenuItem("退出博邦服务", "关闭服务端并退出")

	a.controller = servertray.NewController(nil, nil, a.info, a.handleEvent)
	a.bindActions()
	a.refreshAddresses()
	go func() { _ = a.controller.Start() }()
	go a.watchNetwork()
}

func (a *trayApplication) exit() {
	a.shutdown()
}

func (a *trayApplication) shutdown() {
	a.stopOnce.Do(func() {
		close(a.stopped)
		if a.controller != nil {
			ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
			_ = a.controller.Stop(ctx)
			cancel()
		}
	})
}

func (a *trayApplication) bindActions() {
	go func() {
		for range a.openItem.ClickedCh {
			_ = servertray.OpenTarget(a.defaultURL())
		}
	}()
	go func() {
		for range a.checkItem.ClickedCh {
			ctx, cancel := context.WithTimeout(context.Background(), 4*time.Second)
			err := servertray.WaitReady(ctx, a.currentInfo().Port)
			cancel()
			if err != nil {
				_ = servertray.ShowBalloon("博邦 ERP", "服务不可用，请查看日志。", true)
			} else {
				_ = servertray.ShowBalloon("博邦 ERP", "博邦服务运行正常。", false)
			}
		}
	}()
	go func() {
		for range a.restartItem.ClickedCh {
			go func() { _ = a.controller.Restart() }()
		}
	}()
	go func() {
		for range a.logItem.ClickedCh {
			_ = servertray.OpenTarget(a.currentInfo().LogDir)
		}
	}()
	go func() {
		for range a.directoryItem.ClickedCh {
			_ = servertray.OpenTarget(a.currentInfo().ServiceDir)
		}
	}()
	go func() {
		for range a.exitItem.ClickedCh {
			if !servertray.ConfirmExit() {
				continue
			}
			a.exitItem.Disable()
			go func() {
				a.shutdown()
				systray.Quit()
			}()
		}
	}()
}

func (a *trayApplication) handleEvent(event servertray.Event) {
	a.mu.Lock()
	a.info = event.Info
	a.versionItem.SetTitle("版本：" + event.Info.Version)
	a.mu.Unlock()
	switch event.State {
	case servertray.StateStarting:
		a.setState("● 博邦服务正在启动", "正在启动", false, false)
	case servertray.StateRunning:
		a.setState("● 博邦服务运行中", "运行中", true, true)
		a.refreshAddresses()
	case servertray.StateRestarting:
		a.setState("● 博邦服务正在重启", "正在重启", false, false)
	case servertray.StateFailed:
		a.setState("● 博邦服务启动失败", "启动失败", false, true)
	case servertray.StateStopping:
		a.setState("● 博邦服务正在退出", "正在退出", false, false)
	}
	if event.Err != nil {
		_ = servertray.WriteBootstrapError(event.Info.LogDir, event.Err)
	}
	switch event.Notice {
	case servertray.NoticeStarted:
		_ = servertray.ShowBalloon("博邦 ERP", "博邦服务已启用", false)
	case servertray.NoticeRestarted:
		_ = servertray.ShowBalloon("博邦 ERP", "博邦服务已重新启动", false)
	case servertray.NoticeFailed:
		_ = servertray.ShowBalloon("博邦 ERP", "博邦服务启动失败，请查看日志。", true)
	}
}

func (a *trayApplication) setState(title, tooltip string, usable, restartable bool) {
	a.mu.Lock()
	a.serviceUsable = usable
	a.mu.Unlock()
	a.statusItem.SetTitle(title)
	systray.SetTooltip("博邦 ERP · " + tooltip)
	setEnabled(a.addressesRoot, usable)
	setEnabled(a.openItem, usable)
	setEnabled(a.checkItem, usable)
	setEnabled(a.restartItem, restartable)
}

func setEnabled(item *systray.MenuItem, enabled bool) {
	if enabled {
		item.Enable()
	} else {
		item.Disable()
	}
}

func (a *trayApplication) watchNetwork() {
	ticker := time.NewTicker(15 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			a.refreshAddresses()
		case <-a.stopped:
			return
		}
	}
}

func (a *trayApplication) refreshAddresses() {
	addresses, err := servertray.PrivateIPv4()
	if err != nil {
		addresses = nil
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	a.addresses = addresses
	port := a.info.Port
	usable := a.serviceUsable
	needed := len(addresses)
	if needed == 0 {
		needed = 1
	}
	for len(a.addressItems) < needed {
		a.addAddressItemLocked()
	}
	for index, entry := range a.addressItems {
		if index < len(addresses) {
			entry.url = fmt.Sprintf("http://%s:%d", addresses[index], port)
			entry.item.SetTitle(entry.url)
			setEnabled(entry.item, usable)
			entry.item.Show()
		} else if index == 0 && len(addresses) == 0 {
			entry.url = ""
			entry.item.SetTitle("未检测到局域网地址")
			entry.item.Disable()
			entry.item.Show()
		} else {
			entry.url = ""
			entry.item.Hide()
		}
	}
}

func (a *trayApplication) addAddressItem() {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.addAddressItemLocked()
}

func (a *trayApplication) addAddressItemLocked() {
	entry := &addressMenuItem{item: a.addressesRoot.AddSubMenuItem("未检测到局域网地址", "打开该地址")}
	entry.item.Disable()
	a.addressItems = append(a.addressItems, entry)
	go func() {
		for range entry.item.ClickedCh {
			a.mu.Lock()
			url := entry.url
			a.mu.Unlock()
			if url != "" {
				_ = servertray.OpenTarget(url)
			}
		}
	}()
}

func (a *trayApplication) defaultURL() string {
	a.mu.Lock()
	defer a.mu.Unlock()
	if len(a.addresses) > 0 {
		return fmt.Sprintf("http://%s:%d", a.addresses[0], a.info.Port)
	}
	return fmt.Sprintf("http://127.0.0.1:%d", a.info.Port)
}

func (a *trayApplication) currentInfo() servertray.ServiceInfo {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.info
}
