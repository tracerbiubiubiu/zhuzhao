package ticket

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/stretchr/testify/require"
	"github.com/tracerbiubiubiu/zhuzhao/internal/model"
	"github.com/tracerbiubiubiu/zhuzhao/internal/pkg/datafetcher"
	"github.com/tracerbiubiubiu/zhuzhao/internal/pkg/errcode"
	pkgstorage "github.com/tracerbiubiubiu/zhuzhao/internal/pkg/storage"
	"github.com/tracerbiubiubiu/zhuzhao/internal/repository"
)

func TestRiskDispatchCreateRequiresPreview(t *testing.T) {
	for _, tc := range []struct {
		name, version, hash string
		actor               int64
		valid               bool
	}{
		{"missing", "", "", 11, false},
		{"partial", "1", "", 11, false},
		{"zero", "0", strings.Repeat("a", 64), 11, false},
		{"noncanonical", "01", strings.Repeat("a", 64), 11, false},
		{"nonhex", "1", strings.Repeat("z", 64), 11, false},
		{"uppercase", "1", strings.Repeat("A", 64), 11, false},
		{"no actor", "1", strings.Repeat("a", 64), 0, false},
		{"valid", "1", strings.Repeat("a", 64), 11, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			err := validateRiskDispatchCreate(&model.CreateTicketRequest{TypeCode: "incident", Title: "synthetic", OrgID: 4, DefinitionVersion: tc.version, DefinitionHash: tc.hash}, tc.actor)
			if tc.valid {
				require.NoError(t, err)
			} else {
				require.ErrorIs(t, err, errcode.ErrInvalidParams)
			}
		})
	}
	require.ErrorIs(t, validateRiskDispatchCreate(nil, 11), errcode.ErrInvalidParams)
	assigned := int64(12)
	require.ErrorIs(t, validateRiskDispatchCreate(&model.CreateTicketRequest{TypeCode: "incident", Title: "synthetic", OrgID: 4, DefinitionVersion: "1", DefinitionHash: strings.Repeat("a", 64), AssignedTo: &assigned}, 11), errcode.ErrInvalidParams)
}

func TestRiskDispatchCreateRejectsUnguardedTransaction(t *testing.T) {
	s := &Service{ticketRepo: repository.NewTicketRepo(nil)}
	_, err := s.LockRiskDispatchCreateTx(context.Background(), nil, &RiskDispatchCreatePreparation{})
	require.ErrorIs(t, err, errcode.ErrNoPermission)
	called := false
	_, err = s.CreateRiskDispatchTicketTx(context.Background(), nil, &LockedRiskDispatchCreate{}, 22, func(context.Context, pgx.Tx, int64) error { called = true; return nil })
	require.ErrorIs(t, err, errcode.ErrNoPermission)
	require.False(t, called)
}

func TestCreatePreparationOwnsRequest(t *testing.T) {
	supervisor := int64(7)
	original := &model.CreateTicketRequest{CustomData: []byte(`{"x":1}`), Attachments: []string{"synthetic-a"}, EscalationSupervisor: &supervisor}
	copied := cloneCreateTicketRequest(original)
	original.CustomData[5] = '9'
	original.Attachments[0] = "synthetic-b"
	supervisor = 99
	require.JSONEq(t, `{"x":1}`, string(copied.CustomData))
	require.Equal(t, []string{"synthetic-a"}, copied.Attachments)
	require.Equal(t, int64(7), *copied.EscalationSupervisor)
}

type bridgeRowFunc func(...any) error

func (f bridgeRowFunc) Scan(dest ...any) error { return f(dest...) }

type bridgeRecordingTx struct {
	pgx.Tx
	t                     *testing.T
	steps                 []string
	queries               int
	actor, creator        int64
	fileOwner, uploadedBy int64
}

