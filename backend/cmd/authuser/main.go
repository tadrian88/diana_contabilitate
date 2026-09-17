// Command authuser is the operator-only credential provisioning boundary.
// It exposes no HTTP route and never prints or persists plaintext passwords.
package main

import (
	"bufio"
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"strings"
	"syscall"
	"time"

	"diana-contabilitate/backend/internal/authentication"
	"diana-contabilitate/backend/internal/platform/postgres"
	"golang.org/x/term"
)

func main() {
	if len(os.Args) < 2 {
		fatal("usage: authuser provision|reset-password --email EMAIL [--all-clients]")
	}
	flags := flag.NewFlagSet(os.Args[1], flag.ExitOnError)
	email := flags.String("email", "", "login email")
	allClients := flags.Bool("all-clients", false, "grant access to all existing and future clients")
	_ = flags.Parse(os.Args[2:])
	if authentication.NormalizeEmail(*email) == "" {
		fatal("--email is required")
	}
	password, err := readPassword()
	if err != nil {
		fatal(err.Error())
	}
	hash, err := authentication.HashPassword(password)
	password = ""
	if err != nil {
		fatal(err.Error())
	}
	databaseURL := os.Getenv("DATABASE_URL")
	if databaseURL == "" {
		fatal("DATABASE_URL is required")
	}
	store, err := postgres.Open(databaseURL)
	if err != nil {
		fatal("database connection failed")
	}
	defer store.Close()
	authStore := authentication.SQLStore{DB: store.DB}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	switch os.Args[1] {
	case "provision":
		result, err := authStore.Provision(ctx, *email, hash, *allClients, time.Now())
		if errors.Is(err, authentication.ErrAlreadyExists) {
			fmt.Println("already exists")
			os.Exit(2)
		}
		if err != nil {
			fatal("provisioning failed")
		}
		fmt.Println(result)
	case "reset-password":
		result, err := authStore.ResetPassword(ctx, *email, hash, time.Now())
		if errors.Is(err, authentication.ErrNotFound) {
			fmt.Println("not found")
			os.Exit(2)
		}
		if err != nil {
			fatal("password reset failed")
		}
		fmt.Println(result)
	default:
		fatal("usage: authuser provision|reset-password --email EMAIL [--all-clients]")
	}
}

func readPassword() (string, error) {
	if value := os.Getenv("DIANA_AUTH_PASSWORD"); value != "" {
		_ = os.Unsetenv("DIANA_AUTH_PASSWORD")
		return value, nil
	}
	if term.IsTerminal(int(syscall.Stdin)) {
		fmt.Fprint(os.Stderr, "Password: ")
		first, err := term.ReadPassword(int(syscall.Stdin))
		fmt.Fprintln(os.Stderr)
		if err != nil {
			return "", err
		}
		fmt.Fprint(os.Stderr, "Confirm password: ")
		second, err := term.ReadPassword(int(syscall.Stdin))
		fmt.Fprintln(os.Stderr)
		if err != nil {
			return "", err
		}
		if string(first) != string(second) {
			return "", errors.New("passwords do not match")
		}
		return string(first), nil
	}
	line, err := bufio.NewReader(os.Stdin).ReadString('\n')
	if err != nil && len(line) == 0 {
		return "", errors.New("password must be supplied on stdin")
	}
	return strings.TrimRight(line, "\r\n"), nil
}

func fatal(message string) { fmt.Fprintln(os.Stderr, message); os.Exit(1) }
