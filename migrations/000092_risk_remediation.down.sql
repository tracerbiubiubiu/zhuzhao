-- Empty isolated fixtures may roll back. Durable submissions, decisions,
-- receipts, intents and noninitial link state must never be silently discarded.
LOCK TABLE risk_ticket_links, risk_remediation_submissions,
    risk_remediation_reviews, risk_acceptances, risk_remediation_receipts,
    risk_remediation_intents IN ACCESS EXCLUSIVE MODE;

DO $$
BEGIN
    IF EXISTS (SELECT 1 FROM risk_remediation_submissions)
        OR EXISTS (SELECT 1 FROM risk_remediation_reviews)
        OR EXISTS (SELECT 1 FROM risk_acceptances)
        OR EXISTS (SELECT 1 FROM risk_remediation_receipts)
        OR EXISTS (SELECT 1 FROM risk_remediation_intents)
        OR EXISTS (
            SELECT 1 FROM risk_ticket_links
            WHERE remediation_state <> 'open'
                OR proposal_version <> 0 OR resolution_version <> 0
                OR resolution IS NOT NULL OR resolved_risk_revision IS NOT NULL
                OR effective_owner_id IS NOT NULL OR effective_at IS NOT NULL
        ) THEN
        RAISE EXCEPTION 'risk remediation facts exist; retain history and use a compatible forward migration';
    END IF;
END;
$$;

DELETE FROM casbin_rule WHERE ptype = 'p' AND (v1, v2) IN (
    ('/api/v1/risk-items/remediation-context/:id', 'GET'),
    ('/api/v1/risk-items/remediation-operations/:id', 'GET'),
    ('/api/v1/risk-items/remediation-finish', 'POST'),
    ('/api/v1/risk-items/remediation-review', 'POST'),
    ('/api/v1/risk-items/remediation-accept-request', 'POST'),
    ('/api/v1/risk-items/remediation-accept-decision', 'POST'),
    ('/api/v1/risk-items/remediation-assign-handler', 'POST'),
    ('/api/v1/risk-items/remediation-accept-assignment', 'POST')
);
DELETE FROM role_menus WHERE menu_id IN (
    SELECT id FROM menus WHERE code IN ('risk_remediation_finish_btn', 'risk_remediation_review_btn',
        'risk_remediation_accept_request_btn', 'risk_remediation_accept_decision_btn',
        'risk_remediation_assign_handler_btn', 'risk_remediation_accept_assignment_btn')
);
DELETE FROM menu_apis WHERE api_path IN (
    '/api/v1/risk-items/remediation-context/:id', '/api/v1/risk-items/remediation-operations/:id',
    '/api/v1/risk-items/remediation-finish', '/api/v1/risk-items/remediation-review',
    '/api/v1/risk-items/remediation-accept-request', '/api/v1/risk-items/remediation-accept-decision',
    '/api/v1/risk-items/remediation-assign-handler', '/api/v1/risk-items/remediation-accept-assignment'
);
DELETE FROM menus WHERE code IN ('risk_remediation_finish_btn', 'risk_remediation_review_btn',
    'risk_remediation_accept_request_btn', 'risk_remediation_accept_decision_btn',
    'risk_remediation_assign_handler_btn', 'risk_remediation_accept_assignment_btn');

DROP TABLE risk_remediation_intents;
DROP TABLE risk_remediation_receipts;
DROP TABLE risk_acceptances;
DROP TABLE risk_remediation_reviews;
DROP TABLE risk_remediation_submissions;

ALTER TABLE risk_ticket_links
    DROP CONSTRAINT risk_ticket_links_remediation_resolution,
    DROP COLUMN remediation_state,
    DROP COLUMN proposal_version,
    DROP COLUMN resolution_version,
    DROP COLUMN resolution,
    DROP COLUMN resolved_risk_revision,
    DROP COLUMN effective_owner_id,
    DROP COLUMN effective_at;
