package tui

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"

	"g9router/internal/i18n"
	"github.com/charmbracelet/huh"
)

func (ui *UI) huhMode() bool {
	if ui.forceHuh {
		return true
	}
	file, ok := ui.In.(*os.File)
	return ok && IsTerminal(file)
}

func (ui *UI) liveMode() bool {
	return ui.huhMode() && !accessibleMode(ui.In)
}

func (ui *UI) huhChoice(title string, options []string) (string, error) {
	return ui.huhChoiceIO(title, options, ui.In, ui.Out)
}

func (ui *UI) huhChoiceIO(title string, options []string, input io.Reader, output io.Writer) (string, error) {
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
	))
	if err := ui.runHuhIO(form, input, output); err != nil {
		return "", err
	}
	return value, nil
}

func (ui *UI) huhValue(title, value string, password bool) (string, error) {
	input := huh.NewInput().Title(title).Value(&value)
	if password {
		input = input.EchoMode(huh.EchoModePassword)
	}
	form := huh.NewForm(huh.NewGroup(input))
	if err := ui.runHuh(form); err != nil {
		return "", err
	}
	return value, nil
}

func (ui *UI) huhConfirm(title string, value bool) (bool, error) {
	form := huh.NewForm(huh.NewGroup(
		huh.NewConfirm().Title(title).Value(&value),
	))
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
		endpoint := ui.t("menu.endpoint")
		providers := ui.t("menu.providers")
		combos := ui.t("menu.combos")
		statistics := ui.t("menu.statistics")
		cliTools := ui.t("menu.cliTools")
		logs := ui.t("menu.logs")
		settings := ui.t("menu.settings")
		language := ui.t("menu.language")
		exit := ui.t("menu.exit")
		choice, err := ui.mainMenuChoice([]string{endpoint, providers, combos, statistics, cliTools, logs, settings, language, exit})
		if !isInteractiveWriter(ui.Out) || accessibleMode(ui.In) {
			choice, err = ui.huhChoice(ui.t("menu.title"), []string{endpoint, providers, combos, statistics, cliTools, logs, settings, language, exit})
		}
		if err != nil {
			return err
		}
		run := func(name string, action func() error) {
			for {
				err := action()
				if err == nil || errors.Is(err, huh.ErrUserAborted) {
					return
				}
				fmt.Fprintln(ui.Out, Error(err.Error()))
				retry, retryErr := ui.huhConfirm("Retry "+name+"?", true)
				if retryErr != nil || !retry {
					return
				}
			}
		}
		switch choice {
		case providers:
			run(providers, func() error {
				if ui.liveMode() {
					return ui.liveProviders()
				}
				return ui.providers(reader)
			})
		case endpoint:
			run(endpoint, func() error { return ui.liveEndpoint() })
		case combos:
			run(combos, func() error {
				if ui.liveMode() {
					return ui.liveCombos()
				}
				return ui.combos(reader)
			})
		case statistics:
			run(statistics, func() error { return ui.liveStatistics() })
		case cliTools:
			run(cliTools, func() error {
				if ui.liveMode() {
					return ui.liveCLITools()
				}
				return ui.cliTools(reader)
			})
		case logs:
			run(logs, func() error { return ui.liveLogs() })
		case settings:
			run(settings, func() error {
				if ui.liveMode() {
					return ui.liveSettings()
				}
				return ui.settings(reader)
			})
		case language:
			if ui.liveMode() {
				if err := ui.liveLanguage(); err != nil && !errors.Is(err, huh.ErrUserAborted) {
					return err
				}
				continue
			}
			if err := ui.selectLanguage(); err != nil && !errors.Is(err, huh.ErrUserAborted) {
				return err
			}
		case exit:
			return nil
		}
	}
}

func (ui *UI) selectLanguage() error {
	choice, err := ui.huhChoice(ui.t("language.title"), []string{ui.t("language.english"), ui.t("language.vietnamese")})
	if err != nil {
		return err
	}
	if choice == ui.t("language.vietnamese") {
		ui.Locale = i18n.Vietnamese
	} else {
		ui.Locale = i18n.English
	}
	return nil
}
