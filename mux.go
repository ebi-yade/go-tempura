package tempura

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
)

// =================================================================================
// Prefix types for FuncMux keys
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
// Function types for FuncMux values
// =================================================================================

type ReturningAny func(val string) (any, bool)

type ReturningAnyWithError func(val string) (any, bool, error)

type ReturningAnyWithContext func(ctx context.Context, val string) (any, bool)

type ReturningAnyWithContextError func(ctx context.Context, val string) (any, bool, error)

func Func[R any](fn func(val string) (R, bool)) ReturningAny {
	return func(val string) (any, bool) {
		return fn(val)
	}
}

func FuncWithError[R any](fn func(val string) (R, bool, error)) ReturningAnyWithError {
	return func(val string) (any, bool, error) {
		return fn(val)
	}
}

func FuncWithContext[R any](fn func(ctx context.Context, val string) (R, bool)) ReturningAnyWithContext {
	return func(ctx context.Context, val string) (any, bool) {
		return fn(ctx, val)
	}
}

func FuncWithContextError[R any](fn func(ctx context.Context, val string) (R, bool, error)) ReturningAnyWithContextError {
	return func(ctx context.Context, val string) (any, bool, error) {
		return fn(ctx, val)
	}
}

// MuxCallback は、FuncMux に登録する個々関数で、prefixを取り除いたキーを文字列として受け取って何かしらの値を返す必要があります。
// MuxCallback インタフェースを満たすには、 tempura.FuncXXXX 経由で ReturningAnyXXXX 型を生成することが推奨されます。
// tempura.FuncXXXX で登場するジェネリック型制約はanyですが、 template パッケージが処理できない型を利用すると実行時エラーになる可能性があります。
//
// MuxCallback represents individual functions registered in FuncMux, which receive the key as a string, with the prefix removed, and return some value.
// It is recommended to generate a ReturningAnyXXXX type through tempura.FuncXXXX to satisfy the MuxCallback interface.
// The generic type constraint in tempura.FuncXXXX is 'any', but using types that the template package cannot process might result in runtime errors.
type MuxCallback interface {
	_isSupportedMuxCallback()
}

func (fn ReturningAny) _isSupportedMuxCallback() {}

func (fn ReturningAnyWithError) _isSupportedMuxCallback() {}

func (fn ReturningAnyWithContext) _isSupportedMuxCallback() {}

func (fn ReturningAnyWithContextError) _isSupportedMuxCallback() {}

// FuncMux は、1つまたは複数の文字列を引数として受け取るアクションにおいて、引数のプレフィックスに応じて異なる関数を実行するための機構です。
// ただし context.Context を受け取る関数も利用する場合は、 BindContext(ctx) を呼び出して FuncMuxContext を生成する必要があります。
//
// FuncMux is a mechanism for executing different functions depending on the prefix of the arguments in actions that take one or more strings as arguments.
// NOTE: If you want to use a function that takes context.Context, you need to call BindContext(ctx) to generate FuncMuxContext.
type FuncMux map[Prefix]MuxCallback

func (m FuncMux) Validate() error {
	if len(m) == 0 {
		return ErrNoFunctionRegistered
	}
	for k, v := range m {
		switch v.(type) {
		case ReturningAny, ReturningAnyWithError:
			slog.Debug(
				fmt.Sprintf("valid function of FuncMux: %s", k),
				slog.Any("name", fmt.Sprintf("%s", v)),
				slog.Any("type", fmt.Sprintf("%T", v)),
			)

		case ReturningAnyWithContext, ReturningAnyWithContextError:
			err := InvalidFunctionError{MuxType: "FuncMux", Prefix: k, Func: v}
			return fmt.Errorf("consider calling BindContext(ctx) to generate FuncMuxContext: %w", err)

		default:
			return InvalidFunctionError{MuxType: "FuncMux", Prefix: k, Func: v}
		}
	}

	return nil
}

