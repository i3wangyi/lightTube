package main

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"lighttube/internal/yt"
)

func runAccount(env *appEnv, args []string) error {
	if len(args) == 0 {
		return errors.New("usage: lighttube account <signin|signout|status>")
	}

	acc := env.account
	if acc == nil {
		acc = yt.NewAccount()
	}

	switch args[0] {
	case "signin":
		if err := acc.SignIn(context.Background()); err != nil {
			return err
		}
		fmt.Fprintln(env.stdout, "signed in")
		return nil

	case "signout":
		if err := acc.SignOut(); err != nil {
			return err
		}
		fmt.Fprintln(env.stdout, "signed out")
		return nil

	case "status":
		if acc.SignedIn() {
			fmt.Fprintln(env.stdout, "signed in")
		} else {
			fmt.Fprintln(env.stdout, "signed out")
		}
		return nil

	case "list":
		st := env.store
		if st == nil {
			return errors.New("store not available")
		}
		active := st.ActiveAccount()
		for _, a := range st.Settings().Accounts {
			prefix := "  "
			if a.ID == active.ID {
				prefix = "* "
			}
			fmt.Fprintf(env.stdout, "%s%s\t%s\n", prefix, a.ID, a.Label)
		}
		return nil

	case "switch":
		if len(args) < 2 {
			return errors.New("usage: lighttube account switch <id>")
		}
		st := env.store
		if st == nil {
			return errors.New("store not available")
		}
		sw, err := st.SwitchAccount(args[1])
		if err != nil {
			return err
		}
		fmt.Fprintf(env.stdout, "switched to %s (%s)\n", sw.Label, sw.ID)
		return nil

	case "remove":
		if len(args) < 2 {
			return errors.New("usage: lighttube account remove <id>")
		}
		st := env.store
		if st == nil {
			return errors.New("store not available")
		}
		if err := st.RemoveAccount(args[1]); err != nil {
			return err
		}
		fmt.Fprintf(env.stdout, "removed account %s\n", args[1])
		return nil

	default:
		return fmt.Errorf("unknown account command %q; valid commands: signin, signout, status, list, switch, remove", args[0])
	}
}

func runAccountFeed(env *appEnv, args []string) error {
	if len(args) == 0 {
		return errors.New("usage: lighttube feed --account <home|subs|later|history>")
	}

	rawKind := args[0]
	if strings.HasPrefix(rawKind, "=") {
		rawKind = strings.TrimPrefix(rawKind, "=")
	}

	kind, err := yt.NormalizeFeedKind(rawKind)
	if err != nil {
		return fmt.Errorf("unknown account feed kind %q; valid kinds: home, subs, later, history", rawKind)
	}

	acc := env.account
	if acc == nil {
		acc = yt.NewAccount()
	}

	videos, err := acc.Feed(context.Background(), kind)
	if err != nil {
		return err
	}

	for _, v := range videos {
		fmt.Fprintf(env.stdout, "%s\t%s\t%s\n", v.ID, v.Title, v.Channel)
	}

	return nil
}
