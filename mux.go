package tempura

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
)

// =================================================================================
// Prefix types for MultiLookup keys
// =================================================================================

type Prefix interface {
	Match(string) bool
	Strip(string) string
}

type DotPrefix string

func (p DotPrefix) Match(s string) bool {
	return strings.HasPrefix(s, fmt.Sprintf("%s.", p))
}

func (p DotPrefix) Strip(s string) string {
	return strings.TrimPrefix(s, fmt.Sprintf("%s.", p))
}

type SlashPrefix string

func (p SlashPrefix) Match(s string) bool {
	return strings.HasPrefix(s, fmt.Sprintf("%s/", p))
}

func (p SlashPrefix) Strip(s string) string {
	return strings.TrimPrefix(s, fmt.Sprintf("%s/", p))
}

// =================================================================================
// Function types for MultiLookup values
// =================================================================================

type LookupAny func(val string) (any, bool)

type LookupAnyWithError func(val string) (any, bool, error)

type LookupAnyWithContext func(ctx context.Context, val string) (any, bool)

type LookupAnyWithContextError func(ctx context.Context, val string) (any, bool, error)

func Func[R any](fn func(val string) (R, bool)) LookupAny {
	return func(val string) (any, bool) {
		return fn(val)
	}
}

func FuncWithError[R any](fn func(val string) (R, bool, error)) LookupAnyWithError {
	return func(val string) (any, bool, error) {
		return fn(val)
	}
}

func FuncWithContext[R any](fn func(ctx context.Context, val string) (R, bool)) LookupAnyWithContext {
	return func(ctx context.Context, val string) (any, bool) {
		return fn(ctx, val)
	}
}

func FuncWithContextError[R any](fn func(ctx context.Context, val string) (R, bool, error)) LookupAnyWithContextError {
	return func(ctx context.Context, val string) (any, bool, error) {
		return fn(ctx, val)
	}
}

// LookupFunc は、MultiLookup に登録する個々関数で、prefixを取り除いたキーを文字列として受け取って何かしらの値を返す必要があります。
// LookupFunc インタフェースを満たすには、 tempura.FuncXXXX 経由で LookupAnyXXXX 型を生成することが推奨されます。
// tempura.FuncXXXX で登場するジェネリック型制約はanyですが、 template パッケージが処理できない型を利用すると実行時エラーになる可能性があります。
//
// LookupFunc represents individual functions registered in MultiLookup, which receive the key as a string, with the prefix removed, and return some value.
// It is recommended to generate a LookupAnyXXXX type through tempura.FuncXXXX to satisfy the LookupFunc interface.
// The generic type constraint in tempura.FuncXXXX is 'any', but using types that the template package cannot process might result in runtime errors.
type LookupFunc interface {
	_isSupportedLookupFunc()
}

func (fn LookupAny) _isSupportedLookupFunc() {}

func (fn LookupAnyWithError) _isSupportedLookupFunc() {}

func (fn LookupAnyWithContext) _isSupportedLookupFunc() {}

func (fn LookupAnyWithContextError) _isSupportedLookupFunc() {}

// MultiLookup は、1つまたは複数の文字列を引数として受け取るアクションにおいて、引数のプレフィックスに応じて異なる探索関数を実行するための機構です。
// ただし context.Context を受け取る関数も利用する場合は、 BindContext(ctx) を呼び出して MultiLookupContext を生成する必要があります。
//
// MultiLookup is a mechanism for executing different lookup functions depending on the prefix of the arguments in actions that take one or more strings as arguments.
// NOTE: If you want to use a function that takes context.Context, you need to call BindContext(ctx) to generate MultiLookupContext.
type MultiLookup map[Prefix]LookupFunc

func (m MultiLookup) Validate() error {
	if len(m) == 0 {
		return ErrNoFunctionRegistered
	}
	for k, v := range m {
		switch v.(type) {
		case LookupAny, LookupAnyWithError:
			slog.Debug(
				fmt.Sprintf("valid function of MultiLookup: %s", k),
				slog.Any("name", fmt.Sprintf("%s", v)),
				slog.Any("type", fmt.Sprintf("%T", v)),
			)

		case LookupAnyWithContext, LookupAnyWithContextError:
			err := InvalidFunctionError{Type: "MultiLookup", Prefix: k, Func: v}
			return fmt.Errorf("consider calling BindContext(ctx) to generate MultiLookupContext: %w", err)

		default:
			return InvalidFunctionError{Type: "MultiLookup", Prefix: k, Func: v}
		}
	}

	return nil
}

func (m MultiLookup) FuncMapValue(args ...string) (any, error) {
	for _, arg := range args {

		for prefix, fn := range m {
			if !prefix.Match(arg) {
				continue
			}
			suffix := prefix.Strip(arg)
			switch fn := fn.(type) {
			case LookupAny:
				slog.Debug(fmt.Sprintf("executing LookupAny for %s", arg))
				val, ok := fn(suffix)
				if ok {
					return val, nil
				}

			case LookupAnyWithError:
				slog.Debug(fmt.Sprintf("executing LookupAnyWithError for %s", arg))
				val, ok, err := fn(suffix)
				if err != nil {
					return nil, err
				}
				if ok {
					return val, nil
				}

			default:
				err := InvalidFunctionError{Type: "MultiLookup", Prefix: prefix, Func: fn}
				return nil, fmt.Errorf("consider calling Validate() to check the functions: %w", err)
			}
		}

	}

	return nil, MatchFailedError{Args: args, Prefixes: m.prefixes()}
}

