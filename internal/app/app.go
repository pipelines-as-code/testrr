package app

import (
	"bufio"
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"strings"
	"time"

	"testrr/internal/auth"
	"testrr/internal/config"
	"testrr/internal/httpserver"
	"testrr/internal/parser"
	"testrr/internal/store"
)

var ErrUsage = errors.New("usage error")

func Run(ctx context.Context, args []string) error {
	cfg := config.Load()

	command := "serve"
	if len(args) > 0 {
		command = args[0]
		args = args[1:]
	}

	repository, err := store.Open(ctx, cfg.DatabaseURL)
	if err != nil {
		return err
	}
	defer repository.Close()

	switch command {
	case "serve":
		if cfg.AutoMigrate {
			if err := repository.Migrate(ctx); err != nil {
				return err
			}
		}
		server, err := httpserver.New(cfg, repository, parser.NewRegistry(
			parser.NewJUnitParser(),
			parser.NewTRXParser(),
			parser.NewNUnitParser(),
			parser.NewGoTestJSONParser(),
		))
		if err != nil {
			return err
		}
		httpServer := &http.Server{
			Addr:              cfg.Addr,
			Handler:           server,
			ReadHeaderTimeout: 10 * time.Second,
		}
		log.Printf("testrr: starting server on http://localhost%s data_dir=%s database=%s", cfg.Addr, cfg.DataDir, cfg.DatabaseURL)
		return httpServer.ListenAndServe()
	case "migrate":
		return repository.Migrate(ctx)
	case "project":
		return runProjectCommand(ctx, repository, args)
	default:
		return fmt.Errorf("%w: unknown command %q", ErrUsage, command)
	}
}

func runProjectCommand(ctx context.Context, repository store.Repository, args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("%w: expected project subcommand", ErrUsage)
	}

	switch args[0] {
	case "create":
		return projectCreate(ctx, repository, args[1:])
	case "rotate-password":
		return projectRotatePassword(ctx, repository, args[1:])
	case "list":
		return projectList(ctx, repository)
	default:
		return fmt.Errorf("%w: unknown project subcommand %q", ErrUsage, args[0])
	}
}

func projectCreate(ctx context.Context, repository store.Repository, args []string) error {
	fs := flag.NewFlagSet("project create", flag.ContinueOnError)
	var slug string
	var name string
	var username string
	var passwordStdin bool
	fs.StringVar(&slug, "slug", "", "project slug")
	fs.StringVar(&name, "name", "", "project name")
	fs.StringVar(&username, "username", "", "project username")
	fs.BoolVar(&passwordStdin, "password-stdin", false, "read password from stdin")
	if err := fs.Parse(args); err != nil {
		return fmt.Errorf("%w: %v", ErrUsage, err)
	}
	if slug == "" || name == "" || username == "" || !passwordStdin {
		return fmt.Errorf("%w: project create requires --slug, --name, --username, and --password-stdin", ErrUsage)
	}

	password, err := readPassword()
	if err != nil {
		return err
	}
	hashed, err := auth.HashPassword(password)
	if err != nil {
		return err
	}

	_, err = repository.CreateProject(ctx, store.CreateProjectInput{
		ID:           newID(),
		Slug:         slug,
		Name:         name,
		Username:     username,
		PasswordHash: hashed,
		CreatedAt:    time.Now().UTC(),
	})
	return err
}

func projectRotatePassword(ctx context.Context, repository store.Repository, args []string) error {
	fs := flag.NewFlagSet("project rotate-password", flag.ContinueOnError)
	var slug string
	var username string
	var passwordStdin bool
	fs.StringVar(&slug, "slug", "", "project slug")
	fs.StringVar(&username, "username", "", "project username")
	fs.BoolVar(&passwordStdin, "password-stdin", false, "read password from stdin")
	if err := fs.Parse(args); err != nil {
		return fmt.Errorf("%w: %v", ErrUsage, err)
	}
	if slug == "" || username == "" || !passwordStdin {
		return fmt.Errorf("%w: rotate-password requires --slug, --username, and --password-stdin", ErrUsage)
	}

	password, err := readPassword()
	if err != nil {
		return err
	}
	hashed, err := auth.HashPassword(password)
	if err != nil {
		return err
	}
	return repository.RotateProjectCredential(ctx, slug, username, hashed)
}

func projectList(ctx context.Context, repository store.Repository) error {
	projects, err := repository.ListProjects(ctx)
	if err != nil {
		return err
	}
	for _, project := range projects {
		fmt.Printf("%s\t%s\t%s\n", project.Slug, project.Name, project.CreatedAt.Format(time.RFC3339))
	}
	return nil
}

func readPassword() (string, error) {
	reader := bufio.NewReader(os.Stdin)
	password, err := reader.ReadString('\n')
	if err != nil && !errors.Is(err, os.ErrClosed) {
		password = strings.TrimSpace(password)
		if password != "" {
			return password, nil
		}
	}
	password = strings.TrimSpace(password)
	if password == "" {
		return "", fmt.Errorf("%w: no password read from stdin", ErrUsage)
	}
	return password, nil
}

func newID() string {
	value := make([]byte, 16)
	rand.Read(value)
	return hex.EncodeToString(value)
}
