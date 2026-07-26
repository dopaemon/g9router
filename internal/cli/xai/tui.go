package xai

import (
	"context"
	"fmt"
	"io"
	"os"

	"github.com/charmbracelet/huh"
)

func RunTTY(ctx context.Context, out io.Writer) int {
	if ctx.Err() != nil {
		return 1
	}
	prompt := ""
	output := "video.mp4"
	model := defaultModel
	finish := "Generate"
	accessible := os.Getenv("G9ROUTER_ACCESSIBLE") == "1"
	if info, err := os.Stdin.Stat(); err != nil || info.Mode()&os.ModeCharDevice == 0 {
		accessible = true
	}
	form := huh.NewForm(huh.NewGroup(
		huh.NewInput().Title("Prompt").Description("Describe the video to generate").Value(&prompt).Validate(huh.ValidateNotEmpty()),
		huh.NewInput().Title("Output").Value(&output).Validate(huh.ValidateNotEmpty()),
		huh.NewInput().Title("Model").Value(&model).Validate(huh.ValidateNotEmpty()),
		huh.NewSelect[string]().Title("Finish").Options(
			huh.NewOption("Generate", "Generate"),
			huh.NewOption("Back", "Back"),
		).Value(&finish),
	)).WithAccessible(accessible).WithInput(os.Stdin).WithOutput(out).WithTheme(huh.ThemeCharm()).WithWidth(72)
	if err := form.Run(); err != nil {
		fmt.Fprintln(out, err)
		return 1
	}
	if finish == "Back" {
		return 0
	}
	return Run(ctx, []string{"--prompt", prompt, "--output", output, "--model", model}, out, out)
}
