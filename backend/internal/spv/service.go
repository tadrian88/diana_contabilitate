package spv

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"time"

	"diana-contabilitate/backend/internal/fiscalidentity"
	"diana-contabilitate/backend/internal/invoicing"
)

type ServiceConfig struct {
	OAuthClientID, OAuthClientSecret string
	Environment                      string
	InitialWindow, Overlap, ClaimTTL time.Duration
	RetryDelay                       time.Duration
}

type Service struct {
	store    DocumentStore
	client   SPVClient
	parser   InvoiceDocumentParser
	invoices InvoiceIngester
	cipher   TokenCipher
	config   ServiceConfig
	clock    func() time.Time
}

func NewService(store DocumentStore, client SPVClient, parser InvoiceDocumentParser, invoices InvoiceIngester, cipher TokenCipher, cfg ServiceConfig) *Service {
	if cfg.InitialWindow <= 0 {
		cfg.InitialWindow = 60 * 24 * time.Hour
	}
	if cfg.Overlap <= 0 {
		cfg.Overlap = 72 * time.Hour
	}
	if cfg.ClaimTTL <= 0 {
		cfg.ClaimTTL = 10 * time.Minute
	}
	if cfg.RetryDelay <= 0 {
		cfg.RetryDelay = time.Minute
	}
	return &Service{store: store, client: client, parser: parser, invoices: invoices, cipher: cipher, config: cfg, clock: func() time.Time { return time.Now().UTC() }}
}

func (s *Service) Connections(ctx context.Context) ([]Connection, error) {
	return s.store.ListActiveConnections(ctx)
}

func (s *Service) Sync(ctx context.Context, connectionID string) (result SyncResult, returnedErr error) {
	now := s.clock()
	connection, err := s.store.GetConnection(ctx, connectionID)
	if err != nil {
		return result, err
	}
	if guard, ok := s.store.(interface {
		ClientOperational(context.Context, string) (bool, error)
	}); ok {
		allowed, e := guard.ClientOperational(ctx, connection.ClientID)
		if e != nil {
			return result, e
		}
		if !allowed {
			return result, fmt.Errorf("%w: inactive client", ErrPermanent)
		}
	}
	if connection.Status != "ACTIVE" {
		return result, fmt.Errorf("%w: inactive SPV connection", ErrPermanent)
	}
	if s.config.Environment != "" && connection.Environment != s.config.Environment {
		return result, fmt.Errorf("%w: SPV connection environment mismatch", ErrPermanent)
	}
	_ = s.store.MarkSyncStarted(ctx, connection.ID, now)
	defer func() {
		_ = s.store.MarkSyncFinished(context.WithoutCancel(ctx), connection.ID, s.clock(), returnedErr)
	}()
	token, err := s.accessToken(ctx, &connection)
	if err != nil {
		return result, err
	}
	// Received (P) and sent (T) invoices are listed with their own windows
	// (D-125); a document is kept once even if both lists return it.
	seen := map[string]bool{}
	for _, list := range []struct {
		filter MessageFilter
		since  *time.Time
	}{{MessageFilterReceived, connection.LastSuccessfulSyncAt}, {MessageFilterSent, connection.LastSuccessfulSentSyncAt}} {
		start := s.windowStart(now, list.since)
		pages := 1
		for page := 1; page <= pages; page++ {
			messages, totalPages, listErr := s.client.ListMessages(ctx, token, fiscalidentity.ForComparison(connection.CIF, "RO"), list.filter, start, now, page)
			if listErr != nil {
				return result, listErr
			}
			if totalPages > pages {
				pages = totalPages
			}
			result.Pages += 1
			for _, message := range messages {
				document, _, discoverErr := s.store.Discover(ctx, connection, message, now)
				if discoverErr != nil {
					return result, discoverErr
				}
				if seen[document.ID] {
					continue
				}
				seen[document.ID] = true
				result.Documents = append(result.Documents, document)
			}
		}
	}
	return result, nil
}

func (s *Service) windowStart(now time.Time, since *time.Time) time.Time {
	minimum := now.Add(-s.config.InitialWindow)
	if since == nil {
		return minimum
	}
	start := since.Add(-s.config.Overlap)
	if start.Before(minimum) {
		return minimum
	}
	return start
}

// ResolveDirection decides from the parsed XML parties whether the client
// received the invoice or issued it (D-125). A buyer equal to the client wins,
// so self-billing stays a received invoice; a document naming neither party is
// not the client's.
func ResolveDirection(parsed ParsedDocument, clientCUI string) (invoicing.Direction, error) {
	switch {
	case fiscalidentity.SameRomanian(parsed.BuyerCUI, clientCUI):
		return invoicing.DirectionIncoming, nil
	case fiscalidentity.SameRomanian(parsed.SupplierCUI, clientCUI):
		return invoicing.DirectionOutgoing, nil
	}
	return "", fmt.Errorf("%w: neither invoice party matches the target client", ErrPermanent)
}

