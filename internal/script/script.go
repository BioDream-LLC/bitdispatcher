// Package script runs an agency's scripts, in JavaScript (goja) or Lua (gopher-lua), behind one
// definition of what a script sees, may do, how long it may take and what its result means
// (DESIGN.md 5.1).
//
// A script is sandboxed: it has no file system, process, environment, network or debug access, only
// the input it is given and log(). It is always time-limited. It advises and never acts: what it returns
// is shown to a person, who decides. If it fails or runs out of time, the CAD carries on without it and
// says so.
//
// A script defines one function named for its hook, which takes the input and returns the result:
//
//	advise(incident)       -> nothing, or {title, detail, severity, action: {label, command}}, or a list of them
//	recommend(input)       -> nothing, or {prefer: [unit ids], notes: [text]}
//	check(report)          -> nothing, or a list of {field, message}
package script

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/dop251/goja"
	lua "github.com/yuin/gopher-lua"
	"github.com/yuin/gopher-lua/parse"
)

// Languages.
const (
	JS  = "javascript"
	Lua = "lua"
)

// Hooks, and the function each calls.
var Hooks = map[string]string{
	"incident":  "advise",
	"recommend": "recommend",
	"report":    "check",
}

// Limits.
const (
	DefaultTimeout = 50 * time.Millisecond
	MaxTimeout     = time.Second
	MaxSource      = 64 << 10
	maxLog         = 50
)

// Script is compiled code for one hook.
type Script struct {
	Lang string
	Hook string
	fn   string
	js   *goja.Program
	lua  *lua.FunctionProto
}

// Compile checks a script and prepares it to run.
func Compile(lang, hook, src string) (*Script, error) {
	fn, ok := Hooks[hook]
	if !ok {
		return nil, fmt.Errorf("no hook %q", hook)
	}
	if len(src) > MaxSource {
		return nil, errors.New("the script is longer than 64 KB")
	}
	s := &Script{Lang: lang, Hook: hook, fn: fn}
	switch lang {
	case JS:
		p, err := goja.Compile("script.js", src, true)
		if err != nil {
			return nil, fmt.Errorf("javascript: %w", err)
		}
		s.js = p
	case Lua:
		chunk, err := parse.Parse(strings.NewReader(src), "script.lua")
		if err != nil {
			return nil, fmt.Errorf("lua: %w", err)
		}
		proto, err := lua.Compile(chunk, "script.lua")
		if err != nil {
			return nil, fmt.Errorf("lua: %w", err)
		}
		s.lua = proto
	default:
		return nil, fmt.Errorf("the language is %s or %s", JS, Lua)
	}
	return s, nil
}

// Result is what a run gave: the function's return value as plain data (maps, lists, strings, numbers,
// booleans), and what it logged.
type Result struct {
	Value any           `json:"value"`
	Log   []string      `json:"log,omitempty"`
	Took  time.Duration `json:"took"`
}

// ErrTimeout is a script that ran out of time.
var ErrTimeout = errors.New("the script ran out of time")

// Run calls the hook's function with input, which is passed as plain data (it is converted through JSON,
// so the script never holds a Go value).
func (s *Script) Run(ctx context.Context, input any, timeout time.Duration) (Result, error) {
	if timeout <= 0 {
		timeout = DefaultTimeout
	}
	timeout = min(timeout, MaxTimeout)
	raw, err := json.Marshal(input)
	if err != nil {
		return Result{}, err
	}
	var plain any
	json.Unmarshal(raw, &plain)
	start := time.Now()
	var res Result
	switch s.Lang {
	case JS:
		res, err = s.runJS(ctx, plain, timeout)
	default:
		res, err = s.runLua(ctx, plain, timeout)
	}
	res.Took = time.Since(start)
	return res, err
}

func (s *Script) runJS(ctx context.Context, input any, timeout time.Duration) (res Result, err error) {
	vm := goja.New()
	vm.SetFieldNameMapper(goja.TagFieldNameMapper("json", true))
	logf := func(c goja.FunctionCall) goja.Value {
		if len(res.Log) < maxLog {
			var parts []string
			for _, a := range c.Arguments {
				parts = append(parts, a.String())
			}
			res.Log = append(res.Log, strings.Join(parts, " "))
		}
		return goja.Undefined()
	}
	vm.Set("log", logf)
	t := time.AfterFunc(timeout, func() { vm.Interrupt(ErrTimeout) })
	defer t.Stop()
	stop := context.AfterFunc(ctx, func() { vm.Interrupt(ctx.Err()) })
	defer stop()
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("javascript: %v", r)
		}
	}()
	if _, err = vm.RunProgram(s.js); err != nil {
		return res, jsErr(err)
	}
	f, ok := goja.AssertFunction(vm.Get(s.fn))
	if !ok {
		return res, fmt.Errorf("define function %s(input)", s.fn)
	}
	v, err := f(goja.Undefined(), vm.ToValue(input))
	if err != nil {
		return res, jsErr(err)
	}
	res.Value = plainJS(v.Export())
	return res, nil
}

