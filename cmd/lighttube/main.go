package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
	"time"

	"lighttube/internal/player"
	"lighttube/internal/store"
	"lighttube/internal/thumbs"
	"lighttube/internal/ui"
	"lighttube/internal/yt"
)

const version = "0.1.0"

var processStart = time.Now()

func printUsage(w io.Writer) {
	fmt.Fprintf(w, `lightTube — small, fast YouTube client

Usage:
  lighttube [--external-player] [--debug]
  lighttube play <url> [--audio]
  lighttube search <query>
  lighttube feed [--account home|subs|later|history]
  lighttube account <signin|signout|status>
  lighttube sub add <url|@handle|UC id>
  lighttube sub list
  lighttube --version
`)
}

type appEnv struct {
	ytClient  *yt.Client
	store     *store.Store
	account   *yt.Account
	stdout    io.Writer
	stderr    io.Writer
	stdin     io.Reader
	guiRunner func(env *appEnv) error
}

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
}

func run(args []string) error {
	var debug bool
	var search string
	var externalPlayer bool
	var debugPlay string
	var debugPlaySeconds int
	var filtered []string
	for i := 0; i < len(args); i++ {
		arg := args[i]
		if arg == "--debug" {
			debug = true
			continue
		}
		if arg == "--external-player" {
			externalPlayer = true
			continue
		}
		if arg == "--search" && i+1 < len(args) {
			search = args[i+1]
			i++
			continue
		}
		if arg == "--debug-play" && i+1 < len(args) {
			debugPlay = args[i+1]
			i++
			continue
		}
		if arg == "--debug-play-seconds" && i+1 < len(args) {
			debugPlaySeconds, _ = strconv.Atoi(args[i+1])
			i++
			continue
		}
		filtered = append(filtered, arg)
	}

	// For GUI execution (no subcommands), do not read store files before the first frame.
	if len(filtered) == 0 {
		st, err := store.NewDeferred(store.DefaultDataDir())
		if err != nil {
			return fmt.Errorf("initializing store: %w", err)
		}

		env := &appEnv{
			ytClient: yt.NewClient(),
			store:    st,
			account:  yt.NewAccount(),
			stdout:   os.Stdout,
			stderr:   os.Stderr,
			stdin:    os.Stdin,
			guiRunner: func(e *appEnv) error {
				return ui.Run(ui.Options{
					YTClient:         e.ytClient,
					Store:            e.store,
					Player:           player.New(),
					Account:          e.account,
					Thumbs:           thumbs.New(),
					Debug:            debug,
					StartTime:        processStart,
					Search:           search,
					ExternalPlayer:   externalPlayer,
					DebugPlay:        debugPlay,
					DebugPlaySeconds: debugPlaySeconds,
				})
			},
		}
		return runWithEnv(env, filtered)
	}

	st, err := store.New()
	if err != nil {
		return fmt.Errorf("initializing store: %w", err)
	}

	env := &appEnv{
		ytClient: yt.NewClient(),
		store:    st,
		account:  yt.NewAccount(),
		stdout:   os.Stdout,
		stderr:   os.Stderr,
		stdin:    os.Stdin,
	}

	return runWithEnv(env, filtered)
}

func runWithEnv(env *appEnv, args []string) error {
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
		if env.guiRunner != nil {
			return env.guiRunner(env)
		}
		printUsage(env.stderr)
		return errors.New("no command specified")
	}

	switch filtered[0] {
	case "--version", "-version", "version":
		fmt.Fprintln(env.stdout, "lighttube "+version)
		return nil
	case "--help", "-help", "-h", "help":
		printUsage(env.stdout)
		return nil
	case "play":
		return runPlay(env, filtered[1:], debug)
	case "search":
		return runSearch(env, filtered[1:])
	case "feed":
		return runFeed(env, filtered[1:])
	case "account":
		return runAccount(env, filtered[1:])
	case "sub":
		return runSub(env, filtered[1:])
	default:
		printUsage(env.stderr)
		return fmt.Errorf("unknown command %q", filtered[0])
	}
}

