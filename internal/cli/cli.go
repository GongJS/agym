package cli

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/gongjs/agym/internal/accounts"
	"github.com/gongjs/agym/internal/session"
	"github.com/gongjs/agym/internal/usage"
	"github.com/spf13/cobra"
)

type usageErr struct {
	err error
}

func (e usageErr) Error() string { return e.err.Error() }
func (e usageErr) Unwrap() error { return e.err }

type exitCodeErr struct {
	code int
}

func (e exitCodeErr) Error() string { return fmt.Sprintf("exit %d", e.code) }

// Run executes CLI subcommands via Cobra and returns a process exit code.
func Run(args []string, stdout, stderr io.Writer) int {
	root := newRootCommand(stdout, stderr)
	root.SetArgs(args)
	if len(args) == 0 {
		_ = root.Help()
		return 0
	}
	err := root.Execute()
	if err == nil {
		return 0
	}
	var codeErr exitCodeErr
	if errors.As(err, &codeErr) {
		return codeErr.code
	}
	var uErr usageErr
	if errors.As(err, &uErr) {
		fmt.Fprintln(stderr, "agym:", uErr.Error())
		_ = root.Usage()
		return 2
	}
	msg := err.Error()
	if strings.HasPrefix(msg, "unknown command ") || strings.HasPrefix(msg, "unknown flag:") || strings.HasPrefix(msg, "unknown shorthand flag:") {
		fmt.Fprintln(stderr, "agym:", msg)
		_ = root.Usage()
		return 2
	}
	fmt.Fprintln(stderr, "agym:", msg)
	return 1
}

func newRootCommand(stdout, stderr io.Writer) *cobra.Command {
	root := &cobra.Command{
		Use:           "agym",
		Short:         "agym — account manager CLI & menu bar UI",
		Long:          "agym — account manager CLI & menu bar UI\n\nRun without arguments (or with `ui`) to launch the macOS menu bar UI.",
		SilenceErrors: true,
		SilenceUsage:  true,
	}
	root.SetOut(stdout)
	root.SetErr(stderr)
	root.SetFlagErrorFunc(func(_ *cobra.Command, err error) error {
		return usageErr{err: err}
	})

	root.AddCommand(
		&cobra.Command{
			Use:   "ui",
			Short: "Launch the menu bar UI (default when no args)",
		},
		&cobra.Command{
			Use:   "accounts",
			Short: "Print safe account metadata as JSON",
			Args:  noExtraArgs(),
			RunE: func(cmd *cobra.Command, _ []string) error {
				roster, err := accounts.NewDefaultRoster()
				if err != nil {
					return err
				}
				items, err := roster.List(cmd.Context())
				if err != nil {
					return err
				}
				return json.NewEncoder(stdout).Encode(items)
			},
		},
		&cobra.Command{
			Use:   "who",
			Short: "Print the active agy account",
			Args:  noExtraArgs(),
			RunE: func(cmd *cobra.Command, _ []string) error {
				roster, err := accounts.NewDefaultRoster()
				if err != nil {
					return err
				}
				email, err := roster.CurrentEmail(cmd.Context())
				if err != nil {
					return err
				}
				if email == "" {
					fmt.Fprintln(stdout, "No active agy account")
					return exitCodeErr{code: 1}
				}
				fmt.Fprintln(stdout, email)
				return nil
			},
		},
		&cobra.Command{
			Use:     "switch <email>",
			Aliases: []string{"use"},
			Short:   "Switch the live Keychain login to a saved account",
			Args:    exactArgs(1, "switch requires an email address"),
			RunE: func(cmd *cobra.Command, args []string) error {
				roster, err := accounts.NewDefaultRoster()
				if err != nil {
					return err
				}
				if err := roster.Switch(cmd.Context(), args[0]); err != nil {
					return err
				}
				fmt.Fprintf(stdout, "Switched active agy account to %s\n", args[0])
				return nil
			},
		},
		newQuotaCommand(stdout),
		&cobra.Command{
			Use:   "add <email>",
			Short: "Add an existing saved login to the roster",
			Args:  exactArgs(1, "add requires an email address"),
			RunE: func(cmd *cobra.Command, args []string) error {
				roster, err := accounts.NewDefaultRoster()
				if err != nil {
					return err
				}
				added, err := roster.Add(cmd.Context(), args[0])
				if err != nil {
					return err
				}
				if added {
					fmt.Fprintln(stdout, "Account added")
				} else {
					fmt.Fprintln(stdout, "Account already exists")
				}
				return nil
			},
		},
		&cobra.Command{
			Use:   "remove <email>",
			Short: "Remove from rosters (keeps the Keychain backup)",
			Args:  exactArgs(1, "remove requires an email address"),
			RunE: func(_ *cobra.Command, args []string) error {
				roster, err := accounts.NewDefaultRoster()
				if err != nil {
					return err
				}
				if err := roster.Remove(args[0]); err != nil {
					return err
				}
				fmt.Fprintln(stdout, "Account removed from the roster; saved Keychain login was kept")
				return nil
			},
		},
		&cobra.Command{
			Use:   "rotation <email> on|off",
			Short: "Configure automatic-rotation membership",
			Args:  exactArgs(2, "rotation requires an email and on|off"),
			RunE: func(_ *cobra.Command, args []string) error {
				enabled := strings.EqualFold(args[1], "on")
				if !enabled && !strings.EqualFold(args[1], "off") {
					return usageErr{err: errors.New("rotation must be on or off")}
				}
				roster, err := accounts.NewDefaultRoster()
				if err != nil {
					return err
				}
				if err := roster.SetRotation(args[0], enabled); err != nil {
					return err
				}
				fmt.Fprintln(stdout, "Rotation setting updated")
				return nil
			},
		},
		newSessionForwardCommand("install-hook", "Install the quota handoff Stop hook", false, stdout, stderr),
		newSessionForwardCommand("remove-hook", "Remove only the agym Stop hook", false, stdout, stderr),
		newSessionForwardCommand("supervise [--pinned] [--] <agy arguments...>", "Run official agy in an external terminal with safe handoff", true, stdout, stderr),
		newSessionForwardCommand("hook", "Internal command invoked by the Stop hook", false, stdout, stderr),
	)
	return root
}