func jsErr(err error) error {
	var ie *goja.InterruptedError
	if errors.As(err, &ie) {
		if e, ok := ie.Value().(error); ok {
			return e
		}
		return ErrTimeout
	}
	return fmt.Errorf("javascript: %w", err)
}

// plainJS turns goja's exports into the same shapes JSON gives.
func plainJS(v any) any {
	b, err := json.Marshal(v)
	if err != nil {
		return nil
	}
	var out any
	json.Unmarshal(b, &out)
	return out
}

func (s *Script) runLua(ctx context.Context, input any, timeout time.Duration) (res Result, err error) {
	L := lua.NewState(lua.Options{SkipOpenLibs: true, CallStackSize: 200, RegistrySize: 1024, RegistryMaxSize: 64 * 1024, MinimizeStackMemory: true})
	defer L.Close()
	// Only the base, table, string and math libraries, and without the base library's ways to load code
	// or reach outside.
	for _, lib := range []struct {
		name string
		fn   lua.LGFunction
	}{{lua.BaseLibName, lua.OpenBase}, {lua.TabLibName, lua.OpenTable}, {lua.StringLibName, lua.OpenString}, {lua.MathLibName, lua.OpenMath}} {
		L.Push(L.NewFunction(lib.fn))
		L.Push(lua.LString(lib.name))
		L.Call(1, 0)
	}
	for _, name := range []string{"dofile", "loadfile", "load", "loadstring", "require", "module", "collectgarbage", "getfenv", "setfenv", "rawequal", "rawset", "rawget", "newproxy", "print"} {
		L.SetGlobal(name, lua.LNil)
	}
	L.SetGlobal("log", L.NewFunction(func(L *lua.LState) int {
		if len(res.Log) < maxLog {
			var parts []string
			for i := 1; i <= L.GetTop(); i++ {
				parts = append(parts, L.ToStringMeta(L.Get(i)).String())
			}
			res.Log = append(res.Log, strings.Join(parts, " "))
		}
		return 0
	}))
	tctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	L.SetContext(tctx)
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("lua: %v", r)
		}
	}()
	L.Push(L.NewFunctionFromProto(s.lua))
	if err := L.PCall(0, 0, nil); err != nil {
		return res, luaErr(tctx, err)
	}
	f := L.GetGlobal(s.fn)
	if f.Type() != lua.LTFunction {
		return res, fmt.Errorf("define function %s(input)", s.fn)
	}
	L.Push(f)
	L.Push(toLua(L, input))
	if err := L.PCall(1, 1, nil); err != nil {
		return res, luaErr(tctx, err)
	}
	res.Value = fromLua(L.Get(-1), 0)
	return res, nil
}

func luaErr(ctx context.Context, err error) error {
	if errors.Is(ctx.Err(), context.DeadlineExceeded) {
		return ErrTimeout
	}
	if ctx.Err() != nil {
		return ctx.Err()
	}
	return fmt.Errorf("lua: %w", err)
}

func toLua(L *lua.LState, v any) lua.LValue {
	switch x := v.(type) {
	case nil:
		return lua.LNil
	case bool:
		return lua.LBool(x)
	case float64:
		return lua.LNumber(x)
	case string:
		return lua.LString(x)
	case []any:
		t := L.NewTable()
		for _, e := range x {
			t.Append(toLua(L, e))
		}
		return t
	case map[string]any:
		t := L.NewTable()
		for k, e := range x {
			t.RawSetString(k, toLua(L, e))
		}
		return t
	}
	return lua.LNil
}

// fromLua turns a Lua value into plain data: a table with keys 1..n is a list, else a map.
func fromLua(v lua.LValue, depth int) any {
	if depth > 20 {
		return nil
	}
	switch x := v.(type) {
	case lua.LBool:
		return bool(x)
	case lua.LNumber:
		return float64(x)
	case lua.LString:
		return string(x)
	case *lua.LTable:
		n := x.Len()
		if n > 0 {
			out := make([]any, 0, n)
			for i := 1; i <= n; i++ {
				out = append(out, fromLua(x.RawGetInt(i), depth+1))
			}
			return out
		}
		m := map[string]any{}
		x.ForEach(func(k, e lua.LValue) {
			if ks, ok := k.(lua.LString); ok {
				m[string(ks)] = fromLua(e, depth+1)
			}
		})
		if len(m) == 0 {
			return []any{}
		}
		return m
	}
	return nil
}