func (m FuncMux) Execute(args ...string) (any, error) {
	for _, arg := range args {

		for prefix, fn := range m {
			if !prefix.Match(arg) {
				continue
			}
			suffix := prefix.Strip(arg)
			switch fn := fn.(type) {
			case ReturningAny:
				slog.Debug(fmt.Sprintf("executing ReturningAny for %s", arg))
				val, ok := fn(suffix)
				if ok {
					return val, nil
				}

			case ReturningAnyWithError:
				slog.Debug(fmt.Sprintf("executing ReturningAnyWithError for %s", arg))
				val, ok, err := fn(suffix)
				if err != nil {
					return nil, err
				}
				if ok {
					return val, nil
				}

			default:
				err := InvalidFunctionError{MuxType: "FuncMux", Prefix: prefix, Func: fn}
				return nil, fmt.Errorf("consider calling Validate() to check the functions: %w", err)
			}
		}

	}

	return nil, MatchFailedError{Args: args, MuxKeys: m.keys()}
}

func (m FuncMux) keys() []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, fmt.Sprintf("%s", k))
	}
	return keys
}

func (m FuncMux) BindContext(ctx context.Context) *FuncMuxContext {
	return &FuncMuxContext{
		FuncMux: m,
		Ctx:     ctx,
	}
}

// FuncMuxContext は context.Context を受け取る関数を利用できる FuncMux です。 BindContext(ctx) を呼び出して生成してください。
//
// FuncMuxContext is a FuncMux that can use functions that accept context.Context. Generate it by calling BindContext(ctx).
type FuncMuxContext struct {
	FuncMux FuncMux
	Ctx     context.Context
}

func (m *FuncMuxContext) Validate() error {
	if len(m.FuncMux) == 0 {
		return ErrNoFunctionRegistered
	}
	for prefix, fn := range m.FuncMux {
		switch fn.(type) {
		case ReturningAny, ReturningAnyWithError, ReturningAnyWithContext, ReturningAnyWithContextError:
			slog.Debug(
				fmt.Sprintf("valid function of FuncMuxContext: %s", prefix),
				slog.Any("name", fmt.Sprintf("%s", fn)),
				slog.Any("type", fmt.Sprintf("%T", fn)),
			)
		default:
			return InvalidFunctionError{MuxType: "FuncMuxContext", Prefix: prefix, Func: fn}
		}
	}

	return nil
}

func (m *FuncMuxContext) Execute(args ...string) (any, error) {

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

		for prefix, fn := range m.FuncMux {
			if !prefix.Match(arg) {
				continue
			}
			suffix := prefix.Strip(arg)

			switch fn := fn.(type) {
			case ReturningAny:
				slog.Debug(fmt.Sprintf("executing ReturningAny for %s", arg))
				val, ok := fn(suffix)
				promise <- result{val: val, ok: ok, err: nil}
				close(promise)

			case ReturningAnyWithError:
				slog.Debug(fmt.Sprintf("executing ReturningAnyWithError for %s", arg))
				val, ok, err := fn(suffix)
				promise <- result{val: val, ok: ok, err: err}
				close(promise)

			case ReturningAnyWithContext:
				slog.Debug(fmt.Sprintf("executing ReturningAnyWithContext for %s", arg))
				go func() {
					val, ok := fn(ctx, suffix)
					promise <- result{val: val, ok: ok, err: nil}
					close(promise)
				}()

			case ReturningAnyWithContextError:
				slog.Debug(fmt.Sprintf("executing ReturningAnyWithContextError for %s", arg))
				go func() {
					val, ok, err := fn(ctx, suffix)
					promise <- result{val: val, ok: ok, err: err}
					close(promise)
				}()

			default:
				err := InvalidFunctionError{MuxType: "FuncMuxContext", Prefix: prefix, Func: fn}
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

	return nil, MatchFailedError{Args: args, MuxKeys: m.FuncMux.keys()}
}

// =================================================================================
// Defined errors that you can handle with errors.Is / errors.As
// =================================================================================

var ErrNoFunctionRegistered = fmt.Errorf("no function registered")

type InvalidFunctionError struct {
	MuxType string
	Prefix  Prefix
	Func    any
}

func (e InvalidFunctionError) Error() string {
	return fmt.Sprintf("invalid function of %s: %+v with type %T", e.MuxType, e.Prefix, e.Func)
}

type MatchFailedError struct {
	Args    []string
	MuxKeys []string
}

func (e MatchFailedError) Error() string {
	return fmt.Sprintf("match failed for args: %q with functions: %q", e.Args, e.MuxKeys)
}
