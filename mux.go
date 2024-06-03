package tempura

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
)

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

type FuncMux map[Prefix]any

func (m FuncMux) Validate() error {
	for k, v := range m {
		switch v.(type) {
		case ReturningAny, ReturningAnyWithError:
			slog.Debug(
				fmt.Sprintf("valid function of FuncMux: %s", k),
				slog.Any("name", fmt.Sprintf("%s", v)),
				slog.Any("type", fmt.Sprintf("%T", v)),
			)

		default:
			return fmt.Errorf("invalid function of FuncMux: %s with type %T", k, v)
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
				return nil, fmt.Errorf("invalid function of FuncMux: %s with type %T", prefix, fn)
			}
		}

	}

	return nil, fmt.Errorf("not found")
}

type AsyncFuncMux struct {
	FuncMux FuncMux
	Ctx     context.Context
}

func (m FuncMux) BindContext(ctx context.Context) *AsyncFuncMux {
	return &AsyncFuncMux{
		FuncMux: m,
		Ctx:     ctx,
	}
}

func (m *AsyncFuncMux) Validate() error {
	for prefix, fn := range m.FuncMux {
		switch fn.(type) {
		case ReturningAny, ReturningAnyWithError, ReturningAnyWithContext, ReturningAnyWithContextError:
			slog.Debug(
				fmt.Sprintf("valid function of AsyncFuncMux: %s", prefix),
				slog.Any("name", fmt.Sprintf("%s", fn)),
				slog.Any("type", fmt.Sprintf("%T", fn)),
			)
		default:
			return fmt.Errorf("invalid function of AsyncFuncMux: %s with type %T", prefix, fn)
		}
	}

	return nil
}

func (m *AsyncFuncMux) Execute(args ...string) (any, error) {

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
				return nil, fmt.Errorf("invalid function of AsyncFuncMux: %s with type %T", prefix, fn)
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

	return nil, fmt.Errorf("not found")
}