func runPlay(env *appEnv, args []string, debug bool) error {
	fs := flag.NewFlagSet("play", flag.ContinueOnError)
	fs.SetOutput(env.stderr)
	audio := fs.Bool("audio", false, "play audio only")

	if err := fs.Parse(args); err != nil {
		return err
	}

	rest := fs.Args()
	if len(rest) == 0 {
		return errors.New("usage: lighttube play <url> [--audio]")
	}

	url := rest[0]
	for _, extra := range rest[1:] {
		if extra == "--audio" || extra == "-audio" {
			*audio = true
		}
	}

	p := player.New()
	var extraArgs []string
	if env.account != nil && env.account.SignedIn() {
		extraArgs = env.account.YtDlpArgs()
	}
	opts := player.Options{
		URL:       url,
		Audio:     *audio,
		ExtraArgs: extraArgs,
		Stdin:     env.stdin,
		Stdout:    env.stdout,
		Stderr:    env.stderr,
	}

	if debug {
		power := player.DetectPower()
		mode := player.ModeVideo
		if *audio {
			mode = player.ModeAudio
		}
		fmt.Fprintf(env.stderr, "debug: detected power=%v, mode=%v\n", power, mode)
		fmt.Fprintf(env.stderr, "debug: mpv args=%q\n", p.BuildArgs(opts))
	}

	return p.Play(opts)
}

func runSearch(env *appEnv, args []string) error {
	if len(args) == 0 {
		return errors.New("usage: lighttube search <query>")
	}

	query := strings.Join(args, " ")
	videos, err := env.ytClient.Search(context.Background(), query)
	if err != nil {
		return err
	}

	for _, v := range videos {
		durationStr := yt.FormatDuration(v.Duration)
		fmt.Fprintf(env.stdout, "%s\t%s\t%s\t%s\n", v.ID, v.Title, v.Channel, durationStr)
	}

	return nil
}

func runFeed(env *appEnv, args []string) error {
	if len(args) > 0 {
		if args[0] == "--account" || args[0] == "-account" {
			return runAccountFeed(env, args[1:])
		}
		if strings.HasPrefix(args[0], "--account=") || strings.HasPrefix(args[0], "-account=") {
			parts := strings.SplitN(args[0], "=", 2)
			return runAccountFeed(env, []string{parts[1]})
		}
	}

	subs := env.store.Subscriptions()
	if len(subs) == 0 {
		return nil
	}

	channelIDs := make([]string, 0, len(subs))
	for _, sub := range subs {
		if sub.ChannelID != "" {
			channelIDs = append(channelIDs, sub.ChannelID)
		}
	}

	videos, err := env.ytClient.MergedFeed(context.Background(), channelIDs)
	if err != nil {
		return err
	}

	for _, v := range videos {
		dateStr := ""
		if !v.PublishedAt.IsZero() {
			dateStr = v.PublishedAt.Format("2006-01-02 15:04")
		}
		fmt.Fprintf(env.stdout, "%s\t%s\t%s\t%s\n", v.ID, v.Title, v.Channel, dateStr)
	}

	return nil
}

func runSub(env *appEnv, args []string) error {
	if len(args) == 0 {
		return errors.New("usage: lighttube sub <add|list> ...")
	}

	switch args[0] {
	case "add":
		if len(args) < 2 {
			return errors.New("usage: lighttube sub add <url|@handle|UC id>")
		}
		target := args[1]
		channelID, err := env.ytClient.ResolveChannel(context.Background(), target)
		if err != nil {
			return err
		}

		title := channelID
		// Attempt to extract friendly channel title from feed.
		if feedVideos, feedErr := env.ytClient.Feed(context.Background(), channelID); feedErr == nil && len(feedVideos) > 0 {
			if feedVideos[0].Channel != "" {
				title = feedVideos[0].Channel
			}
		}

		sub := yt.Subscription{
			ChannelID: channelID,
			Title:     title,
			URL:       "https://www.youtube.com/channel/" + channelID,
		}

		if err := env.store.AddSubscription(sub); err != nil {
			return err
		}

		fmt.Fprintf(env.stdout, "added %s (%s)\n", title, channelID)
		return nil

	case "list":
		for _, sub := range env.store.Subscriptions() {
			fmt.Fprintf(env.stdout, "%s\t%s\t%s\n", sub.ChannelID, sub.Title, sub.URL)
		}
		return nil

	default:
		return fmt.Errorf("unknown sub command %q", args[0])
	}
}
