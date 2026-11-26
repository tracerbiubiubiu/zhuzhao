package handler

import (
	"errors"
	"net/http"
	"net/url"
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/tracerbiubiubiu/zhuzhao-utils/aksk"
	"github.com/tracerbiubiubiu/zhuzhao/internal/middleware"
	"github.com/tracerbiubiubiu/zhuzhao/internal/service/referenceassets"
	"github.com/tracerbiubiubiu/zhuzhao/pkg/referencequery"
)

type ReferenceAssetsHandler struct {
	service  *referenceassets.Service
	verifier *aksk.Verifier
	caller   string
}

func NewReferenceAssetsHandler(service *referenceassets.Service, ak string, key []byte) *ReferenceAssetsHandler {
	return &ReferenceAssetsHandler{service: service, verifier: &aksk.Verifier{Keys: map[string][]byte{ak: key}}, caller: ak}
}

// Query reads the selected synthetic asset directory through the reference app.
//
// @Summary Query the reference asset directory
// @ID referenceAssetListV1
// @Tags reference-assets
// @Produce json
// @Security BearerAuth
// @Param owner_org_refs query []string false "Organization filter" collectionFormat(multi)
// @Param status query []string false "Device status filter" collectionFormat(multi)
// @Param requested_fields query []string false "Fields to return" collectionFormat(multi)
// @Param limit query int false "Page size (1 to 50)"
// @Param cursor query string false "Cursor for the same query and session"
// @Success 200 {object} referencequery.QueryPage
// @Failure 400,403,409,429,503 {object} referencequery.Failure
// @Router /api/v1/extensions/reference-assets [get]
func (h *ReferenceAssetsHandler) Query(c *gin.Context) {
	if c.Request.URL.EscapedPath() != referencequery.QueryPath || len(c.Request.URL.RawQuery) > 8192 {
		writeReferenceError(c, &referencequery.Failure{Status: http.StatusBadRequest, Code: "invalid_input"})
		return
	}
	credential, ok := middleware.AuthenticatedReadCredential(c)
	if !ok {
		writeReferenceError(c, &referencequery.Failure{Status: http.StatusForbidden, Code: "not_authorized"})
		return
	}
	query, err := parseReferenceQuery(c.Request.URL.RawQuery)
	if err != nil {
		writeReferenceError(c, err)
		return
	}
	page, err := h.service.Read(c.Request.Context(), credential, c.GetString("request_id"), query)
	if err != nil {
		writeReferenceError(c, err)
		return
	}
	c.JSON(http.StatusOK, page)
}

func parseReferenceQuery(raw string) (referencequery.QueryRequest, error) {
	invalid := &referencequery.Failure{Status: http.StatusBadRequest, Code: "invalid_input"}
	values, err := url.ParseQuery(raw)
	if err != nil || len(values["limit"]) > 1 || len(values["cursor"]) > 1 {
		return referencequery.QueryRequest{}, invalid
	}
	for key := range values {
		switch key {
		case "limit", "cursor", "owner_org_refs", "status", "requested_fields":
		default:
			return referencequery.QueryRequest{}, invalid
		}
	}
	q := referencequery.QueryRequest{Cursor: values.Get("cursor"), Limit: 20, RequestedFields: values["requested_fields"],
		Parameters: referencequery.Parameters{OwnerOrgRefs: values["owner_org_refs"], Status: values["status"]}}
	if values.Has("limit") {
		q.Limit, err = strconv.Atoi(values.Get("limit"))
		if err != nil {
			return referencequery.QueryRequest{}, invalid
		}
	}
	if !values.Has("requested_fields") {
		q.RequestedFields = []string{"device_id", "name", "owner_org_ref", "category", "status", "observed_at", "stale"}
	}
	return q, nil
}

func (h *ReferenceAssetsHandler) Admit(c *gin.Context) {
	var inv referencequery.Invocation
	if err := referencequery.ReadSigned(c.Request, h.verifier, referencequery.AdmissionPath, &inv); err != nil {
		writeReferenceError(c, err)
		return
	}
	c.Set(aksk.ContextKeyCaller, h.caller)
	admission, err := h.service.Admit(c.Request.Context(), inv)
	if err != nil {
		writeReferenceError(c, err)
		return
	}
	c.JSON(http.StatusOK, admission)
}

func writeReferenceError(c *gin.Context, err error) {
	var failure *referencequery.Failure
	if !errors.As(err, &failure) {
		failure = &referencequery.Failure{Status: http.StatusServiceUnavailable, Code: "source_unavailable"}
	}
	if failure.RetryAfterMS != nil {
		c.Header("Retry-After", strconv.Itoa((*failure.RetryAfterMS+999)/1000))
	}
	c.JSON(failure.Status, failure)
}
