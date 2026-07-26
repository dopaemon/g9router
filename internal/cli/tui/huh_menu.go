package tui

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"

	"github.com/charmbracelet/huh"
)

func (ui *UI) huhMode() bool {
	file, ok := ui.In.(*os.File)
	return ok && IsTerminal(file)
}

func (ui *UI) huhChoice(title string, options []string) (string, error) {
	if len(options) == 0 {
		return "", fmt.Errorf("no choices available")
	}
	value := options[0]
	formOptions := make([]huh.Option[string], 0, len(options))
	for _, option := range options {
		formOptions = append(formOptions, huh.NewOption(option, option))
	}
	form := huh.NewForm(huh.NewGroup(
		huh.NewSelect[string]().Title(title).Options(formOptions...).Value(&value),
	)).WithAccessible(false)
	if err := ui.runHuh(form); err != nil {
		return "", err
	}
	return value, nil
}

func (ui *UI) huhValue(title, value string, password bool) (string, error) {
	input := huh.NewInput().Title(title).Value(&value)
	if password {
		input = input.EchoMode(huh.EchoModePassword)
	}
	form := huh.NewForm(huh.NewGroup(input)).WithAccessible(false)
	if err := ui.runHuh(form); err != nil {
		return "", err
	}
	return value, nil
}

func (ui *UI) huhConfirm(title string, value bool) (bool, error) {
	form := huh.NewForm(huh.NewGroup(
		huh.NewConfirm().Title(title).Value(&value),
	)).WithAccessible(false)
	if err := ui.runHuh(form); err != nil {
		return false, err
	}
	return value, nil
}

func (ui *UI) huhNumber(title string, labels []string) (int, error) {
	options := make([]string, 0, len(labels))
	for index, label := range labels {
		options = append(options, fmt.Sprintf("%d. %s", index+1, label))
	}
	choice, err := ui.huhChoice(title, options)
	if err != nil {
		return 0, err
	}
	fields := strings.Fields(choice)
	if len(fields) == 0 {
		return 0, fmt.Errorf("invalid choice")
	}
	index, err := strconv.Atoi(strings.TrimSuffix(fields[0], "."))
	if err != nil {
		return 0, err
	}
	return index, nil
}

func (ui *UI) readChoice(reader *bufio.Reader, title string, options []string) (string, error) {
	if ui.huhMode() {
		return ui.huhChoice(title, options)
	}
	fmt.Fprint(ui.Out, title+": ")
	line, err := reader.ReadString('\n')
	if err != nil {
		return "", err
	}
	return line, nil
}

func isInteractiveWriter(writer io.Writer) bool {
	file, ok := writer.(*os.File)
	return ok && IsTerminal(file)
}

func (ui *UI) huhMenu(reader *bufio.Reader) error {
	for {
		choice, err := ui.huhChoice("9Router CLI", []string{"Providers", "API Keys", "Combos", "CLI Tools", "Settings", "OAuth", "Exit"})
		if err != nil {
			return err
		}
		switch choice {
		case "Providers":
			if err := ui.providers(reader); err != nil {
				return err
			}
		case "API Keys":
			if err := ui.apiKeys(reader); err != nil {
				return err
			}
		case "Combos":
			if err := ui.combos(reader); err != nil {
				return err
			}
		case "CLI Tools":
			if err := ui.cliTools(reader); err != nil {
				return err
			}
		case "Settings":
			if err := ui.settings(reader); err != nil {
				return err
			}
		case "OAuth":
			if err := ui.oauth(reader); err != nil {
				return err
			}
		case "Exit":
			return nil
		}
	}
}
