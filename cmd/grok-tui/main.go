package main

import (
	"fmt"
	"os"
	"strings"

	"github.com/Bhattisahb121/grok-tui/internal/client"
	"github.com/Bhattisahb121/grok-tui/internal/config"
	"github.com/Bhattisahb121/grok-tui/internal/tui"
	tea "github.com/charmbracelet/bubbletea"
)

func main() {
	if len(os.Args) > 1 {
		switch strings.ToLower(os.Args[1]) {
		case "init":
			runInit()
			return
		case "help", "--help", "-h":
			printHelp()
			return
		case "version", "--version", "-v":
			fmt.Println("grok-tui v0.1.0")
			return
		}
	}

	cfgPath := config.DefaultConfigPath()
	if envPath := os.Getenv("GROK_TUI_CONFIG"); envPath != "" {
		cfgPath = envPath
	}

	cfg, err := config.Load(cfgPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %s\n\n", err)
		fmt.Fprintf(os.Stderr, "Run 'grok-tui init' to create a config file, then add your cookies.\n")
		fmt.Fprintf(os.Stderr, "Config path: %s\n", cfgPath)
		os.Exit(1)
	}

	grokClient := client.New(cfg)
	model := tui.New(grokClient, cfgPath)

	p := tea.NewProgram(model, tea.WithAltScreen())
	if _, err := p.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %s\n", err)
		os.Exit(1)
	}
}

func runInit() {
	cfgPath := config.DefaultConfigPath()
	if envPath := os.Getenv("GROK_TUI_CONFIG"); envPath != "" {
		cfgPath = envPath
	}

	if _, err := os.Stat(cfgPath); err == nil {
		fmt.Printf("Config file already exists: %s\n", cfgPath)
		fmt.Println("Edit it to update your settings.")
		return
	}

	if err := config.CreateDefault(cfgPath); err != nil {
		fmt.Fprintf(os.Stderr, "Failed to create config: %s\n", err)
		os.Exit(1)
	}

	fmt.Printf("Config file created: %s\n\n", cfgPath)
	fmt.Println("Next steps:")
	fmt.Println("  1. Open grok.com in your browser and log in")
	fmt.Println("  2. Open Developer Tools (F12) → Network tab")
	fmt.Println("  3. Send a message to Grok")
	fmt.Println("  4. Find the request to /rest/app-chat/conversations/new")
	fmt.Println("  5. Copy the Cookie header value and paste it into the config file")
	fmt.Println("  6. Optionally copy x-statsig-id header value too")
	fmt.Println()
	fmt.Println("Cookie should contain: x-anonuserid, x-challenge, x-signature, sso, sso-rw")
}

func printHelp() {
	fmt.Println(`grok-tui — Grok Web Chat TUI Client

Usage:
  grok-tui          Start the TUI chat
  grok-tui init     Create a config file
  grok-tui help     Show this help
  grok-tui version  Show version

Environment:
  GROK_TUI_CONFIG   Path to config file (default: ~/.config/grok-tui/config.json)

TUI Commands:
  /new, /reset      Start a new conversation
  /model [name]     Show or change the model (grok-3, grok-3-mini, grok-2)
  /help             Show help in the TUI
  /quit, /exit      Exit

TUI Shortcuts:
  Enter             Send message
  Ctrl+D            Insert newline
  Ctrl+C            Cancel streaming / Quit

Setup:
  1. Run 'grok-tui init' to create a config file
  2. Log into grok.com in your browser
  3. Open DevTools (F12) → Network tab → send a message
  4. Copy the Cookie header from the request to /rest/app-chat/conversations/new
  5. Paste it into the config file's "cookie" field
  6. Run 'grok-tui' to start chatting`)
}
