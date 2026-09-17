package invoicing

import (
	"fmt"
)

type TransitionTrigger string

const (
	TriggerArchiveCompleted       TransitionTrigger = "ARCHIVE_COMPLETED"
	TriggerStartMatching          TransitionTrigger = "START_MATCHING"
	TriggerMatchingDecision       TransitionTrigger = "MATCHING_DECISION"
	TriggerContractConfirmed      TransitionTrigger = "CONTRACT_CONFIRMED"
	TriggerDuplicateDetected      TransitionTrigger = "DUPLICATE_DETECTED"
	TriggerDuplicateCleared       TransitionTrigger = "DUPLICATE_CLEARED"
	TriggerHeaderParsed           TransitionTrigger = "HEADER_PARSED"
	TriggerLinesParsed            TransitionTrigger = "LINES_PARSED"
	TriggerClassificationDecision TransitionTrigger = "CLASSIFICATION_DECISION"
	TriggerReviewCompleted        TransitionTrigger = "REVIEW_COMPLETED"
	TriggerSagaHandoff            TransitionTrigger = "SAGA_HANDOFF"
	TriggerSagaExported           TransitionTrigger = "SAGA_EXPORTED"
)

type TransitionDefinition struct {
	From                   PipelineStatus
	To                     PipelineStatus
	Trigger                TransitionTrigger
	Automatic              bool
	Module2Executable      bool
	ContinueAsynchronously bool
}

var transitionDefinitions = []TransitionDefinition{
	{StatusDownloaded, StatusArchived, TriggerArchiveCompleted, true, true, true},
	{StatusArchived, StatusMatching, TriggerStartMatching, true, true, true},
	{StatusMatching, StatusAwaitingContract, TriggerMatchingDecision, false, false, false},
	{StatusMatching, StatusAwaitingMatchConfirm, TriggerMatchingDecision, false, false, false},
	{StatusMatching, StatusDedupeChecked, TriggerMatchingDecision, true, false, true},
	{StatusAwaitingMatchConfirm, StatusDedupeChecked, TriggerContractConfirmed, false, false, true},
	{StatusDedupeChecked, StatusDuplicate, TriggerDuplicateDetected, true, false, false},
	{StatusDedupeChecked, StatusHeaderRead, TriggerDuplicateCleared, true, true, true},
	{StatusHeaderRead, StatusLinesRead, TriggerLinesParsed, true, true, true},
	{StatusLinesRead, StatusClassified, TriggerClassificationDecision, true, false, false},
	{StatusClassified, StatusAwaitingReview, TriggerClassificationDecision, false, false, false},
	{StatusClassified, StatusReadyForSAGA, TriggerClassificationDecision, true, false, true},
	{StatusAwaitingReview, StatusReadyForSAGA, TriggerReviewCompleted, false, false, true},
	{StatusReadyForSAGA, StatusExporting, TriggerSagaHandoff, true, true, true},
	{StatusExporting, StatusExported, TriggerSagaExported, true, true, false},
}

func TransitionDefinitions() []TransitionDefinition {
	return append([]TransitionDefinition(nil), transitionDefinitions...)
}

func FindTransition(from, to PipelineStatus) (TransitionDefinition, bool) {
	for _, definition := range transitionDefinitions {
		if definition.From == from && definition.To == to {
			return definition, true
		}
	}
	return TransitionDefinition{}, false
}

func ValidateModule2Transition(from, to PipelineStatus, trigger TransitionTrigger) (TransitionDefinition, error) {
	definition, ok := FindTransition(from, to)
	if !ok || definition.Trigger != trigger {
		return TransitionDefinition{}, fmt.Errorf("illegal pipeline transition %s -> %s using %s", from, to, trigger)
	}
	if !definition.Module2Executable {
		return TransitionDefinition{}, fmt.Errorf("pipeline transition %s -> %s is deferred", from, to)
	}
	return definition, nil
}

func (s PipelineStatus) Terminal() bool { return s == StatusDuplicate || s == StatusExported }
