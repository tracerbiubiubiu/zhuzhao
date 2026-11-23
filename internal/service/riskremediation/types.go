// Package riskremediation owns per-asset remediation and acceptance facts.
// All entry points join a caller-owned C04 transaction; they never commit it.
package riskremediation

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type EvidenceRef struct {
	Kind    string `json:"kind"`
	Ref     string `json:"ref"`
	Version string `json:"version"`
}
type Command struct {
	ActorID                int64
	OperationID            string
	TicketID               int64
	ExpectedTicketRevision int64
	// TicketBinding is the owning adapter's lowercase SHA-256 for the pinned
	// definition version and normalized common transition inputs.
	TicketBinding string `json:"ticket_binding,omitempty"`
}
type Item struct {
	LinkID               int64
	ExpectedLinkRevision int64
	ExpectedRiskRevision int64
	Resolution           string
	Measures             string
	Evidence             []EvidenceRef
}
type FinishCommand struct {
	Command
	Items []Item
}
type ReviewItem struct {
	LinkID                  int64
	ExpectedLinkRevision    int64
	ExpectedRiskRevision    int64
	ExpectedProposalVersion int64
	Decision                string
	Note                    string
	Evidence                []EvidenceRef
}
type ReviewCommand struct {
	Command
	Items []ReviewItem
}
type AcceptanceRequest struct {
	Command
	RiskID               int64
	ExpectedRiskRevision int64
	LinkID               int64
	ExpectedLinkRevision int64
	Reason               string
	ValidUntil           time.Time
}
type AcceptanceDecision struct {
	Command
	AcceptanceID               int64
	ExpectedAcceptanceRevision int64
	ExpectedRiskRevision       int64
	Decision                   string
	Reason                     string
}
type ItemResult struct {
	LinkID            int64
	RiskID            int64
	LinkRevision      int64
	RiskRevision      int64
	ProposalVersion   int64
	ResolutionVersion int64
	Result            string
}
type Result struct {
	TicketID           int64
	TicketRevision     int64
	DefinitionVersion  int64  `json:",omitempty"`
	AssignedToUserID   int64  `json:",omitempty"`
	SuccessorUserID    int64  `json:",omitempty"`
	Generation         int64  `json:",omitempty"`
	Status             string `json:",omitempty"`
	Items              []ItemResult
	AcceptanceID       int64
	AcceptanceRevision int64
	AcceptanceStatus   string
	TerminalAllowed    bool
}

// Access is a server-created authorization query, never a grant from the body.
// Prelock must check current credentials/L1 and lock organization/pinned definition
// before the kernel locks group/risk/ticket/link. Authorize rechecks object and
// action qualification with the locked current owner; receipt reads use receipt.
type Access struct {
	// RiskIDs gives Prelock the complete original receipt set, including detached risks.
	RiskIDs  []int64
	Action   string
	ActorID  int64
	TicketID int64
	RiskID   int64
	OrgID    int64
	OwnerID  int64
	LinkID   int64
}
type TicketCheck struct {
	Action           string
	ActorID          int64
	TicketID         int64
	ExpectedRevision int64
}
type Policy struct{ ReviewGate bool }

// EvidenceMode separates eligibility checks from transaction-local attachment
// writes. Its zero value is read-only so omitted modes cannot acquire bindings.
type EvidenceMode string

const (
	EvidenceValidate EvidenceMode = ""
	EvidenceBind     EvidenceMode = "bind"
)

type EvidenceCheck struct {
	Access
	Mode EvidenceMode
	// Historical is set only for read-only revalidation of the original
	// submission. Check its stable version, current read access, and original
	// target binding; do not require the reviewer to own a new upload or acquire
	// another binding. Historical must never be accepted with EvidenceBind.
	Historical bool
	Evidence   []EvidenceRef
}
type TicketEffect struct {
	TicketCheck
	TerminalAllowed bool
	Items           []ItemResult
}

// Callbacks are required adapters, not authorization defaults. All run on the
// supplied transaction. EvidenceValidate must only check stable references,
// readability, and (for new evidence) file ownership/eligibility. Historical
// evidence uses the original target binding instead of new-upload eligibility.
// Validation must never write bindings or other business effects.
// EvidenceBind must recheck that eligibility and atomically acquire only the
// supplied new evidence bindings on the supplied transaction. An adapter must
// reject unknown modes and must not write through a pool or outer transaction.
// ApplyTicket must execute only legal
// pinned-definition edges and persist ticket history/intents atomically. It may
// keep a partially reviewed ticket nonterminal. No network calls under locks.
type Callbacks struct {
	// Admit checks current credentials and L1 only; it must take no domain locks.
	Admit        func(context.Context, pgx.Tx, Access) error
	Prelock      func(context.Context, pgx.Tx, Access) error
	Authorize    func(context.Context, pgx.Tx, Access) error
	TicketPolicy func(context.Context, pgx.Tx, TicketCheck) (Policy, error)
	Evidence     func(context.Context, pgx.Tx, EvidenceCheck) error
	ApplyTicket  func(context.Context, pgx.Tx, TicketEffect) (int64, error)
}
type Service struct {
	pool      *pgxpool.Pool
	callbacks Callbacks
}

func New(pool *pgxpool.Pool, callbacks Callbacks) *Service {
	return &Service{pool: pool, callbacks: callbacks}
}