func newQuotaCommand(stdout io.Writer) *cobra.Command {
	var force bool
	cmd := &cobra.Command{
		Use:   "quota",
		Short: "Print cached per-account quota JSON; optionally refresh now",
		Args: func(_ *cobra.Command, args []string) error {
			if len(args) != 0 {
				return usageErr{err: errors.New("quota accepts only --refresh")}
			}
			return nil
		},
		RunE: func(cmd *cobra.Command, _ []string) error {
			roster, err := accounts.NewDefaultRoster()
			if err != nil {
				return err
			}
			ctx := cmd.Context()
			if ctx == nil {
				ctx = context.Background()
			}
			items, err := roster.List(ctx)
			if err != nil {
				return err
			}
			emails := make([]string, len(items))
			for i, item := range items {
				emails[i] = item.Email
			}
			cachePath, err := usage.DefaultCachePath()
			if err != nil {
				return err
			}
			cache, err := usage.NewRefresher(roster, usage.AgyRunner{}, cachePath).Refresh(ctx, emails, force)
			if err != nil {
				return err
			}
			rows := make([]map[string]any, 0, len(items))
			for _, item := range items {
				row := map[string]any{"email": item.Email, "current": item.Current}
				if quota, ok := cache.View(item.Email); ok {
					row["quota"] = quota
				}
				rows = append(rows, row)
			}
			return json.NewEncoder(stdout).Encode(rows)
		},
	}
	cmd.Flags().BoolVarP(&force, "refresh", "f", false, "Force refresh quotas from official agy now")
	return cmd
}

func newSessionForwardCommand(use, short string, disableFlags bool, stdout, stderr io.Writer) *cobra.Command {
	name := strings.Fields(use)[0]
	return &cobra.Command{
		Use:                use,
		Short:              short,
		DisableFlagParsing: disableFlags,
		RunE: func(_ *cobra.Command, args []string) error {
			code, _ := session.HandleCommand(name, args, stdout, stderr)
			if code != 0 {
				return exitCodeErr{code: code}
			}
			return nil
		},
	}
}

func noExtraArgs() cobra.PositionalArgs {
	return func(_ *cobra.Command, args []string) error {
		if len(args) != 0 {
			return usageErr{err: fmt.Errorf("unexpected arguments: %s", strings.Join(args, " "))}
		}
		return nil
	}
}

func exactArgs(n int, message string) cobra.PositionalArgs {
	return func(_ *cobra.Command, args []string) error {
		if len(args) != n {
			return usageErr{err: errors.New(message)}
		}
		return nil
	}
}
