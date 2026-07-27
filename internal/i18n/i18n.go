package i18n

import "strings"

const (
	English    = "en"
	Vietnamese = "vi"
)

var translations = map[string]map[string]string{
	English: {
		"menu.endpoint": "Endpoint & Key", "menu.providers": "Providers", "menu.combos": "Combos", "menu.statistics": "Statistics", "menu.cliTools": "CLI Tools", "menu.settings": "Settings", "menu.language": "Language", "menu.exit": "Exit", "menu.title": "9Router CLI", "language.title": "Choose language", "language.english": "English", "language.vietnamese": "Tiếng Việt", "endpoint.title": "Endpoint & Key", "endpoint.card": "API Endpoint", "endpoint.local": "Local", "endpoint.tunnel": "Tunnel", "endpoint.tailscale": "Tailscale", "keys.card": "API Keys", "keys.none": "No API keys found.", "keys.selectShow": "Press v to select an API, then show it", "keys.created": "API key created. Press v to show it.", "keys.tunnelToggle": "Toggle Tunnel", "keys.tunnelUpdated": "Tunnel updated", "keys.tunnelUnchanged": "Tunnel unchanged", "keys.tailscaleToggle": "Toggle Tailscale", "keys.tailscaleUpdated": "Tailscale updated", "keys.tailscaleUnchanged": "Tailscale unchanged", "keys.toggle": "On/Off", "keys.toggleUpdated": "API key status updated", "keys.create": "Create API key", "keys.rename": "Rename", "keys.delete": "Delete", "keys.show": "Show", "keys.back": "Back", "keys.autoRefresh": "Auto refresh: 2s", "common.on": "ON", "common.off": "OFF", "common.notInstalled": "not installed", "common.error": "Error", "common.back": "Back", "common.save": "Save", "common.cancel": "Cancel", "form.apiName": "API name", "form.finish": "Finish", "form.reveal": "Reveal this API key in terminal?", "form.delete": "Delete this API key?", "form.disableTunnel": "Disable Tunnel?", "form.disableTailscale": "Disable Tailscale?", "form.chooseKey": "Choose API key",
	},
	Vietnamese: {
		"menu.endpoint":           "Endpoint & Key",
		"menu.providers":          "Nhà cung cấp",
		"menu.combos":             "Combo",
		"menu.statistics":         "Thống kê",
		"menu.cliTools":           "Công cụ CLI",
		"menu.settings":           "Cài đặt",
		"menu.language":           "Ngôn ngữ",
		"menu.exit":               "Thoát",
		"menu.title":              "9Router CLI",
		"language.title":          "Chọn ngôn ngữ",
		"language.english":        "English",
		"language.vietnamese":     "Tiếng Việt",
		"endpoint.title":          "Endpoint & Key",
		"endpoint.card":           "API Endpoint",
		"endpoint.local":          "Local",
		"endpoint.tunnel":         "Tunnel",
		"endpoint.tailscale":      "Tailscale",
		"keys.card":               "API Keys",
		"keys.none":               "Chưa có API key.",
		"keys.selectShow":         "Nhấn v để chọn API rồi hiển thị",
		"keys.created":            "Đã tạo API key. Nhấn v để hiển thị.",
		"keys.tunnelToggle":       "Bật/tắt Tunnel",
		"keys.tunnelUpdated":      "Đã cập nhật Tunnel",
		"keys.tunnelUnchanged":    "Tunnel không thay đổi",
		"keys.tailscaleToggle":    "Bật/tắt Tailscale",
		"keys.tailscaleUpdated":   "Đã cập nhật Tailscale",
		"keys.tailscaleUnchanged": "Tailscale không thay đổi",
		"keys.toggle":             "Bật/tắt",
		"keys.toggleUpdated":      "Đã cập nhật trạng thái API key",
		"keys.create":             "Tạo API key",
		"keys.rename":             "Đổi tên",
		"keys.delete":             "Xóa",
		"keys.show":               "Hiển thị",
		"keys.back":               "Quay lại",
		"keys.autoRefresh":        "Tự động làm mới: 2 giây",
		"common.on":               "BẬT",
		"common.off":              "TẮT",
		"common.notInstalled":     "chưa cài",
		"common.error":            "Lỗi",
		"common.back":             "Quay lại",
		"common.save":             "Lưu",
		"common.cancel":           "Hủy",
		"form.apiName":            "Tên API",
		"form.finish":             "Hoàn tất",
		"form.reveal":             "Hiển thị API key trong terminal?",
		"form.delete":             "Xóa API key này?",
		"form.disableTunnel":      "Tắt Tunnel?",
		"form.disableTailscale":   "Tắt Tailscale?",
		"form.chooseKey":          "Chọn API key",
	},
}

func init() {
	translations[English]["keys.controls"] = "Controls"
	translations[Vietnamese]["keys.controls"] = "Điều khiển"
}

func Normalize(locale string) string {
	if strings.EqualFold(strings.TrimSpace(locale), Vietnamese) {
		return Vietnamese
	}
	return English
}

func T(locale, key string) string {
	locale = Normalize(locale)
	if value := translations[locale][key]; value != "" {
		return value
	}
	return key
}
