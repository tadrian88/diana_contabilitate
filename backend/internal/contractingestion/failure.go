package contractingestion

import (
	"errors"
	"fmt"
)

type ExtractionRetryPolicy string

const (
	RetryNever    ExtractionRetryPolicy = "NEVER"
	RetryStandard ExtractionRetryPolicy = "STANDARD"
	RetryOnce     ExtractionRetryPolicy = "ONCE"
)

const (
	FailureExtractorConfiguration   = "EXTRACTOR_CONFIGURATION"
	FailureSourceIntegrity          = "SOURCE_INTEGRITY"
	FailureProviderAuthentication   = "PROVIDER_AUTHENTICATION"
	FailureProviderRejected         = "PROVIDER_REJECTED"
	FailureProviderRateLimit        = "PROVIDER_RATE_LIMIT"
	FailureProviderUnavailable      = "PROVIDER_UNAVAILABLE"
	FailureProviderNetwork          = "PROVIDER_NETWORK"
	FailureTimeout                  = "TIMEOUT"
	FailureInterrupted              = "INTERRUPTED"
	FailureInvalidProviderEnvelope  = "INVALID_PROVIDER_ENVELOPE"
	FailureInvalidStructuredOutput  = "INVALID_STRUCTURED_OUTPUT"
	FailureProposalValidationFailed = "PROPOSAL_VALIDATION_FAILED"
	FailureNoContractData           = "NO_CONTRACT_DATA"
	FailureLeaseExpired             = "LEASE_EXPIRED"
)

// ExtractionFailure contains only diagnostics that are safe to persist and log.
// Its cause is deliberately omitted from Error to prevent provider or document
// content from leaking through worker logs.
type ExtractionFailure struct {
	Category       string
	Retry          ExtractionRetryPolicy
	HTTPStatus     int
	Provider       string
	Model          string
	ValidationCode string
	ValidationPath string
	cause          error
}

func (e *ExtractionFailure) Error() string {
	if e.HTTPStatus != 0 {
		return fmt.Sprintf("contract extraction failure: %s (status %d)", e.Category, e.HTTPStatus)
	}
	return "contract extraction failure: " + e.Category
}

func (e *ExtractionFailure) Unwrap() error { return e.cause }

func (e *ExtractionFailure) Is(target error) bool {
	if target == ErrExtractionPermanent {
		return e.Retry == RetryNever
	}
	if target == ErrExtractionTransient {
		return e.Retry == RetryStandard || e.Retry == RetryOnce
	}
	return false
}

func extractionFailure(category string, retry ExtractionRetryPolicy, cause error, httpStatus int) *ExtractionFailure {
	failure := &ExtractionFailure{Category: category, Retry: retry, HTTPStatus: httpStatus, cause: cause}
	var validation *ProposalValidationError
	if errors.As(cause, &validation) {
		failure.ValidationCode = validation.Code
		failure.ValidationPath = validation.Path
	}
	return failure
}

func ExtractionFailureDetails(err error) (*ExtractionFailure, bool) {
	var failure *ExtractionFailure
	ok := errors.As(err, &failure)
	return failure, ok
}

func annotateExtractionFailure(err error, provider, model string) error {
	failure, ok := ExtractionFailureDetails(err)
	if ok {
		failure.Provider = provider
		failure.Model = model
	}
	return err
}

// ProposalValidationError identifies a semantic rule without retaining the
// rejected value, provider output, evidence, or document content.
type ProposalValidationError struct {
	Code string
	Path string
}

func (e *ProposalValidationError) Error() string {
	if e.Path == "" {
		return "proposal validation failed: " + e.Code
	}
	return "proposal validation failed: " + e.Code + " at " + e.Path
}

func proposalValidationError(code, path string) error {
	return &ProposalValidationError{Code: code, Path: path}
}
