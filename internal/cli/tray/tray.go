//go:build tray

package tray

import (
	"fmt"
	"os/exec"
	"runtime"
	"strconv"

	"g9router/internal/cli/autostart"
	"github.com/getlantern/systray"
)

var fallbackIcon = []byte{
	0x89, 0x50, 0x4e, 0x47, 0x0d, 0x0a, 0x1a, 0x0a, 0x00, 0x00, 0x00, 0x0d,
	0x49, 0x48, 0x44, 0x52, 0x00, 0x00, 0x00, 0x10, 0x00, 0x00, 0x00, 0x10,
	0x08, 0x06, 0x00, 0x00, 0x00, 0x1f, 0xf3, 0xff, 0x61, 0x00, 0x00, 0x00,
	0x0d, 0x49, 0x44, 0x41, 0x54, 0x78, 0x9c, 0x63, 0xfc, 0xcf, 0xc0, 0xf0,
	0x1f, 0x08, 0x00, 0x00, 0x00, 0xff, 0xff, 0x03, 0x00, 0x00, 0x00, 0x00,
	0x01, 0x00, 0x01, 0x00, 0x00, 0x00, 0x00, 0x49, 0x45, 0x4e, 0x44, 0xae,
	0x42, 0x60, 0x82,
}

func Run(port int, executable string, quit func()) {
	systray.Run(func() {
		systray.SetIcon(fallbackIcon)
		systray.SetTitle("9Router")
		systray.SetTooltip("9Router - Port " + strconv.Itoa(port))
		open := systray.AddMenuItem("Open Dashboard", "Open 9Router dashboard")
		auto := systray.AddMenuItem("Auto-start", "Toggle auto-start")
		quitItem := systray.AddMenuItem("Quit", "Stop 9Router")
		if enabled, _ := autostart.Enabled(); enabled {
			auto.SetTitle("Auto-start enabled")
		}
		go func() {
			for {
				select {
				case <-open.ClickedCh:
					openDashboard(port)
				case <-auto.ClickedCh:
					if enabled, _ := autostart.Enabled(); enabled {
						_ = autostart.Disable()
						auto.SetTitle("Auto-start")
					} else if err := autostart.Enable(executable); err == nil {
						auto.SetTitle("Auto-start enabled")
					}
				case <-quitItem.ClickedCh:
					if quit != nil {
						quit()
					}
					systray.Quit()
					return
				}
			}
		}()
	}, nil)
}

func openDashboard(port int) {
	url := "http://localhost:" + strconv.Itoa(port) + "/dashboard"
	var command string
	switch runtime.GOOS {
	case "darwin":
		command = "open"
	case "windows":
		command = "rundll32"
		url = "url.dll,FileProtocolHandler " + url
	default:
		command = "xdg-open"
	}
	if err := exec.Command(command, url).Start(); err != nil {
		fmt.Println("dashboard available at", url)
	}
}
