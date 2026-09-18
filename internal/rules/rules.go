package rules

import (
	"fmt"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/expr-lang/expr"
	"github.com/expr-lang/expr/vm"
)

const DefaultScript = `{ "dest": "cf" }`

type Result struct {
	Dest    string `expr:"dest"`
	DelayMs int    `expr:"delay_ms"`
}

type Engine struct {
	mu       sync.RWMutex
	path     string
	mtime    time.Time
	program  *vm.Program
	err      error
	source   string
}

func New(path string) *Engine {
	e := &Engine{path: path}
	_ = e.Reload()
	return e
}

func Path(dataDir string) string {
	return dataDir + "/rules.expr"
}

func EnsureDefault(path string) error {
	if _, err := os.Stat(path); err == nil {
		return nil
	}
	return os.WriteFile(path, []byte(DefaultScript+"\n"), 0o644)
}

func (e *Engine) Path() string { return e.path }

func (e *Engine) Source() string {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return e.source
}

func (e *Engine) CompileErr() string {
	e.mu.RLock()
	defer e.mu.RUnlock()
	if e.err == nil {
		return ""
	}
	return e.err.Error()
}

func (e *Engine) Reload() error {
	_ = EnsureDefault(e.path)
	info, err := os.Stat(e.path)
	if err != nil {
		e.mu.Lock()
		e.err = err
		e.program = nil
		e.mu.Unlock()
		return err
	}
	data, err := os.ReadFile(e.path)
	if err != nil {
		return err
	}
	src := strings.TrimSpace(string(data))
	if src == "" {
		src = DefaultScript
	}
	prog, err := expr.Compile(src, expr.Env(compileEnv()), expr.AsAny())
	e.mu.Lock()
	defer e.mu.Unlock()
	e.source = src
	e.mtime = info.ModTime()
	if err != nil {
		e.err = err
		e.program = nil
		return err
	}
	e.err = nil
	e.program = prog
	return nil
}

func (e *Engine) ReloadIfChanged() {
	info, err := os.Stat(e.path)
	if err != nil {
		return
	}
	e.mu.RLock()
	same := info.ModTime().Equal(e.mtime)
	e.mu.RUnlock()
	if !same {
		_ = e.Reload()
	}
}

func compileEnv() map[string]any {
	return map[string]any{
		"phase":    "",
		"host":     "",
		"path":     "",
		"request":  map[string]string{},
		"response": map[string]string{},
		"header":   headerFn,
		"match":    matchFn,
		"lower":    strings.ToLower,
	}
}

// Eval runs the script. Returns dest and/or delay_ms.
func (e *Engine) Eval(phase, host, path string, reqH, respH map[string]string) (Result, error) {
	e.ReloadIfChanged()
	e.mu.RLock()
	prog := e.program
	cerr := e.err
	e.mu.RUnlock()
	if prog == nil {
		if cerr != nil {
			return Result{Dest: "cf"}, fmt.Errorf("rules compile: %w", cerr)
		}
		return Result{Dest: "cf"}, nil
	}
	env := compileEnv()
	env["phase"] = phase
	env["host"] = host
	env["path"] = path
	env["request"] = lowerKeys(reqH)
	env["response"] = lowerKeys(respH)
	out, err := expr.Run(prog, env)
	if err != nil {
		return Result{Dest: "cf"}, err
	}
	return parseResult(out), nil
}

func parseResult(out any) Result {
	r := Result{Dest: "cf"}
	v, ok := out.(map[string]any)
	if !ok {
		return r
	}
	if d, ok := v["dest"].(string); ok && d != "" {
		r.Dest = d
	}
	switch n := v["delay_ms"].(type) {
	case int:
		r.DelayMs = n
	case int64:
		r.DelayMs = int(n)
	case float64:
		r.DelayMs = int(n)
	}
	return r
}

func lowerKeys(h map[string]string) map[string]string {
	out := make(map[string]string, len(h))
	for k, v := range h {
		out[strings.ToLower(k)] = v
	}
	return out
}

func headerFn(h map[string]string, name string) string {
	if h == nil {
		return ""
	}
	return h[strings.ToLower(name)]
}

func matchFn(re, s string) bool {
	ok, err := matchRegex(re, s)
	if err != nil {
		return false
	}
	return ok
}
