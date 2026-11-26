// The reference asset application hosts the read-only directory profile and a
// frozen HTTP caller for checking Core upgrades.
package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"io"
	"log"
	"net/http"
	"os"
	"reflect"
	"slices"
	"strings"
	"time"

	"github.com/tracerbiubiubiu/zhuzhao-utils/aksk"
	"github.com/tracerbiubiubiu/zhuzhao/pkg/referencequery"
)

type appHandler struct {
	verifier  *aksk.Verifier
	admission *referencequery.Peer
	directory *referencequery.Client
	timeout   time.Duration
	inflight  chan struct{}
}

func main() {
	mode := flag.String("mode", "server", "server or compatibility client")
	listen := flag.String("listen", "127.0.0.1:45334", "HTTP listen address")
	coreAK := flag.String("core-ak", "", "trusted Core service AK")
	coreKeyFile := flag.String("core-key-file", "", "file containing the Core-to-app service key")
	admissionOrigin := flag.String("admission-origin", "", "Core origin for admission callbacks")
	admissionAK := flag.String("admission-ak", "", "app AK for admission callbacks")
	admissionKeyFile := flag.String("admission-key-file", "", "file containing the app-to-Core service key")
	sourceOrigin := flag.String("source-origin", "", "reference source origin")
	sourceAK := flag.String("source-ak", "", "app AK for directory reads")
	sourceKeyFile := flag.String("source-key-file", "", "file containing the app-to-source service key")
	timeout := flag.Duration("timeout", 2*time.Second, "total HTTP request timeout")
	callerOrigin := flag.String("core-origin", "", "Core origin for compatibility client mode")
	tokenFile := flag.String("token-file", "", "optional file containing the Core user's access token for client mode")
	query := flag.String("query", "limit=1&requested_fields=device_id", "URL query for compatibility client mode")
	flag.Parse()
	if *timeout <= 0 || *timeout > time.Minute {
		log.Fatal("invalid HTTP timeout")
	}

	switch *mode {
	case "client":
		token, err := optionalCredential(*tokenFile)
		if err != nil {
			log.Fatal(err)
		}
		if err := checkCore(*callerOrigin, token, *query, *timeout, os.Stdout); err != nil {
			log.Fatal(err)
		}
	case "server":
		coreKey := requiredCredential(*coreKeyFile)
		admissionKey := requiredCredential(*admissionKeyFile)
		sourceKey := requiredCredential(*sourceKeyFile)
		if *coreAK == "" || strings.ContainsAny(*coreAK, "=, \t\r\n") ||
			*coreAK == *admissionAK || *coreAK == *sourceAK || *admissionAK == *sourceAK ||
			coreKey == admissionKey || coreKey == sourceKey || admissionKey == sourceKey {
			log.Fatal("reference service identities must be valid and separate")
		}
		admission, err := referencequery.NewPeer(*admissionOrigin, *admissionAK, admissionKey, *timeout)
		if err != nil {
			log.Fatal("invalid reference app admission configuration")
		}
		source, err := referencequery.NewClient(*sourceOrigin, *sourceAK, sourceKey, *timeout)
		if err != nil {
			log.Fatal("invalid reference app source configuration")
		}
		server := &http.Server{
			Addr: *listen, Handler: newAppHandler(*coreAK, coreKey, admission, source, *timeout),
			ReadHeaderTimeout: 2 * time.Second, ReadTimeout: 4 * time.Second,
			WriteTimeout: *timeout + 2*time.Second, IdleTimeout: 30 * time.Second, MaxHeaderBytes: 16 << 10,
		}
		log.Fatal(server.ListenAndServe())
	default:
		log.Fatal("unrecognized reference app mode")
	}
}

func requiredCredential(path string) string {
	key, err := optionalCredential(path)
	if err != nil || key == "" {
		log.Fatal("missing or invalid reference service key")
	}
	return key
}

func optionalCredential(path string) (string, error) {
	if path == "" {
		return "", nil
	}
	f, err := os.Open(path)
	if err != nil {
		return "", errors.New("unable to open credential file")
	}
	defer f.Close()
	value, err := io.ReadAll(io.LimitReader(f, 8193))
	credential := strings.TrimSpace(string(value))
	if err != nil || len(value) > 8192 || credential == "" || strings.ContainsAny(credential, "\r\n") {
		return "", errors.New("invalid credential file")
	}
	return credential, nil
}

