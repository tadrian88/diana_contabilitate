// Command spvconnect performs the operator-only OAuth bootstrap. It deliberately
// does not expose an unauthenticated HTTP credential-management endpoint.
package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"time"

	"diana-contabilitate/backend/internal/platform/config"
	"diana-contabilitate/backend/internal/platform/postgres"
	"diana-contabilitate/backend/internal/spv"
)

func main() {
	if len(os.Args) < 2 {
		fatal("usage: spvconnect authorize|exchange")
	}
	cfg, err := config.Load()
	if err != nil {
		fatal(err.Error())
	}
	switch os.Args[1] {
	case "authorize":
		flags := flag.NewFlagSet("authorize", flag.ExitOnError)
		redirect := flags.String("redirect-uri", "", "registered OAuth redirect URI")
		state := flags.String("state", "", "cryptographically random one-time state")
		_ = flags.Parse(os.Args[2:])
		if *redirect == "" || *state == "" || cfg.SPVOAuthClientID == "" {
			fatal("redirect-uri, state and SPV_OAUTH_CLIENT_ID are required")
		}
		fmt.Println(spv.AuthorizationURL(cfg.SPVOAuthClientID, *redirect, *state))
	case "exchange":
		flags := flag.NewFlagSet("exchange", flag.ExitOnError)
		clientID := flags.String("accounting-client-id", "", "Diana accounting client ID")
		code := flags.String("code", "", "OAuth authorization code")
		redirect := flags.String("redirect-uri", "", "same registered OAuth redirect URI")
		_ = flags.Parse(os.Args[2:])
		if *clientID == "" || *code == "" || *redirect == "" {
			fatal("accounting-client-id, code and redirect-uri are required")
		}
		cipher, err := spv.NewAESGCMCipher(cfg.SPVTokenEncryptionKey)
		if err != nil {
			fatal(err.Error())
		}
		api := spv.NewHTTPClient(nil, cfg.SPVAPIBaseURL, cfg.SPVTokenURL)
		token, err := api.ExchangeToken(context.Background(), *code, cfg.SPVOAuthClientID, cfg.SPVOAuthClientSecret, *redirect)
		if err != nil {
			fatal(err.Error())
		}
		access, err := cipher.Encrypt(token.AccessToken)
		if err != nil {
			fatal(err.Error())
		}
		refresh, err := cipher.Encrypt(token.RefreshToken)
		if err != nil {
			fatal(err.Error())
		}
		store, err := postgres.Open(cfg.DatabaseURL)
		if err != nil {
			fatal(err.Error())
		}
		defer store.Close()
		now := time.Now().UTC()
		var refreshExpires *time.Time
		if token.RefreshExpiresIn > 0 {
			value := now.Add(token.RefreshExpiresIn)
			refreshExpires = &value
		}
		connection, err := store.InstallSPVConnection(context.Background(), *clientID, cfg.SPVEnvironment, access, refresh, now.Add(token.ExpiresIn), refreshExpires, now)
		if err != nil {
			fatal(err.Error())
		}
		fmt.Println(connection.ID)
	default:
		fatal("usage: spvconnect authorize|exchange")
	}
}
func fatal(message string) { fmt.Fprintln(os.Stderr, message); os.Exit(1) }
