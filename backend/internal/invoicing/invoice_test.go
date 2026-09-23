package invoicing

import "testing"

func TestPipelineVocabularyMatchesApprovedFrontend(t *testing.T) {
	expected := []PipelineStatus{
		"DOWNLOADED", "ARCHIVED", "MATCHING", "AWAITING_CONTRACT",
		"AWAITING_MATCH_CONFIRM", "DEDUPE_CHECKED", "HEADER_READ", "LINES_READ",
		"COMMERCIAL_VALIDATING", "AWAITING_COMMERCIAL_REVIEW", "COMMERCIALLY_VALIDATED",
		"CLASSIFIED", "AWAITING_REVIEW", "READY_FOR_SAGA", "EXPORTING", "EXPORTED", "DUPLICATE",
	}
	if len(PipelineStatuses) != len(expected) {
		t.Fatalf("got %d statuses, want %d", len(PipelineStatuses), len(expected))
	}
	for i := range expected {
		if PipelineStatuses[i] != expected[i] {
			t.Fatalf("status %d is %q, want %q", i, PipelineStatuses[i], expected[i])
		}
	}
}

func TestStateMachineDefinesEveryApprovedLegalEdgeAndRejectsAllOtherPairs(t *testing.T) {
	definitions := TransitionDefinitions()
	legal := make(map[[2]PipelineStatus]TransitionDefinition, len(definitions))
	for _, definition := range definitions {
		key := [2]PipelineStatus{definition.From, definition.To}
		if _, duplicate := legal[key]; duplicate {
			t.Fatalf("duplicate transition definition %s -> %s", definition.From, definition.To)
		}
		legal[key] = definition
	}

	for _, from := range PipelineStatuses {
		for _, to := range PipelineStatuses {
			definition, found := FindTransition(from, to)
			want, expected := legal[[2]PipelineStatus{from, to}]
			if found != expected {
				t.Fatalf("transition %s -> %s found=%v, want %v", from, to, found, expected)
			}
			if found && definition != want {
				t.Fatalf("transition %s -> %s differs: %+v != %+v", from, to, definition, want)
			}
		}
	}
}

func TestModule2ExecutableAndDeferredTransitionsAreExplicit(t *testing.T) {
	for _, definition := range TransitionDefinitions() {
		_, err := ValidateModule2Transition(definition.From, definition.To, definition.Trigger)
		if definition.Module2Executable && err != nil {
			t.Errorf("executable transition %s -> %s rejected: %v", definition.From, definition.To, err)
		}
		if !definition.Module2Executable && err == nil {
			t.Errorf("deferred transition %s -> %s was activated", definition.From, definition.To)
		}
	}
}

func TestTerminalStatesHaveNoOutgoingTransition(t *testing.T) {
	for _, terminal := range []PipelineStatus{StatusDuplicate, StatusExported} {
		if !terminal.Terminal() {
			t.Fatalf("%s should be terminal", terminal)
		}
		for _, target := range PipelineStatuses {
			if _, found := FindTransition(terminal, target); found {
				t.Fatalf("terminal state %s transitions to %s", terminal, target)
			}
		}
	}
}
