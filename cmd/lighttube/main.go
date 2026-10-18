package main

import (
	"errors"
	"flag"
	"fmt"
	"os"

	"lighttube/internal/player"
)

const version = "0.1.0"

func printUsage() {
	fmt.Fprintf(os.Stderr, "lightTube — small, fast YouTube client\n\nUsage:\n  lighttube play <url> [--audio]\n  lighttube --version\n")
}

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
}

func run(args []string) error {
	if len(args) == 0 {
		printUsage()
		return errors.New("no command specified")
	}

	var debug bool
	var filtered []string
	for _, arg := range args {
		if arg == "--debug" {
			debug = true
			continue
		}
		filtered = append(filtered, arg)
	}

	if len(filtered) == 0 {
		printUsage()
		return errors.New("no command specified")
	}

	switch filtered[0] {
	case "--version", "-version", "version":
		fmt.Println("lighttube " + version)
		return nil
	case "--help", "-help", "-h", "help":
		printUsage()
		return nil
	case "play":
		return runPlay(filtered[1:], debug)
	default:
		printUsage()
		return fmt.Errorf("unknown command %q", filtered[0])
	}
}

func runPlay(args []string, debug bool) error {
	fs := flag.NewFlagSet("play", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	audio := fs.Bool("audio", false, "play audio only")

	if err := fs.Parse(args); err != nil {
		return err
	}

	rest := fs.Args()
	if len(rest) == 0 {
		return errors.New("usage: lighttube play <url> [--audio]")
	}

	url := rest[0]
	// Support trailing flags such as 'play <url> --audio'.
	for _, extra := range rest[1:] {
		if extra == "--audio" || extra == "-audio" {
			*audio = true
		}
	}

	p := player.New()
	opts := player.Options{
		URL:    url,
		Audio:  *audio,
		Stdin:  os.Stdin,
		Stdout: os.Stdout,
		Stderr: os.Stderr,
	}

	if debug {
		power := player.DetectPower()
		mode := player.ModeVideo
		if *audio {
			mode = player.ModeAudio
		}
		fmt.Fprintf(os.Stderr, "debug: detected power=%v, mode=%v\n", power, mode)
		fmt.Fprintf(os.Stderr, "debug: mpv args=%q\n", p.BuildArgs(opts))
	}

	return p.Play(opts)
}