func (tx *bridgeRecordingTx) QueryRow(_ context.Context, q string, args ...any) pgx.Row {
	tx.queries++
	switch {
	case strings.Contains(q, "INSERT INTO tickets"):
		tx.steps = append(tx.steps, "ticket")
		tx.creator = args[5].(int64)
		return bridgeRowFunc(func(d ...any) error {
			*d[0].(*int64) = 101
			*d[1].(*time.Time) = time.Unix(1, 0)
			*d[2].(*time.Time) = time.Unix(1, 0)
			now := time.Unix(1, 0)
			*d[3].(**time.Time) = &now
			*d[4].(*int64) = 1
			return nil
		})
	case strings.Contains(q, "UPDATE tickets"):
		tx.steps = append(tx.steps, "sla")
		return bridgeRowFunc(func(d ...any) error { *d[0].(**time.Time) = nil; return nil })
	case strings.Contains(q, "array_agg(id)"):
		tx.steps = append(tx.steps, "file-lock")
		return bridgeRowFunc(func(d ...any) error { *d[0].(*[]int64) = []int64{301}; return nil })
	case strings.Contains(q, "SELECT count(*) FROM ticket_attachments"):
		return bridgeRowFunc(func(d ...any) error { *d[0].(*int) = 0; return nil })
	case strings.Contains(q, "FROM file_objects fo WHERE"):
		tx.steps = append(tx.steps, "file-check")
		return bridgeRowFunc(func(d ...any) error {
			*d[0].(*int64) = 301
			*d[1].(*string) = "attachment_create"
			*d[2].(*int64) = tx.fileOwner
			*d[3].(*string) = "confirmed"
			return nil
		})
	case strings.Contains(q, "INSERT INTO ticket_events"):
		tx.steps = append(tx.steps, "created")
		tx.actor = args[1].(int64)
		require.Equal(tx.t, EventCreated, args[2])
		return bridgeRowFunc(func(d ...any) error { *d[0].(*int64) = 201; *d[1].(*time.Time) = time.Unix(1, 0); return nil })
	default:
		tx.t.Fatalf("unexpected query: %s", q)
		return nil
	}
}
func (tx *bridgeRecordingTx) Exec(_ context.Context, q string, args ...any) (pgconn.CommandTag, error) {
	switch {
	case strings.Contains(q, "INSERT INTO ticket_attachments"):
		tx.uploadedBy = args[2].(int64)
		tx.steps = append(tx.steps, "file-bind")
	case strings.Contains(q, "UPDATE file_objects"):
		tx.steps = append(tx.steps, "file-owner")
	default:
		require.Contains(tx.t, q, "UPDATE tickets SET human_key")
		tx.steps = append(tx.steps, "key")
	}
	return pgconn.NewCommandTag("UPDATE 1"), nil
}

func syntheticPreparedCreate() *preparedTicketCreate {
	return &preparedTicketCreate{
		actor: 11, ttype: &model.TicketType{}, humanKey: "TK-synthetic-1",
		req:    model.CreateTicketRequest{TypeCode: "incident", OrgID: 4},
		ticket: model.Ticket{TypeCode: "incident", CreatedBy: 22, OrgID: 4, OrgPath: "synthetic", Status: StatusOpen, Priority: 3},
	}
}

func TestPreparedCreatePreservesIdentityAndWritesLinkBeforeFiles(t *testing.T) {
	s := &Service{ticketRepo: repository.NewTicketRepo(nil)}
	for _, files := range []bool{false, true} {
		t.Run(fmt.Sprint(files), func(t *testing.T) {
			p := syntheticPreparedCreate()
			if files {
				p.req.Attachments = []string{"synthetic-upload"}
			}
			tx := &bridgeRecordingTx{t: t}
			ticket := p.ticket
			err := s.writePreparedCreateTx(context.Background(), tx, p, &ticket, func(_ context.Context, supplied pgx.Tx, id int64) error {
				require.Same(t, tx, supplied)
				require.Equal(t, int64(101), id)
				require.Equal(t, "TK-synthetic-1", ticket.HumanKey)
				tx.steps = append(tx.steps, "link")
				return nil
			})
			if files {
				require.ErrorIs(t, err, errcode.ErrServiceUnavailable)
				require.Equal(t, []string{"ticket", "sla", "created", "key", "link"}, tx.steps)
			} else {
				require.NoError(t, err)
				require.Equal(t, []string{"ticket", "sla", "created", "key", "link"}, tx.steps)
				require.Equal(t, "TK-synthetic-1", ticket.HumanKey)
			}
			require.Equal(t, int64(22), tx.creator)
			require.Equal(t, int64(11), tx.actor)
			require.Equal(t, StatusOpen, ticket.Status)
			require.Nil(t, ticket.AssignedTo)
		})
	}
}

