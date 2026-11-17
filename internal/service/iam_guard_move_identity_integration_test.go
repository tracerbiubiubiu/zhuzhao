//go:build integration

package service_test

import (
	"context"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tracerbiubiubiu/zhuzhao/internal/repository"
)

func TestIAMGuard_MoveCascadesActualIdentitySubtreeOnly(t *testing.T) {
	resetRBACTables(t)
	env := setupDelegation(t)
	ctx := context.Background()
	_, err := testPool.Exec(ctx, `UPDATE organizations SET deleted_at=NOW() WHERE id=ANY($1)`, []int64{env.pID, env.vgID})
	require.NoError(t, err)
	current := identityRecreate(t, env.pID, false)
	var child, historicalChild int64
	require.NoError(t, testPool.QueryRow(ctx, `INSERT INTO organizations(code,name,parent_id,path,is_virtual,status) SELECT code,'Synthetic current child',$2,path,is_virtual,1 FROM organizations WHERE id=$1 RETURNING id`, env.vgID, current).Scan(&child))
	require.NoError(t, testPool.QueryRow(ctx, `INSERT INTO organizations(code,name,parent_id,path,status,deleted_at) SELECT $1::text,'Synthetic actual historical child',id,(path::text||'.archived')::ltree,1,NOW() FROM organizations WHERE id=$2 RETURNING id`, "move_archived_"+uniqueSuffix(), current).Scan(&historicalChild))
	type paths struct {
		org, ticket, template int64
		before                string
		moving                bool
	}
	rows := []paths{{org: env.vgID}, {org: child, moving: true}, {org: historicalChild, moving: true}}
	for i := range rows {
		row := &rows[i]
		row.ticket = identityTicket(t, row.org, env.mem1)
		require.NoError(t, testPool.QueryRow(ctx, `SELECT org_path::text FROM tickets WHERE id=$1`, row.ticket).Scan(&row.before))
		require.NoError(t, testPool.QueryRow(ctx, `INSERT INTO ticket_templates(code,name,type_code,org_id,org_path,created_by,deleted_at) SELECT $1,'Synthetic template','iam_identity',id,path,$3,CASE WHEN deleted_at IS NOT NULL THEN NOW() ELSE NULL END FROM organizations WHERE id=$2 RETURNING id`, "move_template_"+uniqueSuffix(), row.org, env.mem1).Scan(&row.template))
	}
	repo := repository.NewOrgRepo(testPool)
	before, err := repo.FindByID(ctx, current)
	require.NoError(t, err)
	destination := iambOrg(t, "move_destination_"+uniqueSuffix())
	require.NoError(t, repo.Move(ctx, current, &destination))
	after, err := repo.FindByID(ctx, current)
	require.NoError(t, err)
	for _, row := range rows {
		expected := row.before
		if row.moving {
			expected = strings.Replace(row.before, before.Path, after.Path, 1)
		}
		var ticketPath, templatePath string
		require.NoError(t, testPool.QueryRow(ctx, `SELECT org_path::text FROM tickets WHERE id=$1`, row.ticket).Scan(&ticketPath))
		require.NoError(t, testPool.QueryRow(ctx, `SELECT org_path::text FROM ticket_templates WHERE id=$1`, row.template).Scan(&templatePath))
		assert.Equal(t, expected, ticketPath, "ticket actual org=%d moving=%t", row.org, row.moving)
		assert.Equal(t, expected, templatePath, "template actual org=%d moving=%t", row.org, row.moving)
	}
}
