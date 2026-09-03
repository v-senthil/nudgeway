// Command fullwa-cli is the admin CLI for the fullWA platform.
//
// Subcommands:
//
//	tenant create --slug --name                       — create an organization
//	user create   --org-slug --email --password [--admin]
//	                                                  — create a platform user
//	migrate up|down|status [--steps N]                — run schema migrations
//	                                                  (shells out to the `migrate` binary)
//	version                                           — print version
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"time"

	appauth "github.com/fullwa/fullwa/internal/application/auth"
	infauth "github.com/fullwa/fullwa/internal/infrastructure/auth"
	"github.com/fullwa/fullwa/internal/infrastructure/config"
	fmysql "github.com/fullwa/fullwa/internal/infrastructure/mysql"
)

func main() {
	if len(os.Args) < 2 {
		usage()
		os.Exit(2)
	}
	if err := dispatch(os.Args[1], os.Args[2:]); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
}

func dispatch(cmd string, args []string) error {
	switch cmd {
	case "tenant":
		return runTenant(args)
	case "user":
		return runUser(args)
	case "migrate":
		return runMigrate(args)
	case "version":
		fmt.Println("fullwa-cli 0.0.0-dev")
		return nil
	case "-h", "--help", "help":
		usage()
		return nil
	default:
		usage()
		return fmt.Errorf("unknown subcommand: %s", cmd)
	}
}

func usage() {
	fmt.Fprintln(os.Stderr, `fullwa-cli — fullWA admin CLI

usage:
  fullwa-cli <subcommand> [flags]

subcommands:
  tenant create --slug S --name N            create an organization
  user   create --org-slug S --email E --password P [--admin]
                                             create a platform user
  migrate up|down|status [--steps N]         run schema migrations
  version                                    print version
  help                                       show this message`)
}

func loadConfig() (config.Config, error) {
	path := os.Getenv("FULLWA_CONFIG")
	if path == "" {
		path = "config/local.yaml"
	}
	return config.Load(path)
}

// --- tenant ------------------------------------------------------------------

func runTenant(args []string) error {
	if len(args) == 0 {
		return errors.New("tenant: missing action (create)")
	}
	action, rest := args[0], args[1:]
	switch action {
	case "create":
		fs := flag.NewFlagSet("tenant create", flag.ExitOnError)
		slug := fs.String("slug", "", "organization slug (unique)")
		name := fs.String("name", "", "human-readable name")
		if err := fs.Parse(rest); err != nil {
			return err
		}
		if *slug == "" || *name == "" {
			return errors.New("--slug and --name required")
		}
		cfg, err := loadConfig()
		if err != nil {
			return err
		}
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		db, err := fmysql.Open(ctx, cfg.MySQL)
		if err != nil {
			return err
		}
		defer func() { _ = db.Close() }()
		b := fmysql.NewBootstrap(db, argonParamsFromConfig(cfg.Auth))
		id, err := b.CreateOrg(ctx, *slug, *name)
		if err != nil {
			return err
		}
		fmt.Printf("organization created: id=%s slug=%s\n", id, *slug)
		return nil
	default:
		return fmt.Errorf("tenant: unknown action %q", action)
	}
}

// --- user --------------------------------------------------------------------

