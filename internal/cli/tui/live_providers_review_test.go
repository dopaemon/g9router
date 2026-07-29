package tui

import (
	"errors"
	"io"
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"
)

func TestProviderReviewNavigationStates(t *testing.T) {
	model := &providerLiveModel{ui: &UI{width: 80}, tab: freeProviderTab}
	if got := model.itemCount(); got != 0 {
		t.Fatalf("empty Free provider count = %d, want no actionable rows", got)
	}
	if got := model.cardContent(); strings.Contains(got, "›") {
		t.Fatalf("empty Free provider has focus marker: %q", got)
	}
	if got := providerMenuItem(9, "model", false); !strings.Contains(got, "10") {
		t.Fatalf("tenth provider row = %q", got)
	}
	if got := providerMenuItem(0, "model", true); !strings.Contains(got, "›") {
		t.Fatalf("focused provider row = %q", got)
	}
}

func TestProviderRowsFitNarrowWidth(t *testing.T) {
	for width := 8; width <= 40; width++ {
		ui := &UI{width: width}
		for _, selected := range []bool{false, true} {
			row := fitProviderMenuItem(0, "provider with a long name", selected, ui.innerWidth())
			if got := lipgloss.Width(row); got > ui.innerWidth() {
				t.Fatalf("width %d selected=%t row width=%d, available=%d", width, selected, got, ui.innerWidth())
			}
		}
	}
}

func TestProviderRefreshErrorKeepsLastData(t *testing.T) {
	item := provider{ID: "kept", Name: "Kept"}
	model := &providerLiveModel{custom: []provider{item}, ui: &UI{width: 80}}
	model.Update(providerDataMsg{err: errors.New("offline")})
	if len(model.custom) != 1 || model.custom[0].ID != item.ID {
		t.Fatalf("refresh error replaced provider data: %#v", model.custom)
	}
}

func TestProviderActionClearsPreviousTestRetry(t *testing.T) {
	model := &providerLiveModel{lastAction: "test"}
	_ = model.action(func(_ io.Reader, _ io.Writer) (string, error) { return "", nil })
	if model.lastAction != "" {
		t.Fatalf("non-test action kept retry target %q", model.lastAction)
	}
}

func TestProvidersViewFitsNarrowTerminal(t *testing.T) {
	for width := 8; width <= 40; width++ {
		model := &providerLiveModel{ui: &UI{width: width, height: 40}, loading: true}
		view := sizedModel{ui: model.ui, model: model}.View()
		for _, line := range strings.Split(view, "\n") {
			if got := lipgloss.Width(line); got > width {
				t.Fatalf("terminal width %d rendered line width %d: %q", width, got, line)
			}
		}
	}
}
