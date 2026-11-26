// The synthetic asset source serves only the version 1 read-only directory query.
package main

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha256"
	_ "embed"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/tracerbiubiubiu/zhuzhao-utils/aksk"
	"github.com/tracerbiubiubiu/zhuzhao/pkg/referencequery"
)

//go:embed assets.json
var fixture []byte

const mappingVersion = "ref-assets-mapping-v1"
const maxFixtureBytes = 1 << 20

type assetRecord struct {
	DeviceID    string  `json:"device_id"`
	Name        string  `json:"name"`
	OwnerOrgRef string  `json:"owner_org_ref"`
	Category    string  `json:"category"`
	Status      string  `json:"status"`
	ObservedAt  *string `json:"observed_at"`
	Stale       *bool   `json:"stale"`
}

type assetSource struct {
	records    []assetRecord
	revision   string
	observedAt string
	cursorKey  []byte
	verifier   *aksk.Verifier
}

func main() {
	listen := flag.String("listen", "127.0.0.1:45335", "HTTP listen address")
	appAK := flag.String("app-ak", "", "trusted app service AK")
	keyFile := flag.String("app-key-file", "", "file containing the trusted app service key")
	fixturePath := flag.String("fixture", "", "optional synthetic asset fixture JSON file")
	flag.Parse()

	key, err := os.ReadFile(*keyFile)
	if err != nil {
		log.Fatal("unable to read the reference source key file")
	}
	source, err := loadSource(*fixturePath, *appAK, string(key))
	if err != nil {
		log.Fatal(err)
	}
	server := &http.Server{
		Addr: *listen, Handler: source,
		ReadHeaderTimeout: 2 * time.Second, ReadTimeout: 4 * time.Second,
		WriteTimeout: 4 * time.Second, IdleTimeout: 30 * time.Second, MaxHeaderBytes: 16 << 10,
	}
	log.Fatal(server.ListenAndServe())
}

func loadSource(path, ak, rawKey string) (*assetSource, error) {
	key := strings.TrimSpace(rawKey)
	if ak == "" || strings.ContainsAny(ak, "=, \t\r\n") || key == "" || len(key) > 1024 {
		return nil, errors.New("invalid reference source credential")
	}
	raw, err := loadFixture(path)
	if err != nil {
		return nil, err
	}
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.DisallowUnknownFields()
	var records []assetRecord
	if err := dec.Decode(&records); err != nil {
		return nil, errors.New("invalid synthetic asset fixture")
	}
	var trailing any
	if dec.Decode(&trailing) != io.EOF || records == nil {
		return nil, errors.New("synthetic asset fixture must contain one JSON array")
	}

	seen := make(map[string]bool, len(records))
	for _, r := range records {
		if !validRecord(r) || seen[r.DeviceID] {
			return nil, errors.New("invalid or duplicate asset in synthetic fixture")
		}
		seen[r.DeviceID] = true
	}
	slices.SortFunc(records, func(left, right assetRecord) int {
		if len(left.DeviceID) != len(right.DeviceID) {
			return len(left.DeviceID) - len(right.DeviceID)
		}
		return strings.Compare(left.DeviceID, right.DeviceID)
	})

	sum := sha256.Sum256(raw)
	return &assetSource{
		records: records, revision: hex.EncodeToString(sum[:]), observedAt: utcNow(),
		cursorKey: []byte(key), verifier: &aksk.Verifier{Keys: map[string][]byte{ak: []byte(key)}},
	}, nil
}

func loadFixture(path string) ([]byte, error) {
	if path == "" {
		return fixture, nil
	}
	f, err := os.Open(path)
	if err != nil {
		return nil, errors.New("unable to open the synthetic asset fixture")
	}
	defer f.Close()
	raw, err := io.ReadAll(io.LimitReader(f, maxFixtureBytes+1))
	if err != nil || len(raw) > maxFixtureBytes {
		return nil, errors.New("unable to read the bounded synthetic asset fixture")
	}
	return raw, nil
}

