package model

import (
	"math"
	"strings"
	"time"
)

// RiskFirstAcceptRule is the optional policy in a published assigned state.
type RiskFirstAcceptRule struct {
	RuleID              string `json:"rule_id"`
	RuleVersion         int64  `json:"rule_version"`
	AcceptWithinMinutes int64  `json:"accept_within_minutes"`
}

func (r RiskFirstAcceptRule) Valid() bool {
	return r.RuleID != "" && strings.TrimSpace(r.RuleID) == r.RuleID && len(r.RuleID) <= 128 &&
		r.RuleVersion > 0 && r.AcceptWithinMinutes > 0 && r.AcceptWithinMinutes <= math.MaxInt64/int64(time.Minute)
}