func (m MultiLookup) prefixes() []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, fmt.Sprintf("%s", k))
	}
	return keys
}

func (m MultiLookup) BindContext(ctx context.Context) *MultiLookupContext {
	return &MultiLookupContext{
		MultiLookup: m,
		Ctx:         ctx,
	}
}

// MultiLookupContext は context.Context を受け取る関数を利用できる MultiLookup です。 BindContext(ctx) を呼び出して生成してください。
//
// MultiLookupContext is a MultiLookup that can use functions that accept context.Context. Generate it by calling BindContext(ctx).
type MultiLookupContext struct {
	MultiLookup MultiLookup
	Ctx         context.Context
}

func (m *MultiLookupContext) Validate() error {
	if m.Ctx == nil {
		return fmt.Errorf("consider calling BindContext(ctx): %w", ErrContextUntypedNil)
	}
	if len(m.MultiLookup) == 0 {
		return ErrNoFunctionRegistered
	}
	for prefix, fn := range m.MultiLookup {
		switch fn.(type) {
		case LookupAny, LookupAnyWithError, LookupAnyWithContext, LookupAnyWithContextError:
			slog.Debug(
				fmt.Sprintf("valid function of MultiLookupContext: %s", prefix),
				slog.Any("name", fmt.Sprintf("%s", fn)),
				slog.Any("type", fmt.Sprintf("%T", fn)),
			)
		default:
			return InvalidFunctionError{Type: "MultiLookupContext", Prefix: prefix, Func: fn}
		}
	}

	return nil
}

func (m *MultiLookupContext) FuncMapValue(args ...string) (any, error) {

	type result struct {
		val any
		ok  bool
		err error
	}
	results := make([]chan result, 0, len(args))
	for range args {
		results = append(results, make(chan result, 1))
	}

	ctx, cancel := context.WithCancel(m.Ctx)
	defer cancel()

	// 非同期処理の発火または同期処理実行
	// en: Fire asynchronous processing or execute synchronous processing
	for index, arg := range args {
		promise := results[index]

		for prefix, fn := range m.MultiLookup {
			if !prefix.Match(arg) {
				continue
			}
			suffix := prefix.Strip(arg)

			switch fn := fn.(type) {
			case LookupAny:
				slog.DebugContext(ctx, fmt.Sprintf("executing LookupAny for %s", arg))
				val, ok := fn(suffix)
				promise <- result{val: val, ok: ok, err: nil}
				close(promise)

			case LookupAnyWithError:
				slog.DebugContext(ctx, fmt.Sprintf("executing LookupAnyWithError for %s", arg))
				val, ok, err := fn(suffix)
				promise <- result{val: val, ok: ok, err: err}
				close(promise)

			case LookupAnyWithContext:
				slog.DebugContext(ctx, fmt.Sprintf("executing LookupAnyWithContext for %s", arg))
				go func() {
					val, ok := fn(ctx, suffix)
					promise <- result{val: val, ok: ok, err: nil}
					close(promise)
				}()

			case LookupAnyWithContextError:
				slog.DebugContext(ctx, fmt.Sprintf("executing LookupAnyWithContextError for %s", arg))
				go func() {
					val, ok, err := fn(ctx, suffix)
					promise <- result{val: val, ok: ok, err: err}
					close(promise)
				}()

			default:
				err := InvalidFunctionError{Type: "MultiLookupContext", Prefix: prefix, Func: fn}
				return nil, fmt.Errorf("unexpected error! it might be a bug: %w", err)
			}
		}

	}

	for _, promise := range results {
		select {
		case res := <-promise:
			if res.err != nil {
				return nil, res.err
			}
			if res.ok {
				return res.val, nil
			}
		}
	}

	return nil, MatchFailedError{Args: args, Prefixes: m.MultiLookup.prefixes()}
}

// =================================================================================
// Defined errors that you can handle with errors.Is / errors.As
// =================================================================================

var ErrNoFunctionRegistered = fmt.Errorf("no function registered")
var ErrContextUntypedNil = fmt.Errorf("context.Context is untyped nil")

type InvalidFunctionError struct {
	Type   string
	Prefix Prefix
	Func   any
}

func (e InvalidFunctionError) Error() string {
	return fmt.Sprintf("invalid function of %s: %+v with type %T", e.Type, e.Prefix, e.Func)
}

type MatchFailedError struct {
	Args     []string
	Prefixes []string
}

func (e MatchFailedError) Error() string {
	return fmt.Sprintf("match failed for args: %q within prefixes: %q", e.Args, e.Prefixes)
}
