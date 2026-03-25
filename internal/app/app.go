package app

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"

	onboardingweixin "codecli-channels/internal/onboarding/weixin"
)

const (
	commandRun     = "run"
	commandWeixin  = "weixin"
	commandHelp    = "help"
	commandUnknown = "unknown"
)

type commandEnv struct {
	stdout io.Writer
	stderr io.Writer

	runWeixinSetup    func(context.Context, onboardingweixin.SetupOptions) (*onboardingweixin.SetupResult, error)
	verifyWeixinToken func(context.Context, onboardingweixin.VerifyTokenOptions) error
}

func Main(args []string, stdout, stderr io.Writer) int {
	env := commandEnv{
		stdout:            stdout,
		stderr:            stderr,
		runWeixinSetup:    onboardingweixin.RunSetupFlow,
		verifyWeixinToken: onboardingweixin.VerifyToken,
	}

	command, rest := topLevelCommand(args)
	var err error
	switch command {
	case commandRun:
		err = runService(rest, env)
	case commandWeixin:
		err = runWeixin(rest, env)
	case commandHelp:
		printRootUsage(stdout)
	case commandUnknown:
		err = fmt.Errorf("未知命令: %s", firstArg(args))
		printRootUsage(stderr)
	default:
		err = fmt.Errorf("未知命令: %s", command)
	}
	if err == nil {
		return 0
	}
	if errors.Is(err, flagErrHelp) {
		return 0
	}
	if stderr != nil {
		_, _ = fmt.Fprintf(stderr, "%v\n", err)
	}
	return 1
}

func topLevelCommand(args []string) (string, []string) {
	if len(args) == 0 {
		return commandRun, nil
	}
	if strings.HasPrefix(args[0], "-") {
		return commandRun, args
	}
	switch args[0] {
	case commandRun:
		return commandRun, args[1:]
	case commandWeixin:
		return commandWeixin, args[1:]
	case commandHelp, "-h", "--help":
		return commandHelp, nil
	default:
		return commandUnknown, args
	}
}

func firstArg(args []string) string {
	if len(args) == 0 {
		return ""
	}
	return args[0]
}

func printRootUsage(w io.Writer) {
	if w == nil {
		return
	}
	_, _ = fmt.Fprintln(w, "Usage:")
	_, _ = fmt.Fprintln(w, "  codecli-channels [-config path]")
	_, _ = fmt.Fprintln(w, "  codecli-channels run [-config path]")
	_, _ = fmt.Fprintln(w, "  codecli-channels weixin setup [-config path] [options]")
	_, _ = fmt.Fprintln(w, "  codecli-channels weixin bind -token <token> [-config path] [options]")
}