func TestPreparedCreateStopsOnLinkFailure(t *testing.T) {
	s := &Service{ticketRepo: repository.NewTicketRepo(nil)}
	p := syntheticPreparedCreate()
	p.req.Attachments = []string{"synthetic-upload"}
	tx := &bridgeRecordingTx{t: t}
	ticket := p.ticket
	rejected := errors.New("synthetic link rejected")
	err := s.writePreparedCreateTx(context.Background(), tx, p, &ticket, func(context.Context, pgx.Tx, int64) error { return rejected })
	require.ErrorIs(t, err, rejected)
	require.Equal(t, []string{"ticket", "sla", "created", "key"}, tx.steps)
}

func TestPreparedCreateValidatesBeforeWrite(t *testing.T) {
	s := &Service{ticketRepo: repository.NewTicketRepo(nil)}
	p := syntheticPreparedCreate()
	ticket := p.ticket
	ticket.CustomData = make([]byte, CustomDataMaxBytes+1)
	tx := &bridgeRecordingTx{t: t}
	err := s.writePreparedCreateTx(context.Background(), tx, p, &ticket, nil)
	var ec *errcode.Error
	require.ErrorAs(t, err, &ec)
	require.Equal(t, errcode.ErrInvalidParams.Code, ec.Code)
	require.Zero(t, tx.queries)
}

type createLockTx struct {
	pgx.Tx
	t       *testing.T
	steps   []string
	hash    string
	version int64
}

func (tx *createLockTx) QueryRow(_ context.Context, q string, args ...any) pgx.Row {
	switch {
	case strings.Contains(q, "FROM ticket_types"):
		require.Contains(tx.t, q, "FOR SHARE")
		tx.steps = append(tx.steps, "type")
		return bridgeRowFunc(func(d ...any) error {
			*d[0].(*int64) = 1
			*d[1].(*bool) = true
			v := tx.version
			*d[2].(**int64) = &v
			return nil
		})
	case strings.Contains(q, "FROM ticket_templates"):
		require.Contains(tx.t, q, "FOR SHARE")
		tx.steps = append(tx.steps, "template")
		return bridgeRowFunc(func(d ...any) error {
			*d[0].(*int64) = 2
			*d[1].(*int64) = 3
			*d[2].(*string) = "incident"
			*d[3].(*int) = 3
			*d[4].(*json.RawMessage) = []byte(`{}`)
			return nil
		})
	case strings.Contains(q, "FROM ticket_type_definitions"):
		tx.steps = append(tx.steps, "definition")
		require.Contains(tx.t, q, "definition_hash=ticket_definition_snapshot_hash(snapshot)")
		return bridgeRowFunc(func(d ...any) error {
			*d[0].(*string) = tx.hash
			*d[1].(*json.RawMessage) = []byte(`{"type":{"code":"incident","states":["open","assigned","closed"],"transitions":{"open":["assigned","closed"]},"transition_meta":{},"default_sla_hours":0,"has_custom_fields":false},"fields":[]}`)
			return nil
		})
	case strings.Contains(q, "FROM organizations"):
		require.Contains(tx.t, q, "FOR SHARE")
		tx.steps = append(tx.steps, "org")
		return bridgeRowFunc(func(d ...any) error { *d[0].(*int64) = 4; *d[5].(*string) = "synthetic.current"; return nil })
	default:
		tx.t.Fatalf("unexpected lock query: %s", q)
		return nil
	}
}

