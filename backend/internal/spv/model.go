package spv

import (
	"context"
	"errors"
	"time"

	"diana-contabilitate/backend/internal/invoicing"
)

const (
	SourceName    = "ANAF_SPV"
	ParserTypeUBL = "UBL_2_1_CIUS_RO"
	ParserVersion = "DIANA_UBL_V1"
)

var (
	ErrPermanent                = errors.New("permanent SPV ingestion failure")
	ErrTransient                = errors.New("transient SPV ingestion failure")
	ErrDocumentClaimed          = errors.New("SPV source document is already claimed")
	ErrInvalidOAuthState        = errors.New("invalid, expired, or consumed SPV OAuth state")
	ErrReauthenticationRequired = errors.New("SPV connection requires reauthentication")
	ErrConnectionInactive       = errors.New("SPV connection is inactive")
	ErrConfiguration            = errors.New("SPV application configuration is incomplete")
)

type Connection struct {
	ID, ClientID, CIF, Environment                     string
	AccessTokenCiphertext, RefreshTokenCiphertext      string
	AccessTokenExpiresAt                               time.Time
	RefreshTokenExpiresAt, LastSuccessfulSyncAt        *time.Time
	ConnectedAt, LastSyncStartedAt, LastSyncFinishedAt *time.Time
	Status                                             string
	LastSyncStatus, LastError                          string
	Revision                                           uint64
}

type OAuthState struct {
	ID, StateHash, ClientID, Environment, ReturnPath string
	ExpiresAt, CreatedAt                             time.Time
	ConsumedAt                                       *time.Time
}

type Actor struct{ ID, Display, CorrelationID string }

type ConnectionStore interface {
	ClientCUI(context.Context, string) (string, error)
	ConnectionByClient(context.Context, string) (*Connection, error)
	CreateOAuthState(context.Context, OAuthState) error
	ConsumeOAuthState(context.Context, string, time.Time) (OAuthState, error)
	CompleteOAuthConnection(context.Context, OAuthState, string, string, time.Time, *time.Time, Actor, time.Time) (Connection, bool, error)
	DisconnectSPVConnection(context.Context, string, string, Actor, time.Time) (Connection, bool, error)
	RecordManualSyncRequested(context.Context, Connection, string, Actor, time.Time) (bool, error)
}

type OAuthClient interface {
	ExchangeToken(context.Context, string, string, string, string) (TokenResponse, error)
}

type SyncPublisher interface {
	PublishSPVSync(context.Context, string) (string, error)
}

type Message struct {
	ID, UploadID, RequestID, Type, CreatedRaw string
	CreatedAt                                 *time.Time
}

type SourceDocument struct {
	ID, ConnectionID, ClientID, ExternalMessageID string
	Status, FailureKind                           string
	RawDocument                                   []byte
	ContentSHA256, ContentType                    string
	Attempts                                      uint
}

type ParsedDocument struct {
	Invoice  invoicing.IngestionInput
	BuyerCUI string
	Format   string
	Version  string
}

type DocumentStore interface {
	ListActiveConnections(context.Context) ([]Connection, error)
	GetConnection(context.Context, string) (Connection, error)
	SaveTokens(context.Context, string, uint64, string, string, time.Time, *time.Time, time.Time) (Connection, error)
	MarkSyncStarted(context.Context, string, time.Time) error
	MarkSyncFinished(context.Context, string, time.Time, error) error
	Discover(context.Context, Connection, Message, time.Time) (SourceDocument, bool, error)
	ClaimSourceDocument(context.Context, string, string, time.Time, time.Duration) (SourceDocument, error)
	StoreRaw(context.Context, string, string, []byte, string, string, time.Time) error
	MarkSourceFailed(context.Context, string, string, string, string, time.Time, time.Time) error
	MarkProcessed(context.Context, string, string, string, string, string, time.Time) error
}

type InvoiceDocumentParser interface {
	Parse([]byte) (ParsedDocument, error)
}

type SPVClient interface {
	ListIncoming(context.Context, string, string, time.Time, time.Time, int) ([]Message, int, error)
	Download(context.Context, string, string) ([]byte, string, error)
	RefreshToken(context.Context, string, string, string) (TokenResponse, error)
}

type InvoiceIngester interface {
	Ingest(context.Context, invoicing.IngestionInput) (*invoicing.Invoice, bool, error)
}

type TokenResponse struct {
	AccessToken, RefreshToken   string
	ExpiresIn, RefreshExpiresIn time.Duration
}

type TokenCipher interface {
	Encrypt(string) (string, error)
	Decrypt(string) (string, error)
}

type SyncResult struct {
	Documents []SourceDocument
	Pages     int
}
