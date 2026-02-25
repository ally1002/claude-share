package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

var version = "dev"

func main() {
	showVersion := flag.Bool("version", false, "Show version")
	flag.Usage = printUsage
	flag.Parse()

	if *showVersion {
		fmt.Println(version)
		return
	}

	if flag.NArg() < 1 {
		printUsage()
		os.Exit(1)
	}

	claudeDir := filepath.Join(os.Getenv("HOME"), ".claude")

	switch flag.Arg(0) {
	case "list":
		cmdList(claudeDir, flag.Args()[1:])
	case "export":
		cmdExport(claudeDir, flag.Args()[1:])
	case "version":
		fmt.Println(version)
	case "help":
		printUsage()
	default:
		fmt.Fprintf(os.Stderr, "Unknown command: %s\n", flag.Arg(0))
		printUsage()
		os.Exit(1)
	}
}

func printUsage() {
	fmt.Fprintln(os.Stderr, `Usage:
  claude-share [global options] command [command options]

Global options:
  --version    Show version
  --help       Show this help

Commands:
  list         List all sessions
  export       Export a session to HTML

Examples:
  claude-share list --project myproject
  claude-share export abc123 -o output.html`)
}

func cmdList(claudeDir string, args []string) {
	fs := flag.NewFlagSet("list", flag.ExitOnError)
	project := fs.String("project", "", "Filter sessions by project path substring")
	fs.Parse(args)

	sessions, err := ParseHistory(claudeDir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	filter := *project
	for _, s := range sessions {
		if filter != "" && !strings.Contains(strings.ToLower(s.Project), strings.ToLower(filter)) {
			continue
		}
		ts := time.UnixMilli(s.Timestamp).Format("2006-01-02 15:04")
		prompt := s.FirstPrompt
		if len(prompt) > 60 {
			prompt = prompt[:60] + "…"
		}
		projName := filepath.Base(s.Project)
		if projName == "" || projName == "." {
			projName = s.Project
		}
		fmt.Printf("%-38s  %-20s  %s  %s\n", s.ID, projName, ts, prompt)
	}
}

func cmdExport(claudeDir string, args []string) {
	var flagArgs []string
	var positional []string
	for i := 0; i < len(args); i++ {
		if strings.HasPrefix(args[i], "-") {
			flagArgs = append(flagArgs, args[i])
			if args[i] == "-o" && i+1 < len(args) {
				i++
				flagArgs = append(flagArgs, args[i])
			}
		} else {
			positional = append(positional, args[i])
		}
	}

	fs := flag.NewFlagSet("export", flag.ExitOnError)
	output := fs.String("o", "", "Output file (default: stdout)")
	includeTools := fs.Bool("include-tools", false, "Include tool calls and results")
	includeThinking := fs.Bool("include-thinking", false, "Include thinking blocks")
	fs.Parse(flagArgs)

	if len(positional) < 1 {
		fmt.Fprintln(os.Stderr, "Error: session ID required")
		fmt.Fprintln(os.Stderr, "Usage: claude-share export <session-id> [-o file] [--include-tools] [--include-thinking]")
		os.Exit(1)
	}
	sessionID := positional[0]

	sessionPath, err := FindSessionPath(claudeDir, sessionID)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	opts := ParseOpts{
		IncludeTools:    *includeTools,
		IncludeThinking: *includeThinking,
	}
	messages, err := ParseSession(sessionPath, opts)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error parsing session: %v\n", err)
		os.Exit(1)
	}

	if len(messages) == 0 {
		fmt.Fprintln(os.Stderr, "No messages found in session")
		os.Exit(1)
	}

	sessions, err := ParseHistory(claudeDir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Warning: could not load session history: %v\n", err)
	}
	meta := SessionMeta{SessionID: sessionID, MessageCount: len(messages)}
	for _, s := range sessions {
		if s.ID == sessionID {
			meta.Project = filepath.Base(s.Project)
			meta.Date = time.UnixMilli(s.Timestamp).Format("Jan 2, 2006")
			meta.FirstPrompt = s.FirstPrompt
			break
		}
	}

	htmlStr, err := RenderHTML(messages, meta, RenderOpts{
		IncludeTools:    *includeTools,
		IncludeThinking: *includeThinking,
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error rendering: %v\n", err)
		os.Exit(1)
	}

	if *output != "" {
		if err := os.WriteFile(*output, []byte(htmlStr), 0644); err != nil {
			fmt.Fprintf(os.Stderr, "Error writing file: %v\n", err)
			os.Exit(1)
		}
		fmt.Fprintf(os.Stderr, "Exported to %s\n", *output)
	} else {
		fmt.Print(htmlStr)
	}
}
