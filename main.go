package main

import (
	"fmt"
	"os"

	tea "github.com/charmbracelet/bubbletea"
)

func printHelp() {
	fmt.Println("gogo - a terminal app")
	fmt.Println()
	fmt.Println("Usage: gogo [flags]")
	fmt.Println()
	fmt.Println("Flags:")
	fmt.Println("  -h, --help     show this help")
	fmt.Println("  -v, --version  show version")
}

func main() {
	for _, arg := range os.Args[1:] {
		switch arg {
		case "-h", "--help":
			printHelp()
			return
		case "-v", "--version":
			fmt.Println(Version)
			return
		}
	}

	p := tea.NewProgram(newModel(), tea.WithAltScreen())
	if _, err := p.Run(); err != nil {
		fmt.Printf("Error: %s\n", err.Error())
		os.Exit(1)
	}
}
