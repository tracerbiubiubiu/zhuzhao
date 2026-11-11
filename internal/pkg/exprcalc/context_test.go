package exprcalc

import (
	"context"
	"fmt"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"
)

type evaluationMarker struct{}

func TestContextFunction_RuntimeIsolation(t *testing.T) {
	AddFunction("context_marker", func(params ...any) (any, error) {
		ctx, ok := params[0].(context.Context)
		if !ok {
			return nil, fmt.Errorf("missing context: %T", params[0])
		}
		return ctx.Value(evaluationMarker{}), nil
	}, new(func(context.Context) string))
	const src = `context_marker()`
	require.NoError(t, CheckSyntax(src))
	first, err := compileCached(src)
	require.NoError(t, err)
	var wg sync.WaitGroup
	errors := make(chan error, 64)
	for i := 0; i < 64; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			want := fmt.Sprintf("request-%d", i)
			fields := map[string]any{contextEnvKey: "attacker", "ordinary": "untouched"}
			ctx := context.WithValue(context.Background(), evaluationMarker{}, want)
			got, err := EvalFormula(src, EvalEnv{Fields: fields, Context: ctx})
			if err != nil || got != want {
				errors <- fmt.Errorf("%s: got=%v err=%v", want, got, err)
			}
			if fields[contextEnvKey] != "attacker" {
				errors <- fmt.Errorf("mutated caller map")
			}
		}(i)
	}
	wg.Wait()
	close(errors)
	for err := range errors {
		require.NoError(t, err)
	}
	last, err := compileCached(src)
	require.NoError(t, err)
	require.Same(t, first, last, "same compiled program must read each invocation's runtime context")
}

func TestContextFunction_SandboxAndCompatibility(t *testing.T) {
	for _, src := range []string{`$env`, `$env["\\u001fexpr_context"]`, `$env.Context`, "\x1fexpr_context", `get($env, "ctx")`} {
		_, err := EvalFormula(src, EvalEnv{Fields: map[string]any{contextEnvKey: "attacker"}, Context: context.Background()})
		require.Error(t, err, src)
	}
	got, err := EvalFormula(`upper(value)`, EvalEnv{Fields: map[string]any{"value": "ok", contextEnvKey: "attacker"}})
	require.NoError(t, err)
	require.Equal(t, "OK", got)
	out, err := ApplyFormulas([]FormulaField{{Key: "copy", Formula: `upper(value)`, Type: "input"}}, map[string]any{"value": "ok"})
	require.NoError(t, err)
	require.Equal(t, "OK", out["copy"])
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err = EvalFormula(`upper(value)`, EvalEnv{Fields: map[string]any{"value": "ok"}, Context: ctx})
	require.ErrorIs(t, err, context.Canceled)
}