func (s *Service) ProcessDocument(ctx context.Context, documentID, owner string) (invoiceID string, created bool, returnedErr error) {
	now := s.clock()
	document, err := s.store.ClaimSourceDocument(ctx, documentID, owner, now, s.config.ClaimTTL)
	if err != nil {
		return "", false, err
	}
	fail := func(kind string, cause error) (string, bool, error) {
		available := now
		if kind == "TRANSIENT" {
			available = now.Add(s.config.RetryDelay)
		}
		safe := safeError(cause)
		_ = s.store.MarkSourceFailed(context.WithoutCancel(ctx), document.ID, owner, kind, safe, available, s.clock())
		return "", false, cause
	}
	connection, err := s.store.GetConnection(ctx, document.ConnectionID)
	if err != nil {
		return fail("TRANSIENT", err)
	}
	raw := document.RawDocument
	hash := document.ContentSHA256
	contentType := document.ContentType
	if len(raw) == 0 {
		token, tokenErr := s.accessToken(ctx, &connection)
		if tokenErr != nil {
			if errors.Is(tokenErr, ErrReauthenticationRequired) {
				_ = s.store.MarkSyncFinished(context.WithoutCancel(ctx), connection.ID, s.clock(), tokenErr)
			}
			return fail(errorKind(tokenErr), tokenErr)
		}
		raw, contentType, err = s.client.Download(ctx, token, document.ExternalMessageID)
		if err != nil {
			return fail(errorKind(err), err)
		}
		sum := sha256.Sum256(raw)
		hash = hex.EncodeToString(sum[:])
		if err = s.store.StoreRaw(ctx, document.ID, owner, raw, contentType, hash, s.clock()); err != nil {
			return fail("TRANSIENT", err)
		}
	}
	parsed, err := s.parser.Parse(raw)
	if err != nil {
		return fail(errorKind(err), err)
	}
	direction, err := ResolveDirection(parsed, connection.CIF)
	if err != nil {
		return fail("PERMANENT", err)
	}
	parsed.Invoice.Direction = direction
	if direction == invoicing.DirectionOutgoing && parsed.Invoice.CustomerName == "" {
		return fail("PERMANENT", fmt.Errorf("%w: issued invoice has no customer name", ErrPermanent))
	}
	if direction == invoicing.DirectionIncoming {
		// The customer of a received invoice is the client itself.
		parsed.Invoice.CustomerName, parsed.Invoice.CustomerIdentifier = "", ""
	}
	if parsed.Invoice.SourceFacts != nil {
		parsed.Invoice.SourceFacts.SourceDocumentID = document.ID
		parsed.Invoice.SourceFacts.SourceHash = hash
		parsed.Version = parsed.Invoice.SourceFacts.ParserVersion
	}
	parsed.Invoice.ClientID = connection.ClientID
	parsed.Invoice.ExternalDeliveryID = document.ExternalMessageID
	parsed.Invoice.SPVReference = "ANAF:" + connection.ID + ":" + document.ExternalMessageID
	invoice, created, err := s.invoices.Ingest(ctx, parsed.Invoice)
	if err != nil {
		return fail("TRANSIENT", err)
	}
	if err = s.store.MarkProcessed(ctx, document.ID, owner, invoice.ID, parsed.Format, parsed.Version, s.clock()); err != nil {
		return "", created, err
	}
	return invoice.ID, created, nil
}

func (s *Service) accessToken(ctx context.Context, connection *Connection) (string, error) {
	access, err := s.cipher.Decrypt(connection.AccessTokenCiphertext)
	if err != nil {
		return "", fmt.Errorf("%w: decrypt access token", ErrPermanent)
	}
	if s.clock().Before(connection.AccessTokenExpiresAt.Add(-time.Minute)) {
		return access, nil
	}
	refresh, err := s.cipher.Decrypt(connection.RefreshTokenCiphertext)
	if err != nil {
		return "", fmt.Errorf("%w: decrypt refresh token", ErrPermanent)
	}
	if s.config.OAuthClientID == "" || s.config.OAuthClientSecret == "" {
		return "", fmt.Errorf("%w: OAuth application credentials unavailable", ErrPermanent)
	}
	response, err := s.client.RefreshToken(ctx, refresh, s.config.OAuthClientID, s.config.OAuthClientSecret)
	if err != nil {
		if errors.Is(err, ErrPermanent) {
			return "", errors.Join(ErrReauthenticationRequired, err)
		}
		return "", err
	}
	encAccess, err := s.cipher.Encrypt(response.AccessToken)
	if err != nil {
		return "", err
	}
	encRefresh, err := s.cipher.Encrypt(response.RefreshToken)
	if err != nil {
		return "", err
	}
	expires := s.clock().Add(response.ExpiresIn)
	var refreshExpires *time.Time
	if response.RefreshExpiresIn > 0 {
		value := s.clock().Add(response.RefreshExpiresIn)
		refreshExpires = &value
	}
	updated, err := s.store.SaveTokens(ctx, connection.ID, connection.Revision, encAccess, encRefresh, expires, refreshExpires, s.clock())
	if err != nil {
		return "", err
	}
	*connection = updated
	return response.AccessToken, nil
}

func errorKind(err error) string {
	if errors.Is(err, ErrPermanent) {
		return "PERMANENT"
	}
	return "TRANSIENT"
}
func safeError(err error) string {
	if err == nil {
		return ""
	}
	value := err.Error()
	if len(value) > 500 {
		value = value[:500]
	}
	return value
}
