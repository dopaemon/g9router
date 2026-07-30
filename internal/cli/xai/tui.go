package xai

import (
	"context"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

type videoForm struct {
	inputs [3]textinput.Model
	cursor int
	finish int
	err    error
	cancel bool
	output io.Writer
}

func RunTTY(ctx context.Context, out io.Writer) int {
	if ctx.Err() != nil {
		return 1
	}
	model := newVideoForm(out)
	result, err := tea.NewProgram(model, tea.WithInput(os.Stdin), tea.WithOutput(out)).Run()
	if err != nil {
		fmt.Fprintln(out, err)
		return 1
	}
	form := result.(*videoForm)
	if form.cancel {
		return 0
	}
	return Run(ctx, []string{"--prompt", form.inputs[0].Value(), "--output", form.inputs[1].Value(), "--model", form.inputs[2].Value()}, out, out)
}

func newVideoForm(output io.Writer) *videoForm {
	values := []string{"", "video.mp4", defaultModel}
	labels := []string{"Prompt", "Output", "Model"}
	model := &videoForm{output: output}
	for index := range model.inputs {
		input := textinput.New()
		input.Prompt = labels[index] + ": "
		input.SetValue(values[index])
		input.CharLimit = 4096
		model.inputs[index] = input
	}
	model.inputs[0].Focus()
	return model
}

func (model *videoForm) Init() tea.Cmd {
	return textinput.Blink
}

func (model *videoForm) Update(message tea.Msg) (tea.Model, tea.Cmd) {
	if key, ok := message.(tea.KeyMsg); ok {
		switch key.String() {
		case "ctrl+c", "esc":
			model.cancel = true
			return model, tea.Quit
		case "tab", "down":
			model.next()
		case "shift+tab", "up":
			model.previous()
		case "left", "h", "right", "l":
			if model.cursor == 3 {
				if key.String() == "left" || key.String() == "h" {
					model.finish = 0
				} else {
					model.finish = 1
				}
			}
		case "enter":
			if model.cursor == 3 {
				if model.finish == 1 {
					model.cancel = true
					return model, tea.Quit
				}
				if strings.TrimSpace(model.inputs[0].Value()) == "" || strings.TrimSpace(model.inputs[1].Value()) == "" || strings.TrimSpace(model.inputs[2].Value()) == "" {
					model.err = fmt.Errorf("prompt, output, and model are required")
					return model, nil
				}
				return model, tea.Quit
			}
			model.next()
		}
	}
	if model.cursor < len(model.inputs) {
		var command tea.Cmd
		model.inputs[model.cursor], command = model.inputs[model.cursor].Update(message)
		return model, command
	}
	return model, nil
}

func (model *videoForm) next() {
	if model.cursor < len(model.inputs) {
		model.inputs[model.cursor].Blur()
	}
	model.cursor = (model.cursor + 1) % 4
	if model.cursor < len(model.inputs) {
		model.inputs[model.cursor].Focus()
	}
}

func (model *videoForm) previous() {
	if model.cursor < len(model.inputs) {
		model.inputs[model.cursor].Blur()
	}
	model.cursor = (model.cursor + 3) % 4
	if model.cursor < len(model.inputs) {
		model.inputs[model.cursor].Focus()
	}
}

func (model *videoForm) View() string {
	rows := make([]string, 0, len(model.inputs)+1)
	for index := range model.inputs {
		rows = append(rows, model.inputs[index].View())
	}
	finish := "Generate"
	if model.finish == 1 {
		finish = "Back"
	}
	rows = append(rows, lipgloss.NewStyle().Foreground(lipgloss.Color("#A78BFA")).Render("Finish: "+finish))
	if model.err != nil {
		rows = append(rows, lipgloss.NewStyle().Foreground(lipgloss.Color("#F87171")).Render(model.err.Error()))
	}
	controls := "↑↓/tab move  ←→/hl option  Enter confirm  Esc cancel"
	return lipgloss.NewStyle().Padding(1, 2).Border(lipgloss.RoundedBorder()).Render("Generate video\n\n" + strings.Join(rows, "\n") + "\n\n" + lipgloss.NewStyle().Foreground(lipgloss.Color("#94A3B8")).Render(controls))
}
