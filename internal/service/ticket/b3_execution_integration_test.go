//go:build integration

package ticket

import (
	"context"
	"encoding/json"
	"strconv"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/tracerbiubiubiu/zhuzhao/internal/middleware"
	"github.com/tracerbiubiubiu/zhuzhao/internal/model"
	"github.com/tracerbiubiubiu/zhuzhao/internal/pkg/b3protocol"
	"github.com/tracerbiubiubiu/zhuzhao/internal/pkg/errcode"
	"github.com/tracerbiubiubiu/zhuzhao/internal/repository"
	"github.com/tracerbiubiubiu/zhuzhao/internal/service"
)

func TestB3ReportRechecksOriginalEdgeAfterNodeAdvance(t *testing.T) {
	for _, revoke := range []bool{false, true} {
		t.Run("assignee_revoked_"+strconv.FormatBool(revoke), func(t *testing.T) {
			ctx := context.Background()
			svc, actor, org := setupTicketAdmin(t)
			store := repository.NewB3Store(testPool)
			svc.SetB3Store(store, true)
			actions := NewActionRegistry()
			require.NoError(t, actions.Register(NewSpawnTaskAction(nil)))
			svc.SetActionRegistry(actions)
			code := "b3_edge_" + uniqueSuffix()
			_, err := svc.CreateTicketType(ctx, &model.CreateTicketTypeRequest{Code: code, Name: code})
			require.NoError(t, err)
			_, err = svc.UpdateTicketType(ctx, code, &model.UpdateTicketTypeRequest{TransitionMeta: rawJSON(`{
                "open>assigned": {
                    "roles": ["assignee"],
                    "post_actions": [{"name":"spawn_task","params":{
                        "action":"ticket.evidence.report.generate","params":{"report_profile":"evidence_summary_v1"}
                    }}]
                }
            }`)})
			require.NoError(t, err)
			def := publishD01(t, svc, code, actor)
			tk, err := svc.Create(ctx, &model.CreateTicketRequest{TypeCode: code, Title: code, OrgID: org()}, actor)
			require.NoError(t, err)
			var revision int64
			require.NoError(t, testPool.QueryRow(ctx, "UPDATE tickets SET assigned_to=$2 WHERE id=$1 RETURNING revision", tk.ID, actor).Scan(&revision))
			httpCtx := d02ReplayHTTPContext(t, ctx, actor)
			original, err := svc.Transition(httpCtx, &model.TransitionTicketRequest{
				ID: tk.ID, To: StatusAssigned, IdempotencyKey: code,
				ExpectedRevision:  rawJSON(strconv.Quote(fmtInt(revision))),
				DefinitionVersion: rawJSON(strconv.Quote(fmtInt(def.DefinitionVersion))),
				CustomDataPatch:   rawJSON("{}"),
			}, actor)
			require.NoError(t, err)
			_, err = testPool.Exec(ctx, "UPDATE tickets SET status='in_progress',assigned_to=CASE WHEN $2 THEN NULL ELSE assigned_to END WHERE id=$1", tk.ID, revoke)
			require.NoError(t, err)
			runs, err := store.TicketRuns(ctx, tk.ID)
			require.NoError(t, err)
			require.Len(t, runs, 1)
			run := runs[0]
			purpose, admitted := middleware.IAMRequestFromContext(httpCtx)
			require.True(t, admitted)
			flow := service.NewB3Flow(testPool, store, nil, nil, svc.RunB3Authorized, purpose.Model, service.B3Subscription{})
			params, err := json.Marshal(b3protocol.TaskParams{OperationID: run.OperationID})
			require.NoError(t, err)
			_, err = flow.Callback(ctx, service.B3Callback{
				ProtocolVersion: 2, ActionVersion: 1, TaskID: run.TaskID, Action: run.Action,
				OperationID: run.OperationID, DeliveryID: "tr-edge-123456789", Params: params,
			})
			if revoke {
				require.ErrorIs(t, err, errcode.ErrForbidden)
			} else {
				require.NoError(t, err)
			}
			latest, err := svc.Get(ctx, tk.ID, actor)
			require.NoError(t, err)
			require.Equal(t, StatusInProgress, latest.Status)
			require.Len(t, latest.TaskRuns, 1)
			got := latest.TaskRuns[0]
			require.Equal(t, original.Revision, got.TicketRevision)
			require.Equal(t, original.DefinitionRef.DefinitionHash, got.DefinitionHash)
			require.Equal(t, StatusAssigned, got.Node)
			if revoke {
				require.Equal(t, "held", got.Status)
				require.Empty(t, got.Result)
			} else {
				require.Equal(t, "succeeded", got.Status)
				require.NotEmpty(t, got.Result)
			}
		})
	}
}
