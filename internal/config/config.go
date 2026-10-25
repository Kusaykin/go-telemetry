package config

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"math"
	"strconv"
	"text/tabwriter"
	"time"
)

const (
	DefaultAddress    = "localhost:8080"
	addressUsage      = "адрес эндпоинта HTTP-сервера"
	envAddress        = "ADDRESS"
	envReportInterval = "REPORT_INTERVAL"
	envPollInterval   = "POLL_INTERVAL"
)

type LookupEnv func(key string) (string, bool)

type flagSet struct {
	*flag.FlagSet
	envs []envVar
}

type envVar struct {
	name string
	flag *flag.Flag
}

func newFlagSet(name string, errOut io.Writer) *flagSet {
	fs := &flagSet{FlagSet: flag.NewFlagSet(name, flag.ContinueOnError)}
	fs.SetOutput(errOut)
	fs.Usage = fs.usage

	return fs
}

func (fs *flagSet) stringVar(dst *string, name, env, usage string) {
	fs.StringVar(dst, name, *dst, usage)
	fs.bindEnv(env, name)
}

func (fs *flagSet) secondsVar(dst *time.Duration, name, env, usage string) {
	fs.Var((*secondsValue)(dst), name, usage)
	fs.bindEnv(env, name)
}

func (fs *flagSet) nonNegativeSecondsVar(dst *time.Duration, name, env, usage string) {
	fs.Var((*nonNegativeSecondsValue)(dst), name, usage)
	fs.bindEnv(env, name)
}

func (fs *flagSet) boolVar(dst *bool, name, env, usage string) {
	fs.BoolVar(dst, name, *dst, usage)
	fs.bindEnv(env, name)
}

func (fs *flagSet) bindEnv(env, flagName string) {
	f := fs.Lookup(flagName)
	if f == nil {
		panic(fmt.Sprintf("config: переменная %s привязана к незарегистрированному флагу -%s", env, flagName))
	}

	fs.envs = append(fs.envs, envVar{name: env, flag: f})
}

func (fs *flagSet) usage() {
	out := fs.Output()

	fmt.Fprintf(out, "Usage of %s:\n", fs.Name())
	fs.PrintDefaults()

	if len(fs.envs) == 0 {
		return
	}

	fmt.Fprintln(out, "\nПеременные окружения (приоритетнее флагов):")

	w := tabwriter.NewWriter(out, 0, 0, 2, ' ', 0)
	for _, e := range fs.envs {
		_, usage := flag.UnquoteUsage(e.flag)
		fmt.Fprintf(w, "  %s\t%s (-%s)\n", e.name, usage, e.flag.Name)
	}

	w.Flush()
}

func (fs *flagSet) parse(args []string, lookup LookupEnv) error {
	if err := fs.Parse(args); err != nil {
		return err
	}

	if fs.NArg() > 0 {
		err := fmt.Errorf("неизвестный аргумент: %q", fs.Arg(0))
		fmt.Fprintln(fs.Output(), err)
		fs.Usage()

		return err
	}

	return fs.applyEnv(lookup)
}

func (fs *flagSet) applyEnv(lookup LookupEnv) error {
	if lookup == nil {
		return nil
	}

	for _, e := range fs.envs {
		v, ok := lookup(e.name)
		if !ok || v == "" {
			continue
		}

		if err := fs.Set(e.flag.Name, v); err != nil {
			err = fmt.Errorf("некорректное значение %s=%q: %w", e.name, v, err)
			fmt.Fprintln(fs.Output(), err)

			return err
		}
	}

	return nil
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

const maxSeconds = int64(math.MaxInt64 / int64(time.Second))

var (
	errNotSeconds     = errors.New("ожидается целое число секунд")
	errNotPositive    = errors.New("должно быть больше нуля")
	errNegative       = errors.New("не должно быть отрицательным")
	errTooManySeconds = fmt.Errorf("должно быть не больше %d", maxSeconds)
)

func (v *secondsValue) Set(s string) error {
	d, err := parseSeconds(s)
	if errors.Is(err, errNegative) {
		return errNotPositive
	}

	if err != nil {
		return err
	}

	if d == 0 {
		return errNotPositive
	}

	*v = secondsValue(d)

	return nil
}

// nonNegativeSecondsValue — как secondsValue, но допускает 0.
type nonNegativeSecondsValue time.Duration

func (v *nonNegativeSecondsValue) String() string {
	return (*secondsValue)(v).String()
}

func (v *nonNegativeSecondsValue) Set(s string) error {
	d, err := parseSeconds(s)
	if err != nil {
		return err
	}

	*v = nonNegativeSecondsValue(d)

	return nil
}

func parseSeconds(s string) (time.Duration, error) {
	seconds, err := strconv.ParseInt(s, 10, 64)
	if err != nil && !errors.Is(err, strconv.ErrRange) {
		return 0, errNotSeconds
	}

	if seconds < 0 {
		return 0, errNegative
	}

	if seconds > maxSeconds {
		return 0, errTooManySeconds
	}

	return time.Duration(seconds) * time.Second, nil
}
