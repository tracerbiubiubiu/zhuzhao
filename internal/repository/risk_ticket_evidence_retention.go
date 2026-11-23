package repository

import (
	"context"

	"github.com/jackc/pgx/v5"
	commentref "github.com/tracerbiubiubiu/zhuzhao/internal/model/comment"
)

// hasRiskTicketEvidence reads all persisted proposals and reviews, including
// ended groups and links. A nil key checks whole-ticket deletion, including
// public comment evidence. Writers and both deletion paths hold the ticket
// row lock before accessing this history.
func hasRiskTicketEvidence(ctx context.Context, tx pgx.Tx, ticketID int64, key *string) (bool, error) {
	const q = `
		SELECT EXISTS (
			SELECT 1
			FROM risk_dispatch_groups g
			JOIN risk_ticket_links l ON l.group_id=g.id
			JOIN risk_remediation_submissions s ON s.link_id=l.id
			LEFT JOIN risk_remediation_reviews r ON r.submission_id=s.id
			CROSS JOIN LATERAL jsonb_array_elements(s.evidence_refs || COALESCE(r.evidence_refs, '[]'::jsonb)) AS e(ref)
			WHERE g.ticket_id=$1 AND (
				(e.ref->>'kind'='file' AND ($2::text IS NULL OR e.ref->>'ref'=$2))
				OR ($2::text IS NULL AND e.ref->>'kind'='reference' AND EXISTS (
					SELECT 1 FROM ticket_comments c
					WHERE c.ticket_id=$1 AND NOT c.is_internal AND e.ref->>'ref'=$3::text || c.id::text
				))
			)
		)`
	var referenced bool
	err := tx.QueryRow(ctx, q, ticketID, key, commentref.EvidenceRefPrefix).Scan(&referenced)
	return referenced, err
}

// GC also sees objects orphaned before the retention guard existed. A
// historical group may have lost its ticket_id, and a persisted evidence
// reference has no bucket, so preserve exact file keys in the swept bucket.
// A deleted object cannot be added as evidence; the confirmed-orphan update
// rechecks after locking the file, consistent with evidence writers.
const noRiskRemediationFileReference = `AND NOT EXISTS (
	SELECT 1
	FROM (
		SELECT evidence_refs FROM risk_remediation_submissions
		UNION ALL
		SELECT evidence_refs FROM risk_remediation_reviews
	) history
	CROSS JOIN LATERAL jsonb_array_elements(history.evidence_refs) AS e(ref)
	WHERE f.purpose IN ('ticket_attachment','ticket_richtext')
	  AND e.ref->>'kind'='file' AND e.ref->>'ref'=f.object_key
)`