func validRecord(r assetRecord) bool {
	if !positiveID(r.DeviceID) || !positiveID(r.OwnerOrgRef) ||
		strings.TrimSpace(r.Name) == "" || len(r.Name) > 256 ||
		strings.TrimSpace(r.Category) == "" || len(r.Category) > 128 ||
		!slices.Contains([]string{"healthy", "degraded", "down", "unknown"}, r.Status) || r.Stale == nil {
		return false
	}
	if r.ObservedAt != nil {
		_, err := time.Parse(time.RFC3339Nano, *r.ObservedAt)
		return err == nil
	}
	return true
}

func positiveID(id string) bool {
	n, err := strconv.ParseUint(id, 10, 64)
	return err == nil && n > 0 && strconv.FormatUint(n, 10) == id
}

func (s *assetSource) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	var query referencequery.QueryRequest
	if failure := referencequery.ReadSigned(r, s.verifier, referencequery.ConnectorPath, &query); failure != nil {
		referencequery.WriteFailure(w, failure)
		return
	}
	if failure := requestFailure(query); failure != nil {
		referencequery.WriteFailure(w, failure)
		return
	}
	offset, valid := s.readCursor(query)
	if !valid {
		referencequery.WriteFailure(w, &referencequery.Failure{Status: http.StatusConflict, Code: "contract_mismatch"})
		return
	}

	page := referencequery.QueryPage{
		Items: make([]map[string]any, 0, query.Limit), Complete: true,
		SourceID: query.SourceID, SourceRevision: s.revision, MappingVersion: mappingVersion,
		ObservedAt: s.observedAt, FetchedAt: utcNow(), Warnings: []string{},
	}
	for i := offset; i < len(s.records); i++ {
		row := s.records[i]
		if !slices.Contains(query.Parameters.OwnerOrgRefs, row.OwnerOrgRef) ||
			(len(query.Parameters.Status) > 0 && !slices.Contains(query.Parameters.Status, row.Status)) {
			continue
		}
		if len(page.Items) == query.Limit {
			page.Complete = false
			page.NextCursor = s.cursor(query, i)
			break
		}
		page.Items = append(page.Items, row.project(query.RequestedFields))
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(page)
}

func requestFailure(q referencequery.QueryRequest) *referencequery.Failure {
	if q.ContractVersion != referencequery.Version || q.QueryID != referencequery.QueryID {
		return &referencequery.Failure{Status: http.StatusConflict, Code: "contract_mismatch"}
	}
	if err := referencequery.ValidateRequest(q); err != nil {
		return &referencequery.Failure{Status: http.StatusBadRequest, Code: "invalid_input"}
	}
	return nil
}

func (r assetRecord) project(fields []string) map[string]any {
	var observed any
	if r.ObservedAt != nil {
		observed = *r.ObservedAt
	}
	values := map[string]any{
		"device_id": r.DeviceID, "name": r.Name, "owner_org_ref": r.OwnerOrgRef,
		"category": r.Category, "status": r.Status, "observed_at": observed, "stale": *r.Stale,
	}
	projected := make(map[string]any, len(fields))
	for _, field := range fields {
		projected[field] = values[field]
	}
	return projected
}

func (s *assetSource) cursor(q referencequery.QueryRequest, offset int) string {
	q.Cursor = ""
	body, _ := json.Marshal(q)
	mac := hmac.New(sha256.New, s.cursorKey)
	fmt.Fprintf(mac, "reference-assets-source cursor v1\n%s\n%d\n%s", s.revision, offset, body)
	return strconv.Itoa(offset) + "." + base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
}

func (s *assetSource) readCursor(q referencequery.QueryRequest) (int, bool) {
	if q.Cursor == "" {
		return 0, true
	}
	index, _, found := strings.Cut(q.Cursor, ".")
	offset, err := strconv.Atoi(index)
	if !found || err != nil || offset <= 0 || offset >= len(s.records) {
		return 0, false
	}
	return offset, hmac.Equal([]byte(q.Cursor), []byte(s.cursor(q, offset)))
}

func utcNow() string {
	return time.Now().UTC().Format(time.RFC3339Nano)
}
