package jobs

import (
	"strings"
	"testing"
)

func TestEqualParamsDepthBudget(t *testing.T) {
	for _, depth := range []int{256, 257} {
		raw := []byte(strings.Repeat("[", depth) + "0" + strings.Repeat("]", depth))
		equal, err := EqualParams(raw, raw)
		if depth == 256 && (err != nil || !equal) {
			t.Fatalf("legal boundary input rejected: %v", err)
		}
		if depth == 257 && err == nil {
			t.Fatal("excessively nested input accepted")
		}
	}
}

func TestEqualParamsExactNumbers(t *testing.T) {
	for _, tc := range []struct {
		a, b  string
		equal bool
	}{
		{`{"n":9007199254740992}`, `{"n":9007199254740993}`, false},
		{`{"a":[1e3,-0,0.01]}`, `{"a":[1000.0,0,1e-2]}`, true},
		{`1e9999999999999999999999999`, `10e9999999999999999999999998`, true},
		{`1e9999999999999999999999999`, `1e9999999999999999999999998`, false},
		{`null`, `{}`, true}, {`null`, `[]`, false}, {`"1"`, `1`, false}, {`[]`, `{}`, false}, {``, `{}`, true},
	} {
		eq, err := EqualParams([]byte(tc.a), []byte(tc.b))
		if err != nil || eq != tc.equal {
			t.Fatalf("%s vs %s want %v got %v %v", tc.a, tc.b, tc.equal, eq, err)
		}
	}
}
func TestEqualParamsRejectsAmbiguousOrTrailingInput(t *testing.T) {
	for _, raw := range []string{`{"a":1,"a":2}`, `{"a":[{"b":1,"b":2}]}`, `{} {}`, `[}`, `1e+`} {
		if _, err := EqualParams([]byte(raw), []byte(raw)); err == nil {
			t.Fatalf("must reject %s", raw)
		}
	}
}
