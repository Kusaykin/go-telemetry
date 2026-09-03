package config

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"strconv"
	"time"
)

const DefaultAddress = "localhost:8080"

const addressUsage = "адрес эндпоинта HTTP-сервера"

func newFlagSet(name string, errOut io.Writer) *flag.FlagSet {
	fs := flag.NewFlagSet(name, flag.ContinueOnError)
	fs.SetOutput(errOut)

	return fs
}

func parse(fs *flag.FlagSet, args []string, errOut io.Writer) error {
	if err := fs.Parse(args); err != nil {
		return err
	}

	if fs.NArg() > 0 {
		return fail(fs, errOut, fmt.Errorf("неизвестный аргумент: %q", fs.Arg(0)))
	}

	return nil
}

func fail(fs *flag.FlagSet, errOut io.Writer, err error) error {
	fmt.Fprintln(errOut, err)
	fs.Usage()

	return err
}

func ExitCode(err error) int {
	if err == nil || errors.Is(err, flag.ErrHelp) {
		return 0
	}

	return 2
}

type secondsValue time.Duration

func (v *secondsValue) String() string {
	return strconv.Itoa(int(time.Duration(*v).Seconds()))
}

func (v *secondsValue) Set(s string) error {
	seconds, err := strconv.Atoi(s)
	if err != nil {
		return err
	}

	if seconds <= 0 {
		return fmt.Errorf("должен быть положительным, получено %d", seconds)
	}

	*v = secondsValue(time.Duration(seconds) * time.Second)

	return nil
}

func secondsVar(fs *flag.FlagSet, dst *time.Duration, name, usage string) {
	fs.Var((*secondsValue)(dst), name, usage)
}
