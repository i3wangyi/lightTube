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

	default:
		return fmt.Errorf("unknown account command %q; valid commands: signin, signout, status", args[0])
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
