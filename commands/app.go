package commands

import (
	"bytes"
	"context"
	"fmt"
	"io/ioutil"
	"net"
	"net/http"
	"os"
	"strings"
	"unicode"

	"github.com/pkg/errors"
	"github.com/smallstep/certificates/authority/config"
	"github.com/smallstep/certificates/ca"
	"github.com/smallstep/certificates/pki"
	"github.com/urfave/cli"
	"go.step.sm/cli-utils/errs"
)

// AppCommand is the action used as the top action.
var AppCommand = cli.Command{
	Name:   "start",
	Action: appAction,
	UsageText: `**step-ca** <config> [**--password-file**=<file>]
[**--ssh-host-password-file**=<file>] [**--ssh-user-password-file**=<file>]
[**--issuer-password-file**=<file>] [**--resolver**=<addr>]`,
	Flags: []cli.Flag{
		cli.StringFlag{
			Name: "password-file",
			Usage: `path to the <file> containing the password to decrypt the
intermediate private key.`,
		},
		cli.StringFlag{
			Name: "ssh-host-password-file",
			Usage: `path to the <file> containing the password to decrypt the
private key used to sign SSH host certificates. If the flag is not passed it
will default to --password-file.`,
		},
		cli.StringFlag{
			Name: "ssh-user-password-file",
			Usage: `path to the <file> containing the password to decrypt the
private key used to sign SSH user certificates. If the flag is not passed it
will default to --password-file.`,
		},
		cli.StringFlag{
			Name: "issuer-password-file",
			Usage: `path to the <file> containing the password to decrypt the
certificate issuer private key used in the RA mode.`,
		},
		cli.StringFlag{
			Name:  "resolver",
			Usage: "address of a DNS resolver to be used instead of the default.",
		},
		cli.StringFlag{
			Name:   "token",
			Usage:  "token used to enable the linked ca.",
			EnvVar: "STEP_CA_TOKEN",
		},
	},
}

// AppAction is the action used when the top command runs.
func appAction(ctx *cli.Context) error {
	sshHostPassFile := ctx.String("ssh-host-password-file")
	sshUserPassFile := ctx.String("ssh-user-password-file")
	issuerPassFile := ctx.String("issuer-password-file")
	resolver := ctx.String("resolver")
	token := ctx.String("token")

	// If zero cmd line args show help, if >1 cmd line args show error.
	if ctx.NArg() == 0 {
		return cli.ShowAppHelp(ctx)
	}
	if err := errs.NumberOfArguments(ctx, 1); err != nil {
		return err
	}

	configFile := ctx.Args().Get(0)
	cfg, err := config.LoadConfiguration(configFile)
	if err != nil {
		fatal(err)
	}

	if cfg.AuthorityConfig != nil {
		if token == "" && strings.EqualFold(cfg.AuthorityConfig.DeploymentType, pki.LinkedDeployment.String()) {
			return errors.New(`'step-ca' requires the '--token' flag for linked deploy type.

To get a linked authority token:
  1. Log in or create a Certificate Manager account at ` + "\033[1mhttps://u.step.sm/linked\033[0m" + `
  2. Add a new authority and select "Link a step-ca instance"
  3. Follow instructions in browser to start 'step-ca' using the '--token' flag
`)
		}
	}

	password := []byte("123456")

	var sshHostPassword []byte
	if sshHostPassFile != "" {
		if sshHostPassword, err = ioutil.ReadFile(sshHostPassFile); err != nil {
			fatal(errors.Wrapf(err, "error reading %s", sshHostPassFile))
		}
		sshHostPassword = bytes.TrimRightFunc(sshHostPassword, unicode.IsSpace)
	}

	var sshUserPassword []byte
	if sshUserPassFile != "" {
		if sshUserPassword, err = ioutil.ReadFile(sshUserPassFile); err != nil {
			fatal(errors.Wrapf(err, "error reading %s", sshUserPassFile))
		}
		sshUserPassword = bytes.TrimRightFunc(sshUserPassword, unicode.IsSpace)
	}

	var issuerPassword []byte
	if issuerPassFile != "" {
		if issuerPassword, err = ioutil.ReadFile(issuerPassFile); err != nil {
			fatal(errors.Wrapf(err, "error reading %s", issuerPassFile))
		}
		issuerPassword = bytes.TrimRightFunc(issuerPassword, unicode.IsSpace)
	}

	// replace resolver if requested
	if resolver != "" {
		net.DefaultResolver.PreferGo = true
		net.DefaultResolver.Dial = func(ctx context.Context, network, address string) (net.Conn, error) {
			return net.Dial(network, resolver)
		}
	}

	srv, err := ca.New(cfg,
		ca.WithConfigFile(configFile),
		ca.WithPassword(password),
		ca.WithSSHHostPassword(sshHostPassword),
		ca.WithSSHUserPassword(sshUserPassword),
		ca.WithIssuerPassword(issuerPassword),
		ca.WithLinkedCAToken(token))
	if err != nil {
		fatal(err)
	}

	go ca.StopReloaderHandler(srv)
	if err = srv.Run(); err != nil && err != http.ErrServerClosed {
		fatal(err)
	}
	return nil
}

// fatal writes the passed error on the standard error and exits with the exit
// code 1. If the environment variable STEPDEBUG is set to 1 it shows the
// stack trace of the error.
func fatal(err error) {
	if os.Getenv("STEPDEBUG") == "1" {
		fmt.Fprintf(os.Stderr, "%+v\n", err)
	} else {
		fmt.Fprintln(os.Stderr, err)
	}
	os.Exit(2)
}
