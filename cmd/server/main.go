// Command server runs register_manager and its administrative commands.
//
//	server [serve]                  run the HTTP server; creates the super admin on first start
//	server admin reset-password     set the super admin's password and show its login name
//	server backup --to FILE         write a consistent database snapshot
//	server openapi                  print the OpenAPI document
//
// Configuration comes from environment variables (see internal/config).
package main

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"golang.org/x/term"

	"register_manager/internal/accounts"
	"register_manager/internal/api"
	"register_manager/internal/apperr"
	"register_manager/internal/config"
	"register_manager/internal/server"
	"register_manager/internal/storage"
	"register_manager/web"
)

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, "error:", describe(err))
		os.Exit(1)
	}
}

func describe(err error) string {
	var ae *apperr.Error
	if errors.As(err, &ae) {
		if len(ae.Params) > 0 {
			b, _ := json.Marshal(ae.Params)
			return ae.Code + " " + string(b)
		}
		return ae.Code
	}
	return err.Error()
}

const usage = `usage:
  server [serve]
  server admin reset-password
  server backup --to FILE
  server openapi`

func run(args []string) error {
	cmd := "serve"
	if len(args) > 0 {
		cmd, args = args[0], args[1:]
	}
	switch cmd {
	case "serve":
		return serve()
	case "admin":
		return admin(args)
	case "backup":
		return backup(args)
	case "openapi":
		b, err := json.MarshalIndent(api.OpenAPI(), "", "  ")
		if err != nil {
			return err
		}
		_, err = os.Stdout.Write(append(b, '\n'))
		return err
	case "help", "-h", "--help":
		fmt.Println(usage)
		return nil
	}
	return fmt.Errorf("unknown command %q\n%s", cmd, usage)
}

func setup() (config.Config, *slog.Logger, error) {
	cfg, err := config.FromEnv()
	if err != nil {
		return cfg, nil, err
	}
	log := slog.New(slog.NewJSONHandler(os.Stderr, &slog.HandlerOptions{Level: cfg.LogLevel}))
	slog.SetDefault(log)
	return cfg, log, nil
}

func serve() error {
	cfg, log, err := setup()
	if err != nil {
		return err
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	db, err := storage.Open(ctx, cfg.DBPath, log)
	if err != nil {
		return err
	}
	defer db.Close()

	// The one super admin is created on first start. Its generated password
	// is shown only now, outside the structured log; it must be changed at
	// the first sign-in.
	accts := &accounts.Service{DB: db, Now: func() time.Time { return time.Now().UTC() }}
	credentials, err := accts.EnsureSuperAdmin(ctx)
	if err != nil {
		return fmt.Errorf("create super admin: %w", err)
	}
	if credentials != nil {
		fmt.Fprintf(os.Stderr, "\n"+
			"================================================================\n"+
			"  Super admin created. Sign in and change the password now.\n"+
			"    login:    %s\n"+
			"    password: %s\n"+
			"  This password is shown only once. If it is lost, run:\n"+
			"    server admin reset-password\n"+
			"================================================================\n\n", credentials.Login, credentials.Password)
	}

	handler, _ := server.New(ctx, server.Options{
		DB: db, Log: log, CookieSecure: cfg.CookieSecure,
		TrustedProxyHeader: cfg.TrustedProxyHeader, Frontend: web.Dist(),
	})
	srv := &http.Server{
		Addr:              cfg.Listen,
		Handler:           handler,
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       30 * time.Second,
		WriteTimeout:      60 * time.Second,
		IdleTimeout:       120 * time.Second,
	}
	errc := make(chan error, 1)
	go func() {
		log.Info("listening", "addr", cfg.Listen, "db", cfg.DBPath)
		errc <- srv.ListenAndServe()
	}()
	select {
	case err := <-errc:
		return err
	case <-ctx.Done():
	}
	log.Info("shutting down")
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	return srv.Shutdown(shutdownCtx)
}

func admin(args []string) error {
	if len(args) == 0 {
		return errors.New(usage)
	}
	sub, args := args[0], args[1:]
	if sub != "reset-password" {
		return fmt.Errorf("unknown admin command %q\n%s", sub, usage)
	}
	if len(args) > 0 {
		return fmt.Errorf("admin reset-password takes no arguments; the password is read from the terminal or standard input\n%s", usage)
	}
	cfg, log, err := setup()
	if err != nil {
		return err
	}
	ctx := context.Background()
	db, err := storage.Open(ctx, cfg.DBPath, log)
	if err != nil {
		return err
	}
	defer db.Close()
	svc := &accounts.Service{DB: db, Now: func() time.Time { return time.Now().UTC() }}

	pw, err := readPassword()
	if err != nil {
		return err
	}
	login, err := svc.CLIResetPassword(ctx, pw)
	if err != nil {
		return err
	}
	fmt.Printf("super admin password set; sign in with login name %q\n", login)
	return nil
}

// readPassword reads a new password from the terminal (twice, without echo)
// or a single line from standard input when it is not a terminal. Passwords
// are never accepted as command-line arguments.
func readPassword() (string, error) {
	fd := int(os.Stdin.Fd())
	if term.IsTerminal(fd) {
		fmt.Fprint(os.Stderr, "New password: ")
		a, err := term.ReadPassword(fd)
		fmt.Fprintln(os.Stderr)
		if err != nil {
			return "", err
		}
		fmt.Fprint(os.Stderr, "Repeat password: ")
		b, err := term.ReadPassword(fd)
		fmt.Fprintln(os.Stderr)
		if err != nil {
			return "", err
		}
		if string(a) != string(b) {
			return "", errors.New("passwords do not match")
		}
		return string(a), nil
	}
	line, err := bufio.NewReader(os.Stdin).ReadString('\n')
	if err != nil && line == "" {
		return "", errors.New("no password on standard input")
	}
	return strings.TrimRight(line, "\r\n"), nil
}

func backup(args []string) error {
	fs := flag.NewFlagSet("backup", flag.ContinueOnError)
	to := fs.String("to", "", "target file (must not exist)")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *to == "" {
		return errors.New("--to is required")
	}
	cfg, log, err := setup()
	if err != nil {
		return err
	}
	ctx := context.Background()
	db, err := storage.Open(ctx, cfg.DBPath, log)
	if err != nil {
		return err
	}
	defer db.Close()
	if err := db.Backup(ctx, *to); err != nil {
		return err
	}
	fmt.Printf("backup written to %s\n", *to)
	return nil
}
