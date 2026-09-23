// Command spvconnect performs the operator-only OAuth bootstrap. It deliberately
// does not expose an unauthenticated HTTP credential-management endpoint.
package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"flag"
	"fmt"
	"os"
	"strings"
	"time"

	"diana-contabilitate/backend/internal/apperrors"
	"diana-contabilitate/backend/internal/fiscalidentity"
	"diana-contabilitate/backend/internal/invoicing"
	"diana-contabilitate/backend/internal/platform/config"
	"diana-contabilitate/backend/internal/platform/postgres"
	"diana-contabilitate/backend/internal/spv"
)

func main() {
	if len(os.Args) < 2 {
		fatal(usage)
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
	case "import-fixture":
		flags := flag.NewFlagSet("import-fixture", flag.ExitOnError)
		clientID := flags.String("accounting-client-id", "", "Diana accounting client ID")
		zipPath := flags.String("zip", "", "path to the original ANAF ZIP")
		externalMessageID := flags.String("external-message-id", "", "ANAF message/download ID")
		_ = flags.Parse(os.Args[2:])
		if *clientID == "" || *zipPath == "" || *externalMessageID == "" {
			fatal("accounting-client-id, zip and external-message-id are required")
		}
		if err = importFixture(context.Background(), cfg, fixtureImport{
			ClientID: *clientID, ZIPPath: *zipPath, ExternalMessageID: *externalMessageID,
		}); err != nil {
			fatal(err.Error())
		}
	default:
		fatal(usage)
	}
}

const usage = "usage: spvconnect authorize|exchange|import-fixture"

type fixtureImport struct {
	ClientID, ZIPPath, ExternalMessageID string
}

func importFixture(ctx context.Context, cfg config.Config, input fixtureImport) error {
	if cfg.Environment != "development" && cfg.Environment != "test" {
		return fmt.Errorf("import-fixture is allowed only in development or test")
	}
	raw, err := os.ReadFile(input.ZIPPath)
	if err != nil {
		return fmt.Errorf("read ANAF ZIP: %w", err)
	}
	if len(raw) == 0 {
		return fmt.Errorf("ANAF ZIP is empty")
	}
	parsed, err := (spv.UBLParser{}).Parse(raw)
	if err != nil {
		return fmt.Errorf("validate ANAF ZIP: %w", err)
	}
	store, err := postgres.Open(cfg.DatabaseURL)
	if err != nil {
		return fmt.Errorf("database setup: %w", err)
	}
	defer store.Close()
	clientCUI, err := store.ClientCUI(ctx, input.ClientID)
	if err != nil {
		return fmt.Errorf("load accounting client: %w", err)
	}
	if !fiscalidentity.SameRomanian(parsed.BuyerCUI, clientCUI) {
		return fmt.Errorf("fixture buyer CUI %q does not match client CUI %q", parsed.BuyerCUI, clientCUI)
	}
	connection, err := store.ConnectionByClient(ctx, input.ClientID)
	if err != nil {
		if errors.Is(err, apperrors.ErrNotFound) {
			return fmt.Errorf("client has no simulated SPV connection")
		}
		return fmt.Errorf("load SPV connection: %w", err)
	}
	if connection.Status != "ACTIVE" {
		return fmt.Errorf("SPV connection is %s, expected ACTIVE", connection.Status)
	}
	if !fiscalidentity.SameRomanian(connection.CIF, clientCUI) {
		return fmt.Errorf("SPV connection CUI %q does not match client CUI %q", connection.CIF, clientCUI)
	}
	if !connection.AccessTokenExpiresAt.After(time.Now().UTC().Add(time.Minute)) {
		return fmt.Errorf("simulated SPV access token must remain valid for at least one minute")
	}

	now := time.Now().UTC()
	// The local fixture has no ANAF list response. Use the invoice issue day as
	// an explicit synthetic SPV timestamp only in development/test so the full
	// automatic remittance flow can be exercised without pretending this is
	// production evidence.
	fixtureCreatedAt := parsed.Invoice.IssueDate
	document, _, err := store.Discover(ctx, *connection, spv.Message{
		ID: input.ExternalMessageID, Type: "FACTURA PRIMITA", CreatedRaw: "LOCAL_FIXTURE_FROM_INVOICE_ISSUE_DATE:" + fixtureCreatedAt.Format("2006-01-02"), CreatedAt: &fixtureCreatedAt,
	}, now)
	if err != nil {
		return fmt.Errorf("discover fixture delivery: %w", err)
	}
	wantedHash := sha256.Sum256(raw)
	row, err := store.Client.SPVSourceDocument.Get(ctx, document.ID)
	if err != nil {
		return fmt.Errorf("reload fixture delivery: %w", err)
	}
	if row.ContentSha256 != nil && !strings.EqualFold(*row.ContentSha256, hex.EncodeToString(wantedHash[:])) {
		return fmt.Errorf("external message ID already belongs to different content")
	}
	if row.InvoiceID != nil && row.ProcessingStatus == "PROCESSED" {
		fmt.Printf("source_document_id=%s invoice_id=%s created=false status=PROCESSED\n", row.ID, *row.InvoiceID)
		return nil
	}
	if document.FailureKind == "PERMANENT" {
		return fmt.Errorf("source document is permanently failed; inspect its recorded last_error")
	}

	pipeline := invoicing.NewPipelineService(store, nil, nil)
	service := spv.NewService(store, fixtureSPVClient{raw: raw}, spv.UBLParser{}, pipeline, fixtureCipher{}, spv.ServiceConfig{
		ClaimTTL: cfg.SPVClaimTTL, RetryDelay: cfg.DispatcherRetryMin,
	})
	invoiceID, created, err := service.ProcessDocument(ctx, document.ID, "local-spv-fixture")
	if err != nil {
		return fmt.Errorf("process fixture delivery: %w", err)
	}
	fmt.Printf("source_document_id=%s invoice_id=%s created=%t status=PROCESSED\n", document.ID, invoiceID, created)
	return nil
}

type fixtureSPVClient struct{ raw []byte }

func (c fixtureSPVClient) Download(context.Context, string, string) ([]byte, string, error) {
	return append([]byte(nil), c.raw...), "application/zip", nil
}
func (fixtureSPVClient) ListIncoming(context.Context, string, string, time.Time, time.Time, int) ([]spv.Message, int, error) {
	return nil, 0, fmt.Errorf("fixture client does not support listing")
}
func (fixtureSPVClient) RefreshToken(context.Context, string, string, string) (spv.TokenResponse, error) {
	return spv.TokenResponse{}, fmt.Errorf("%w: fixture connection token expired", spv.ErrPermanent)
}

type fixtureCipher struct{}

func (fixtureCipher) Encrypt(value string) (string, error) { return value, nil }
func (fixtureCipher) Decrypt(string) (string, error)       { return "local-fixture-token", nil }

func fatal(message string) { fmt.Fprintln(os.Stderr, message); os.Exit(1) }
