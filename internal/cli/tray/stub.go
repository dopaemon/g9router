//go:build !tray

package tray

import "fmt"

func Run(int, string, func()) error {
	return fmt.Errorf("system tray requires build tag tray")
}
