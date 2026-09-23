package validationtasks

import "testing"

func TestTaskStateMachineIsExhaustive(t *testing.T) {
	known := make(map[[3]string]Transition)
	for _, transition := range Transitions() {
		key := [3]string{string(transition.TaskType), string(transition.From), string(transition.To)}
		if _, exists := known[key]; exists {
			t.Fatalf("duplicate transition %+v", transition)
		}
		known[key] = transition
	}
	for _, taskType := range Types {
		for _, from := range Statuses {
			for _, to := range Statuses {
				got, found := FindTransition(taskType, from, to)
				want, expected := known[[3]string{string(taskType), string(from), string(to)}]
				if found != expected || found && got != want {
					t.Fatalf("%s %s -> %s found=%v got=%+v", taskType, from, to, found, got)
				}
			}
		}
	}
}

func TestOnlyMissingContractCanWaitInModule3(t *testing.T) {
	transition, found := FindTransition(TypeMissingContract, StatusOpen, StatusWaiting)
	if !found || !transition.Module3Executable || transition.Trigger != TriggerContractRequested {
		t.Fatalf("missing-contract request transition=%+v found=%v", transition, found)
	}
	for _, taskType := range []Type{TypeContractMatch, TypeClassification} {
		if _, found := FindTransition(taskType, StatusOpen, StatusWaiting); found {
			t.Fatalf("%s must not enter WAITING", taskType)
		}
	}
}

func TestResolutionEdgesAreExplicitApplicationTransitions(t *testing.T) {
	for _, taskType := range []Type{TypeContractMatch, TypeClassification} {
		transition, found := FindTransition(taskType, StatusOpen, StatusResolved)
		if !found || transition.Module3Executable {
			t.Fatalf("resolution edge for %s = %+v found=%v", taskType, transition, found)
		}
	}
	for _, from := range []Status{StatusOpen, StatusWaiting} {
		transition, found := FindTransition(TypeMissingContract, from, StatusResolved)
		if !found || transition.Module3Executable || transition.Trigger != TriggerContractBecameAvailable {
			t.Fatalf("missing-contract resume transition from %s = %+v found=%v", from, transition, found)
		}
	}
}

func TestResolvedHasNoOutgoingTransitionAndWaitingOnlyResolvesOnAvailability(t *testing.T) {
	for _, taskType := range Types {
		for _, to := range Statuses {
			if _, found := FindTransition(taskType, StatusResolved, to); found {
				t.Fatalf("unexpected %s %s -> %s", taskType, StatusResolved, to)
			}
		}
	}
	for _, taskType := range []Type{TypeContractMatch, TypeClassification} {
		for _, to := range Statuses {
			if _, found := FindTransition(taskType, StatusWaiting, to); found {
				t.Fatalf("unexpected %s %s -> %s", taskType, StatusWaiting, to)
			}
		}
	}
	if !StatusOpen.Active() || !StatusWaiting.Active() || StatusResolved.Active() || !StatusResolved.Terminal() {
		t.Fatal("active/terminal status semantics changed")
	}
}

func TestValidationTaskVocabularyIncludesCommercialReview(t *testing.T) {
	wantTypes := []Type{"CONTRACT_MATCH", "MISSING_CONTRACT", "COMMERCIAL_REVIEW", "CLASSIFICATION"}
	wantStatuses := []Status{"OPEN", "WAITING", "RESOLVED"}
	if len(Types) != len(wantTypes) || len(Statuses) != len(wantStatuses) {
		t.Fatalf("types=%v statuses=%v", Types, Statuses)
	}
	for index := range wantTypes {
		if Types[index] != wantTypes[index] {
			t.Fatalf("type %d=%s want=%s", index, Types[index], wantTypes[index])
		}
	}
	for index := range wantStatuses {
		if Statuses[index] != wantStatuses[index] {
			t.Fatalf("status %d=%s want=%s", index, Statuses[index], wantStatuses[index])
		}
	}
}

func TestCommercialReviewCanWaitOrResolveButNeverUseModule3Command(t *testing.T) {
	for _, edge := range [][2]Status{{StatusOpen, StatusWaiting}, {StatusOpen, StatusResolved}, {StatusWaiting, StatusResolved}} {
		transition, found := FindTransition(TypeCommercialReview, edge[0], edge[1])
		if !found || transition.Module3Executable {
			t.Fatalf("commercial transition %s -> %s: %+v, found=%v", edge[0], edge[1], transition, found)
		}
	}
}
