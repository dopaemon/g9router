package xai

import (
	"bytes"
	"context"
	"io"
	"strings"

	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

type tuiResult struct {
	code   int
	output string
}

type tuiModel struct {
	inputs  []textinput.Model
	index   int
	spinner spinner.Model
	busy    bool
	result  string
	code    int
}

func newTUIModel() tuiModel {
	labels := []string{"Prompt", "Output", "Model"}
	inputs := make([]textinput.Model, len(labels))
	for index, label := range labels {
		input := textinput.New()
		input.Prompt = label + ": "
		input.Width = 56
		input.CharLimit = 1024
		if label == "Output" {
			input.SetValue("video.mp4")
		}
		if label == "Model" {
			input.SetValue(defaultModel)
		}
		inputs[index] = input
	}
	inputs[0].Focus()
	spin := spinner.New()
	spin.Spinner = spinner.Dot
	return tuiModel{inputs: inputs, spinner: spin}
}

func (model tuiModel) Init() tea.Cmd { return nil }

func (model tuiModel) Update(message tea.Msg) (tea.Model, tea.Cmd) {
	if result, ok := message.(tuiResult); ok {
		model.busy = false
		model.code = result.code
		model.result = result.output
		return model, nil
	}
	if model.busy {
		if _, ok := message.(tea.KeyMsg); ok {
			return model, nil
		}
		var command tea.Cmd
		model.spinner, command = model.spinner.Update(message)
		return model, tea.Batch(command, model.spinner.Tick)
	}
	if key, ok := message.(tea.KeyMsg); ok {
		switch strings.ToLower(key.String()) {
		case "ctrl+c", "q":
			return model, tea.Quit
		case "tab", "down":
			model.inputs[model.index].Blur()
			model.index = (model.index + 1) % len(model.inputs)
			model.inputs[model.index].Focus()
		case "shift+tab", "up":
			model.inputs[model.index].Blur()
			model.index = (model.index - 1 + len(model.inputs)) % len(model.inputs)
			model.inputs[model.index].Focus()
		case "enter":
			if strings.TrimSpace(model.inputs[0].Value()) == "" {
				model.result = "Prompt is required"
				return model, nil
			}
			model.busy = true
			return model, tea.Batch(runTUIJob(model.inputs), model.spinner.Tick)
		}
	}
	var command tea.Cmd
	model.inputs[model.index], command = model.inputs[model.index].Update(message)
	return model, command
}

func (model tuiModel) View() string {
	brand := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#EC4899")).Render("9ROUTER / XAI VIDEO")
	if model.busy {
		return "\n" + brand + "\n\n" + model.spinner.View() + " Generating video…\n"
	}
	content := brand + "\n\n"
	for _, input := range model.inputs {
		content += input.View() + "\n"
	}
	content += "\n" + lipgloss.NewStyle().Foreground(lipgloss.Color("#94A3B8")).Render("tab/↑↓ next field  enter generate  q quit")
	if model.result != "" {
		content += "\n\n" + model.result
	}
	return "\n" + content + "\n"
}

func runTUIJob(inputs []textinput.Model) tea.Cmd {
	return func() tea.Msg {
		var output bytes.Buffer
		args := []string{"--prompt", inputs[0].Value(), "--output", inputs[1].Value(), "--model", inputs[2].Value()}
		code := Run(context.Background(), args, &output, &output)
		return tuiResult{code: code, output: strings.TrimSpace(output.String())}
	}
}

func RunTTY(ctx context.Context, out io.Writer) int {
	program := tea.NewProgram(newTUIModel(), tea.WithOutput(out))
	value, err := program.Run()
	if err != nil {
		return 1
	}
	if ctx.Err() != nil {
		return 1
	}
	return value.(tuiModel).code
}