func newAppHandler(coreAK, coreKey string, admission *referencequery.Peer, directory *referencequery.Client, timeout time.Duration) http.Handler {
	return &appHandler{
		verifier:  &aksk.Verifier{Keys: map[string][]byte{coreAK: []byte(coreKey)}},
		admission: admission, directory: directory, timeout: timeout, inflight: make(chan struct{}, 8),
	}
}

func (a *appHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	var inv referencequery.Invocation
	if failure := referencequery.ReadSigned(r, a.verifier, referencequery.AppPath, &inv); failure != nil {
		referencequery.WriteFailure(w, failure)
		return
	}
	if inv.Audience != referencequery.ExtensionID || inv.CapabilityID != referencequery.QueryID ||
		inv.ContractVersion != referencequery.Version || inv.Method != http.MethodGet || inv.Path != referencequery.QueryPath ||
		inv.ConfigRevision == "" || len(inv.ConfigRevision) > 256 || inv.RequestRef == "" || len(inv.RequestRef) > 256 {
		referencequery.WriteFailure(w, &referencequery.Failure{Status: http.StatusForbidden, Code: "not_authorized"})
		return
	}
	if err := referencequery.ValidateInput(inv.Query); err != nil {
		writeQueryError(w, err)
		return
	}
	select {
	case a.inflight <- struct{}{}:
		defer func() { <-a.inflight }()
	default:
		referencequery.WriteFailure(w, &referencequery.Failure{Status: http.StatusTooManyRequests, Code: "quota"})
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), a.timeout)
	defer cancel()

	var admission referencequery.Admission
	if err := a.admission.Post(ctx, referencequery.AdmissionPath, inv, &admission); err != nil {
		writeQueryError(w, err)
		return
	}
	if !validAdmission(inv.Query, admission.Query) {
		referencequery.WriteFailure(w, &referencequery.Failure{Status: http.StatusServiceUnavailable, Code: "source_unavailable"})
		return
	}
	q := admission.Query
	if len(q.Parameters.OwnerOrgRefs) == 0 {
		if q.Cursor != "" {
			referencequery.WriteFailure(w, &referencequery.Failure{Status: http.StatusConflict, Code: "contract_mismatch"})
			return
		}
		now := time.Now().UTC().Format(time.RFC3339Nano)
		writePage(w, referencequery.QueryPage{
			Items: []map[string]any{}, Complete: true, SourceID: q.SourceID, MappingVersion: "authorization-only-v1",
			ObservedAt: now, FetchedAt: now, Warnings: []string{"source_not_queried"},
		})
		return
	}
	// The application holds the data-owner boundary. Verify owner and device
	// identity from the provider even when those fields are hidden from the caller.
	sourceQuery := q
	sourceQuery.RequestedFields = slices.Clone(q.RequestedFields)
	proofFields := []string{"owner_org_ref", "device_id"}
	if len(q.Parameters.Status) != 0 {
		proofFields = append(proofFields, "status")
	}
	for _, field := range proofFields {
		if !slices.Contains(sourceQuery.RequestedFields, field) {
			sourceQuery.RequestedFields = append(sourceQuery.RequestedFields, field)
		}
	}
	page, err := a.directory.Query(ctx, sourceQuery)
	if err != nil {
		writeQueryError(w, err)
		return
	}
	if ctx.Err() != nil {
		writeQueryError(w, ctx.Err())
		return
	}
	for _, row := range page.Items {
		for _, field := range proofFields {
			if !slices.Contains(q.RequestedFields, field) {
				delete(row, field)
			}
		}
	}
	writePage(w, page)
}

func validAdmission(original, admitted referencequery.QueryRequest) bool {
	if referencequery.ValidateInput(admitted) != nil {
		return false
	}
	for _, org := range admitted.Parameters.OwnerOrgRefs {
		if !slices.Contains(original.Parameters.OwnerOrgRefs, org) {
			return false
		}
	}
	original.Parameters.OwnerOrgRefs = nil
	admitted.Parameters.OwnerOrgRefs = nil
	return reflect.DeepEqual(original, admitted)
}

func writeQueryError(w http.ResponseWriter, err error) {
	var failure *referencequery.Failure
	if !errors.As(err, &failure) {
		failure = &referencequery.Failure{Status: http.StatusServiceUnavailable, Code: "source_unavailable"}
	}
	referencequery.WriteFailure(w, failure)
}

func writePage(w http.ResponseWriter, page referencequery.QueryPage) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(page)
}