func runUser(args []string) error {
	if len(args) == 0 {
		return errors.New("user: missing action (create)")
	}
	action, rest := args[0], args[1:]
	switch action {
	case "create":
		fs := flag.NewFlagSet("user create", flag.ExitOnError)
		orgSlug := fs.String("org-slug", "", "organization slug")
		email := fs.String("email", "", "email address")
		password := fs.String("password", "", "password")
		admin := fs.Bool("admin", false, "grant the built-in admin role")
		if err := fs.Parse(rest); err != nil {
			return err
		}
		if *orgSlug == "" || *email == "" || *password == "" {
			return errors.New("--org-slug, --email, --password required")
		}
		cfg, err := loadConfig()
		if err != nil {
			return err
		}
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		db, err := fmysql.Open(ctx, cfg.MySQL)
		if err != nil {
			return err
		}
		defer func() { _ = db.Close() }()
		b := fmysql.NewBootstrap(db, argonParamsFromConfig(cfg.Auth))
		orgID, err := b.LookupOrgBySlug(ctx, *orgSlug)
		if err != nil {
			return err
		}
		uid, err := b.CreateUser(ctx, orgID, *email, *password)
		if err != nil {
			return err
		}
		fmt.Printf("user created: id=%s org=%s email=%s\n", uid, *orgSlug, *email)
		if *admin {
			rid, err := b.EnsureAdminRole(ctx, orgID)
			if err != nil {
				return err
			}
			if err := b.AssignRole(ctx, uid, rid); err != nil {
				return err
			}
			fmt.Printf("admin role assigned: role_id=%s\n", rid)
		}
		// Reference symbols to keep imports live even in trimmed builds.
		_ = appauth.ErrInvalidCredentials
		_ = infauth.DefaultArgon2Params
		return nil
	default:
		return fmt.Errorf("user: unknown action %q", action)
	}
}

// --- migrate -----------------------------------------------------------------

func runMigrate(args []string) error {
	if len(args) == 0 {
		return errors.New("migrate: expected up|down|status")
	}
	direction := args[0]
	rest := args[1:]
	fs := flag.NewFlagSet("migrate", flag.ExitOnError)
	steps := fs.Int("steps", 0, "number of steps (default: all)")
	if err := fs.Parse(rest); err != nil {
		return err
	}
	if _, err := exec.LookPath("migrate"); err != nil {
		fmt.Fprintln(os.Stderr, `The `+"`migrate`"+` CLI is required. Install with:
  brew install golang-migrate
  # or: go install -tags 'mysql' github.com/golang-migrate/migrate/v4/cmd/migrate@latest`)
		return fmt.Errorf("migrate binary not found: %w", err)
	}
	cfg, err := loadConfig()
	if err != nil {
		return err
	}
	// The migrate CLI needs a mysql:// URL, not a Go driver DSN.
	url, err := driverDSNToMigrateURL(cfg.MySQL.DSN)
	if err != nil {
		return err
	}
	cmdArgs := []string{"-database", url, "-path", "migrations"}
	switch direction {
	case "up":
		cmdArgs = append(cmdArgs, "up")
		if *steps > 0 {
			cmdArgs = append(cmdArgs, fmt.Sprint(*steps))
		}
	case "down":
		cmdArgs = append(cmdArgs, "down")
		if *steps > 0 {
			cmdArgs = append(cmdArgs, fmt.Sprint(*steps))
		} else {
			cmdArgs = append(cmdArgs, "-all")
		}
	case "status", "version":
		cmdArgs = append(cmdArgs, "version")
	default:
		return fmt.Errorf("migrate: unknown direction %q", direction)
	}
	c := exec.Command("migrate", cmdArgs...) //nolint:gosec // args composed from static direction + configured DSN.
	c.Stdout = os.Stdout
	c.Stderr = os.Stderr
	return c.Run()
}

// driverDSNToMigrateURL converts a go-sql-driver DSN
// (user:pass@tcp(host:port)/db?params) to the URL form the migrate CLI expects
// (mysql://user:pass@tcp(host:port)/db?params).
func driverDSNToMigrateURL(dsn string) (string, error) {
	if dsn == "" {
		return "", errors.New("empty DSN")
	}
	return "mysql://" + dsn, nil
}

// argonParamsFromConfig returns argon2 parameters from AuthConfig (or defaults).
// The config parser doesn't expose the sub-fields yet; we use the documented
// defaults here so behavior matches config/example.yaml.
func argonParamsFromConfig(_ config.AuthConfig) infauth.Argon2Params {
	return infauth.DefaultArgon2Params()
}
