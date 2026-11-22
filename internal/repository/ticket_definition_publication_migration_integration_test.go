//go:build integration

package repository_test

import (
	"context"
	"os"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestD01PublicationMigrationRoundTrip(t *testing.T) {
	ctx := context.Background()
	up, err := os.ReadFile("../../migrations/000085_ticket_definition_publication.up.sql")
	require.NoError(t, err, "the publication migration must be registered")
	down, err := os.ReadFile("../../migrations/000085_ticket_definition_publication.down.sql")
	require.NoError(t, err)
	tx, err := testPool.Begin(ctx)
	require.NoError(t, err)
	defer tx.Rollback(ctx)
	var installed bool
	require.NoError(t, tx.QueryRow(ctx, `SELECT to_regclass('ticket_definition_publication_plans') IS NOT NULL`).Scan(&installed))
	if installed {
		_, err = tx.Exec(ctx, string(down))
		require.NoError(t, err)
	}
	_, err = tx.Exec(ctx, `INSERT INTO roles(code,name,priority) VALUES
		('admin','Synthetic admin',10),('superadmin','Synthetic superadmin',1),
		('pubmigration_creator','Synthetic creator',20),('pubmigration_manager','Synthetic manager',20),
		('pubmigration_reader','Synthetic reader',20),('pubmigration_writer','Synthetic writer',20),
		('pubmigration_unassigned','Synthetic unassigned',20),('pubmigration_recreated','Synthetic recreated role',20),
		('pubmigration_no_grant','Synthetic no grant',20),('pubmigration_write_only','Synthetic write only',20)
		ON CONFLICT DO NOTHING;
		INSERT INTO role_menus(role_id,menu_id)
		SELECT r.id,m.id FROM (VALUES
			('pubmigration_creator','ticket_list'),('pubmigration_creator','ticket_create_btn'),
			('pubmigration_manager','ticket_list'),('pubmigration_manager','ticket_type_manage'),
			('pubmigration_reader','ticket_list'),('pubmigration_writer','ticket_list'),('pubmigration_writer','ticket_type_write_btn'),
			('pubmigration_unassigned','ticket_list'),('pubmigration_recreated','ticket_list'),('pubmigration_no_grant','ticket_list'),
			('pubmigration_write_only','ticket_type_write_btn')
		) AS v(role_code,menu_code) JOIN roles r ON r.code=v.role_code JOIN menus m ON m.code=v.menu_code;
		INSERT INTO casbin_rule(ptype,v0,v1,v2) VALUES
			('p','role::pubmigration_creator','/api/v1/ticket-types/:code/fields','GET'),
			('p','role::pubmigration_manager','/api/v1/ticket-types/:code/fields','GET'),
			('p','role::pubmigration_reader','/api/v1/ticket-types/:code/fields','GET'),
			('p','role::pubmigration_writer','/api/v1/ticket-types/:code/fields','GET'),
			('p','role::pubmigration_unassigned','/api/v1/ticket-types/:code/fields','GET'),
			('p','role::pubmigration_recreated','/api/v1/ticket-types/:code/fields','GET');`)
	require.NoError(t, err)
	var originalPolicyID int64
	require.NoError(t, tx.QueryRow(ctx, `SELECT id FROM casbin_rule WHERE ptype='p' AND v0='role::pubmigration_creator'
		AND v1='/api/v1/ticket-types/:code/fields' AND v2='GET'`).Scan(&originalPolicyID))
	_, err = tx.Exec(ctx, string(up))
	require.NoError(t, err)

	var names []string
	selectPostOwners := `SELECT ARRAY(SELECT m.code || ':' || m.menu_type FROM menu_apis a
		JOIN menus m ON m.id=a.menu_id WHERE a.api_method='POST' AND a.api_path IN
		('/api/v1/ticket-types/definitions/plan','/api/v1/ticket-types/definitions/publish') ORDER BY a.api_path,m.code)`
	require.NoError(t, tx.QueryRow(ctx, selectPostOwners).Scan(&names))
	require.Equal(t, []string{"ticket_definition_publish_btn:3", "ticket_definition_publish_btn:3"}, names)
	require.NoError(t, tx.QueryRow(ctx, `SELECT ARRAY(SELECT r.code FROM role_menus rm
		JOIN roles r ON rm.role_id=r.id JOIN menus m ON rm.menu_id=m.id
		WHERE m.code='ticket_definition_publish_btn' ORDER BY r.code)`).Scan(&names))
	require.Equal(t, []string{"admin", "superadmin"}, names)

	for _, tc := range []struct {
		path string
		want []string
	}{
		{"/api/v1/ticket-types/:code/fields", []string{"role::pubmigration_manager", "role::pubmigration_writer"}},
		{"/api/v1/ticket-types/:code/published", []string{"role::pubmigration_creator", "role::pubmigration_manager"}},
	} {
		require.NoError(t, tx.QueryRow(ctx, `SELECT ARRAY(SELECT v0 FROM casbin_rule WHERE ptype='p'
			AND v1=$1 AND v2='GET' AND starts_with(v0,'role::pubmigration_')
			ORDER BY v0)`, tc.path).Scan(&names))
		require.Equal(t, tc.want, names, tc.path)
	}

	_, err = tx.Exec(ctx, `DELETE FROM role_menus WHERE role_id IN (SELECT id FROM roles WHERE code IN ('pubmigration_unassigned','pubmigration_recreated'));
		DELETE FROM roles WHERE code='pubmigration_recreated';
		INSERT INTO roles(code,name,priority) VALUES ('pubmigration_recreated','New role with old code',20);
		INSERT INTO role_menus(role_id,menu_id) SELECT r.id,m.id FROM roles r CROSS JOIN menus m
		WHERE r.code='pubmigration_recreated' AND m.code='ticket_list';
		INSERT INTO casbin_rule(ptype,v0,v1,v2) VALUES('p','role::pubmigration_write_only','/api/v1/ticket-types/:code/fields','GET')`)
	require.NoError(t, err)
	_, err = tx.Exec(ctx, string(down))
	require.NoError(t, err)
	require.NoError(t, tx.QueryRow(ctx, selectPostOwners).Scan(&names))
	require.Empty(t, names)
	require.NoError(t, tx.QueryRow(ctx, `SELECT ARRAY(SELECT m.code FROM menus m JOIN menu_apis a ON a.menu_id=m.id
		WHERE a.api_path='/api/v1/ticket-types/:code/fields' AND a.api_method='GET' ORDER BY m.code)`).Scan(&names))
	require.Equal(t, []string{"ticket_list", "ticket_type_manage"}, names)
	require.NoError(t, tx.QueryRow(ctx, `SELECT count(*) = 0 FROM casbin_rule WHERE ptype='p'
		AND (v1,v2) IN (('/api/v1/ticket-types/definitions/plan','POST'),('/api/v1/ticket-types/definitions/publish','POST'),
		('/api/v1/ticket-types/:code/published','GET'))`).Scan(&installed))
	require.True(t, installed)
	require.NoError(t, tx.QueryRow(ctx, `SELECT ARRAY(SELECT v0 FROM casbin_rule WHERE ptype='p'
		AND v1='/api/v1/ticket-types/:code/fields' AND v2='GET' AND starts_with(v0,'role::pubmigration_') ORDER BY v0)`).Scan(&names))
	require.Equal(t, []string{"role::pubmigration_creator", "role::pubmigration_manager", "role::pubmigration_reader", "role::pubmigration_writer"}, names,
		"down preserves original grants bound to the same role and does not synthesize grants")
	var restoredPolicyID int64
	require.NoError(t, tx.QueryRow(ctx, `SELECT id FROM casbin_rule WHERE ptype='p' AND v0='role::pubmigration_creator'
		AND v1='/api/v1/ticket-types/:code/fields' AND v2='GET'`).Scan(&restoredPolicyID))
	require.Equal(t, originalPolicyID, restoredPolicyID)

	_, err = tx.Exec(ctx, string(up))
	require.NoError(t, err)
	_, err = tx.Exec(ctx, `INSERT INTO ticket_types(code,name) VALUES ('pubmigration_type','Synthetic draft');
		INSERT INTO ticket_definition_publication_plans(actor_id,type_id,type_code,draft_revision,canonical_hash,snapshot_hash,reference_revisions,warnings)
		SELECT 1,id,code,version,repeat('a',64),repeat('b',64),'{}','[]' FROM ticket_types WHERE code='pubmigration_type';
		DELETE FROM ticket_types WHERE code='pubmigration_type';
		INSERT INTO ticket_types(code,name) VALUES ('pubmigration_type','Synthetic recreated type');`)
	require.NoError(t, err, "an unused plan must not pin an unpublished draft")
	require.NoError(t, tx.QueryRow(ctx, `SELECT p.type_id <> t.id FROM ticket_definition_publication_plans p
		JOIN ticket_types t ON p.type_code=t.code WHERE t.code='pubmigration_type'`).Scan(&installed))
	require.True(t, installed, "the original plan keeps the prior type identity")
	_, err = tx.Exec(ctx, `INSERT INTO ticket_type_definitions(type_code,definition_version,definition_hash,schema_format_version,snapshot,source_draft_revision)
		VALUES('pubmigration_type',1,repeat('b',64),1,'{}',1);
		UPDATE ticket_types SET current_published_version=1 WHERE code='pubmigration_type';
		INSERT INTO ticket_definition_publication_plans(actor_id,type_id,type_code,draft_revision,expected_published_version,canonical_hash,snapshot_hash,reference_revisions,warnings)
		SELECT 2,id,code,version,1,repeat('a',64),repeat('b',64),'{}','[]' FROM ticket_types WHERE code='pubmigration_type';
		INSERT INTO ticket_definition_publication_receipts(actor_id,operation_id,request_hash,plan_id,type_code,definition_version,definition_hash,draft_revision,changed)
		SELECT 2,'pubmigration_op',repeat('c',64),id,type_code,1,repeat('b',64),draft_revision,false
		FROM ticket_definition_publication_plans WHERE actor_id=2`)
	require.NoError(t, err)
	beforeRollback, err := tx.Begin(ctx)
	require.NoError(t, err)
	_, err = beforeRollback.Exec(ctx, string(down))
	require.ErrorContains(t, err, "retain ticket definition publication receipts before rollback")
	require.NoError(t, beforeRollback.Rollback(ctx))
	require.NoError(t, tx.QueryRow(ctx, `SELECT NOT changed FROM ticket_definition_publication_receipts
		WHERE actor_id=2 AND operation_id='pubmigration_op'`).Scan(&installed))
	require.True(t, installed, "rollback preserves the original no-change success receipt")
}
