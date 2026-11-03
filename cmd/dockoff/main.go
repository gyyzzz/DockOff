package main

import (
	"errors"
	"flag"
	"fmt"
	"os"
	"sort"
	"strings"

	"github.com/dockoff/dockoff/internal/targets"
)

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, "dockoff:", err)
		os.Exit(1)
	}
}

func run(args []string) error {
	if len(args) == 0 {
		return errors.New("usage: dockoff <targets|prepare|bundle|install> [options]")
	}
	var err error
	switch args[0] {
	case "targets":
		err = runTargets(args[1:])
	case "prepare":
		err = runPrepare(args[1:])
	case "bundle":
		err = runBundle(args[1:])
	case "install":
		err = runInstall(args[1:])
	case "help", "-h", "--help":
		fmt.Println("usage: dockoff <targets|prepare|bundle|install> [options]")
		return nil
	default:
		return fmt.Errorf("unknown command %q", args[0])
	}
	if errors.Is(err, flag.ErrHelp) {
		return nil
	}
	return err
}

func runTargets(args []string) error {
	if len(args) != 0 {
		return fmt.Errorf("targets does not accept arguments")
	}
	profiles, err := targets.All()
	if err != nil {
		return err
	}
	sort.Slice(profiles, func(i, j int) bool { return profiles[i].ID < profiles[j].ID })
	for _, profile := range profiles {
		fmt.Printf("%s\t%s %s\t%s (%s)\n", profile.ID, profile.OSID, profile.OSVersion, profile.NativeArch, profile.GoArch)
	}
	return nil
}

func newFlags(name string) *flag.FlagSet {
	fs := flag.NewFlagSet(name, flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	return fs
}

func required(values ...string) error {
	for _, value := range values {
		if strings.TrimSpace(value) == "" {
			return errors.New("missing required option")
		}
	}
	return nil
}
