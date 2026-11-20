package order

import "fmt"

// Verdict is the gate's decision on a whole order.
type Verdict string

const (
	Execute     Verdict = "execute"
	NeedConfirm Verdict = "confirm"
	Refuse      Verdict = "refuse"
)

// Confidence thresholds. A written order in the exact grammar carries 1.0;
// a spoken one carries whatever the model reported.
const (
	ExecuteAt   = 0.85
	RefuseBelow = 0.50
)

// Gate decides what happens to an order. modeOf is normally Validate bound
// to the site policy; the chief wraps it to honour approved runbooks.
//
// The rule is deliberately small: anything invalid or mumbled is refused,
// anything that changes a target or was heard with less than high confidence
// waits for the principal, and only read-only or pre-approved calls heard
// clearly run on their own.
func Gate(calls []Call, conf float64, modeOf func(Call) (Mode, error)) (Verdict, string) {
	if len(calls) == 0 {
		return Refuse, "no recognised order"
	}
	needs := ""
	for _, c := range calls {
		m, err := modeOf(c)
		if err != nil {
			return Refuse, err.Error()
		}
		if m == Confirm && needs == "" {
			needs = c.Name + " changes the target"
		}
	}
	if conf < RefuseBelow {
		return Refuse, fmt.Sprintf("confidence %.2f is below %.2f", conf, RefuseBelow)
	}
	if needs != "" {
		return NeedConfirm, needs
	}
	if conf < ExecuteAt {
		return NeedConfirm, fmt.Sprintf("confidence %.2f is below %.2f", conf, ExecuteAt)
	}
	return Execute, ""
}