func TestPreparedCreateLockOrderAndDefinitionPin(t *testing.T) {
	for _, tc := range []struct {
		name, hash string
		version    int64
		expected   []string
		conflict   bool
	}{
		{"match", strings.Repeat("a", 64), 1, []string{"type", "template", "definition", "org"}, false},
		{"republished", strings.Repeat("a", 64), 2, []string{"type"}, true},
		{"wrong hash", strings.Repeat("b", 64), 1, []string{"type", "template", "definition"}, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s := &Service{ticketRepo: repository.NewTicketRepo(nil), orgRepo: repository.NewOrgRepo(nil)}
			p := syntheticPreparedCreate()
			v := int64(1)
			p.ttype = &model.TicketType{ID: 1, Code: "incident", IsActive: true, CurrentPublishedVersion: &v}
			p.ticket.DefinitionVersion = &v
			p.req.DefinitionVersion = "1"
			p.req.DefinitionHash = strings.Repeat("a", 64)
			p.req.TemplateCode = "synthetic-template"
			p.template = &model.TicketTemplate{ID: 2, Version: 3, TypeCode: "incident", DefaultPriority: 3, DefaultFields: []byte(`{}`)}
			tx := &createLockTx{t: t, hash: tc.hash, version: tc.version}
			ticket, err := s.lockPreparedCreateTx(context.Background(), tx, p, false)
			if tc.conflict {
				require.ErrorIs(t, err, errcode.ErrConcurrentModification)
			} else {
				require.NoError(t, err)
				require.Equal(t, "synthetic.current", ticket.OrgPath)
				// Two independently locked results never share a definition pointer.
				otherTx := &createLockTx{t: t, hash: tc.hash, version: tc.version}
				other, err := s.lockPreparedCreateTx(context.Background(), otherTx, p, false)
				require.NoError(t, err)
				// Results never expose pointers/bytes owned by the reusable preparation.
				*ticket.DefinitionVersion = 9
				require.Equal(t, int64(1), *p.ticket.DefinitionVersion)
				require.Equal(t, int64(1), *other.DefinitionVersion)
			}
			require.Equal(t, tc.expected, tx.steps)
		})
	}
}

// Both users are valid accounts: only the dispatcher's new upload is bindable.
func TestPreparedCreateFilesUseDispatcherNotCreator(t *testing.T) {
	for _, uploader := range []int64{11, 22} {
		t.Run(fmt.Sprint(uploader), func(t *testing.T) {
			s := &Service{ticketRepo: repository.NewTicketRepo(nil), storageRepo: repository.NewStorageRepo(nil), storageCfg: pkgstorage.NewConfigStore(&pkgstorage.StorageConfig{Bucket: "synthetic", MaxAttachmentsPerTicket: 10})}
			p := syntheticPreparedCreate()
			p.req.Attachments = []string{"synthetic-upload"}
			ticket := p.ticket
			tx := &bridgeRecordingTx{t: t, fileOwner: uploader}
			err := s.writePreparedCreateTx(context.Background(), tx, p, &ticket, func(context.Context, pgx.Tx, int64) error { tx.steps = append(tx.steps, "link"); return nil })
			require.Equal(t, []string{"ticket", "sla", "created", "key", "link", "file-lock", "file-lock", "file-check"}, tx.steps[:8])
			if uploader == 11 {
				require.NoError(t, err)
				require.Equal(t, int64(11), tx.uploadedBy)
			} else {
				var ec *errcode.Error
				require.ErrorAs(t, err, &ec)
				require.Equal(t, errcode.ErrInvalidParams.Code, ec.Code)
				require.Zero(t, tx.uploadedBy)
			}
		})
	}
}

type dispatchFetchSpy struct{ actor int64 }

func (f *dispatchFetchSpy) Fetch(_ context.Context, _ datafetcher.FetchSource, _ string, actor datafetcher.Actor) (map[string]any, error) {
	f.actor = actor.UserID
	return map[string]any{"value": "synthetic-result"}, nil
}
func TestRiskCreateFetchKeepsDispatcher(t *testing.T) {
	spy := &dispatchFetchSpy{}
	reg := datafetcher.NewRegistry()
	reg.Register("internal_db", spy)
	s := &Service{fetcherReg: reg}
	out := s.applyFetches("incident", []byte(`{"lookup":"synthetic-key"}`), 11, []*model.TicketTypeField{{FieldKey: "lookup", FieldMeta: []byte(`{"fetch":{"source":{"type":"internal_db"},"mapping":{"value":"result"}}}`)}})
	require.Equal(t, int64(11), spy.actor)
	require.JSONEq(t, `{"lookup":"synthetic-key","result":"synthetic-result"}`, string(out))
}
