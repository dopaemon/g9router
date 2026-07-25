package autostart

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
)

const appName = "9router"

func Enable(executable string) error {
	home, err := os.UserHomeDir()
	if err != nil {
		return err
	}
	switch runtime.GOOS {
	case "linux":
		path := filepath.Join(home, ".config", "autostart", appName+".desktop")
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			return err
		}
		return os.WriteFile(path, []byte(fmt.Sprintf("[Desktop Entry]\nType=Application\nName=9Router\nExec=%s --tray --skip-update\nTerminal=false\nX-GNOME-Autostart-enabled=true\n", executable)), 0o644)
	case "darwin":
		path := filepath.Join(home, "Library", "LaunchAgents", "com.9router.autostart.plist")
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			return err
		}
		content := fmt.Sprintf("<?xml version=\"1.0\" encoding=\"UTF-8\"?><plist version=\"1.0\"><dict><key>Label</key><string>com.9router.autostart</string><key>ProgramArguments</key><array><string>%s</string><string>--tray</string><string>--skip-update</string></array><key>RunAtLoad</key><true/></dict></plist>", executable)
		return os.WriteFile(path, []byte(content), 0o644)
	case "windows":
		path := filepath.Join(os.Getenv("APPDATA"), "Microsoft", "Windows", "Start Menu", "Programs", "Startup", "9router.vbs")
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			return err
		}
		content := fmt.Sprintf("CreateObject(\"WScript.Shell\").Run \"\"\"%s\" --tray --skip-update\"\", 0, False\n", executable)
		return os.WriteFile(path, []byte(content), 0o644)
	default:
		return fmt.Errorf("autostart unsupported on %s", runtime.GOOS)
	}
}

func Disable() error {
	path, err := configPath()
	if err != nil {
		return err
	}
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}

func Enabled() (bool, error) {
	path, err := configPath()
	if err != nil {
		return false, err
	}
	_, err = os.Stat(path)
	if os.IsNotExist(err) {
		return false, nil
	}
	return err == nil, err
}

func configPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	switch runtime.GOOS {
	case "linux":
		return filepath.Join(home, ".config", "autostart", appName+".desktop"), nil
	case "darwin":
		return filepath.Join(home, "Library", "LaunchAgents", "com.9router.autostart.plist"), nil
	case "windows":
		return filepath.Join(os.Getenv("APPDATA"), "Microsoft", "Windows", "Start Menu", "Programs", "Startup", appName+".vbs"), nil
	default:
		return "", fmt.Errorf("autostart unsupported on %s", runtime.GOOS)
	}
}
