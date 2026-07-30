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
	tea "github.com/charmbracelet/bubbletea"
)

func (ui *UI) huhMode() bool {
	if ui.forceHuh {
		return true
	}
	file, ok := ui.In.(*os.File)
	return ok && IsTerminal(file)
}

func (ui *UI) liveMode() bool {
	if ui.forceTea {
		return true
	}
	return ui.huhMode() && !accessibleMode(ui.In)
}

func (ui *UI) huhChoice(title string, options []string) (string, error) {
	return ui.huhChoiceIO(title, options, ui.In, ui.Out)
}

func (ui *UI) huhChoiceIO(title string, options []string, input io.Reader, output io.Writer) (string, error) {
	if len(options) == 0 {
		return "", fmt.Errorf("no choices available")
	}
	result, err := ui.runTUIForm(title, []tuiField{{label: title, kind: tuiSelect, options: options, value: options[0]}}, input, output)
	if err != nil {
		return "", err
	}
	return result.values[0], nil
}

func (ui *UI) huhValue(title, value string, password bool) (string, error) {
	result, err := ui.runTUIForm(title, []tuiField{{label: title, kind: tuiInput, value: value, password: password}}, ui.In, ui.Out)
	if err != nil {
		return "", err
	}
	return result.values[0], nil
}

func (ui *UI) huhConfirm(title string, value bool) (bool, error) {
	return ui.tuiConfirm(title, ui.In, ui.Out)
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
		quota := ui.t("menu.quota")
		cliTools := ui.t("menu.cliTools")
		logs := ui.t("menu.logs")
		settings := ui.t("menu.settings")
		language := ui.t("menu.language")
		credits := ui.t("menu.credits")
		exit := ui.t("menu.exit")
		items := []string{endpoint, providers, combos, statistics, quota, cliTools, logs, settings, language, credits, exit}
		choice, err := ui.mainMenuChoice(items)
		if !ui.forceTea && (!isInteractiveWriter(ui.Out) || accessibleMode(ui.In)) {
			choice, err = ui.huhChoice(ui.t("menu.title"), items)
		}
		if err != nil {
			return err
		}
		run := func(name string, action func() error) {
			for {
				err := action()
				if err == nil || errors.Is(err, errUserAborted) || errors.Is(err, tea.ErrProgramKilled) || errors.Is(err, tea.ErrInterrupted) {
					return
				}
				fmt.Fprintln(ui.Out, Error(err.Error()))
				retry, retryErr := ui.huhConfirm(fmt.Sprintf(ui.t("common.retryPrompt"), name), true)
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
			run(endpoint, func() error {
				if ui.liveMode() {
					return ui.liveEndpoint()
				}
				return ui.accessibleEndpoint(reader)
			})
		case combos:
			run(combos, func() error {
				if ui.liveMode() {
					return ui.liveCombos()
				}
				return ui.combos(reader)
			})
		case statistics:
			run(statistics, func() error {
				if ui.liveMode() {
					return ui.liveStatistics()
				}
				return ui.accessibleStatistics(reader)
			})
		case quota:
			run(quota, func() error { return ui.liveQuota() })
		case cliTools:
			run(cliTools, func() error {
				if ui.liveMode() {
					return ui.liveCLITools()
				}
				return ui.cliTools(reader)
			})
		case logs:
			run(logs, func() error {
				if ui.liveMode() {
					return ui.liveLogs()
				}
				return ui.accessibleLogs(reader)
			})
		case settings:
			run(settings, func() error {
				if ui.liveMode() {
					return ui.liveSettings()
				}
				return ui.settings(reader)
			})
		case language:
			if ui.liveMode() {
				if err := ui.liveLanguage(); err != nil && !errors.Is(err, errUserAborted) {
					return err
				}
				continue
			}
			if err := ui.selectLanguage(); err != nil && !errors.Is(err, errUserAborted) {
				return err
			}
		case credits:
			run(credits, func() error {
				if ui.liveMode() {
					return ui.liveCredits()
				}
				return ui.accessibleCredits(reader)
			})
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
	locale := i18n.English
	if choice == ui.t("language.vietnamese") {
		locale = i18n.Vietnamese
	}
	if err := ui.saveLocale(locale); err != nil {
		return err
	}
	ui.Locale = locale
	return nil
}
