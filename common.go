package tempurability

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
)

type StringFunc func(val string) (string, bool)

type StringFuncWithError func(val string) (string, bool, error)

type StringFuncWithContext func(ctx context.Context, val string) (string, bool)

type StringFuncWithContextError func(ctx context.Context, val string) (string, bool, error)

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

type StringsMux map[Prefix]any

func (m StringsMux) Validate() error {
	for k, v := range m {
		switch v.(type) {
		case StringFunc, StringFuncWithError:
			slog.Debug(
				fmt.Sprintf("valid function of StringsMux: %s", k),
				slog.String("name", fmt.Sprintf("%s", v)),
				slog.String("type", fmt.Sprintf("%T", v)),
			)

		default:
			return fmt.Errorf("invalid function of StringsMux: %s with type %T", k, v)
		}
	}

	return nil
}

func (m StringsMux) Execute(args ...string) (string, error) {
	for index, arg := range args {
		for prefix, fn := range m {
			if !prefix.Match(arg) {
				continue
			}
			suffix := prefix.Strip(arg)
			switch fn := fn.(type) {
			case StringFunc:
				slog.Debug(fmt.Sprintf("executing StringFunc for %s", arg))
				val, ok := fn(suffix)
				if ok {
					return val, nil
				}

			case StringFuncWithError:
				slog.Debug(fmt.Sprintf("executing StringFuncWithError for %s", arg))
				val, ok, err := fn(suffix)
				if err != nil {
					return "", err
				}
				if ok {
					return val, nil
				}
			}
		}

		if index == len(args)-1 {
			return arg, nil // default value
		}
		return "", fmt.Errorf("no match found for %s", arg)
	}

	return "", fmt.Errorf("no arguments provided")
}

type AsyncStringsMux struct {
	Map StringsMux
	Ctx context.Context
}

func (m StringsMux) BindContext(ctx context.Context) *AsyncStringsMux {
	return &AsyncStringsMux{
		Map: m,
		Ctx: ctx,
	}
}

func (m *AsyncStringsMux) Validate() error {
	for k, v := range m.Map {
		switch v.(type) {
		case StringFunc, StringFuncWithError, StringFuncWithContext, StringFuncWithContextError:
			slog.Debug(
				fmt.Sprintf("valid function of AsyncStringsMux: %s", k),
				slog.String("name", fmt.Sprintf("%s", v)),
				slog.String("type", fmt.Sprintf("%T", v)),
			)
		default:
			return fmt.Errorf("invalid function of AsyncStringsMux: %s with type %T", k, v)
		}
	}

	return nil
}

func (m *AsyncStringsMux) Execute(args ...string) (string, error) {

	type result struct {
		val string
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
		for prefix, fn := range m.Map {
			if !prefix.Match(arg) {
				continue
			}
			suffix := prefix.Strip(arg)

			switch fn := fn.(type) {
			case StringFunc:
				slog.Debug(fmt.Sprintf("executing StringFunc for %s", arg))
				val, ok := fn(suffix)
				promise <- result{val: val, ok: ok, err: nil}
				close(promise)

			case StringFuncWithError:
				slog.Debug(fmt.Sprintf("executing StringFuncWithError for %s", arg))
				val, ok, err := fn(suffix)
				promise <- result{val: val, ok: ok, err: err}
				close(promise)

			case StringFuncWithContext:
				slog.Debug(fmt.Sprintf("executing StringFuncWithContext for %s", arg))
				go func() {
					val, ok := fn(ctx, suffix)
					promise <- result{val: val, ok: ok, err: nil}
					close(promise)
				}()

			case StringFuncWithContextError:
				slog.Debug(fmt.Sprintf("executing StringFuncWithContextError for %s", arg))
				go func() {
					val, ok, err := fn(ctx, suffix)
					promise <- result{val: val, ok: ok, err: err}
					close(promise)
				}()

			default:
				if index == len(args)-1 {
					promise <- result{val: arg, ok: true, err: nil}
					close(promise)
				} else {
					return "", fmt.Errorf("no match found for %s", arg)
				}
			}
		}
	}

	for _, promise := range results {
		select {
		case res := <-promise:
			if res.err != nil {
				return "", res.err
			}
			if res.ok {
				return res.val, nil
			}
		}
	}

	return "", fmt.Errorf("no arguments provided")
}
