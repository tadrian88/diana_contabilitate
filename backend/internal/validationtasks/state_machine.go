package validationtasks

type Trigger string

const (
	TriggerContractRequested           Trigger = "CONTRACT_REQUESTED"
	TriggerContractConfirmed           Trigger = "CONTRACT_CONFIRMED"
	TriggerContractBecameAvailable     Trigger = "CONTRACT_BECAME_AVAILABLE"
	TriggerCommercialCorrectionAwaited Trigger = "COMMERCIAL_CORRECTION_AWAITED"
	TriggerCommercialReviewResolved    Trigger = "COMMERCIAL_REVIEW_RESOLVED"
	TriggerClassificationResolved      Trigger = "CLASSIFICATION_RESOLVED"
)

type Transition struct {
	TaskType          Type
	From              Status
	To                Status
	Trigger           Trigger
	Module3Executable bool
}

var transitions = []Transition{
	{TypeMissingContract, StatusOpen, StatusWaiting, TriggerContractRequested, true},
	{TypeMissingContract, StatusOpen, StatusResolved, TriggerContractBecameAvailable, false},
	{TypeMissingContract, StatusWaiting, StatusResolved, TriggerContractBecameAvailable, false},
	{TypeContractMatch, StatusOpen, StatusResolved, TriggerContractConfirmed, false},
	{TypeCommercialReview, StatusOpen, StatusWaiting, TriggerCommercialCorrectionAwaited, false},
	{TypeCommercialReview, StatusOpen, StatusResolved, TriggerCommercialReviewResolved, false},
	{TypeCommercialReview, StatusWaiting, StatusResolved, TriggerCommercialReviewResolved, false},
	{TypeClassification, StatusOpen, StatusResolved, TriggerClassificationResolved, false},
}

func Transitions() []Transition { return append([]Transition(nil), transitions...) }

func FindTransition(taskType Type, from, to Status) (Transition, bool) {
	for _, transition := range transitions {
		if transition.TaskType == taskType && transition.From == from && transition.To == to {
			return transition, true
		}
	}
	return Transition{}, false
}

func (s Status) Active() bool   { return s != StatusResolved }
func (s Status) Terminal() bool { return s == StatusResolved }
