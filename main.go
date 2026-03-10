package main

import (
	"fmt"
	"os"

	tea "github.com/charmbracelet/bubbletea"

	"cinder/ui"
)

func main() {
	m := ui.NewModel()
	p := tea.NewProgram(m, tea.WithAltScreen())

	if _, err := p.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "visualizer error: %v\n", err)
		os.Exit(1)
	}
}
