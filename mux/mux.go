package mux

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

type FuncMap map[Prefix]any

func (m FuncMap) Validate() error {
	for k, v := range m {
		switch v.(type) {
		case StringFunc, StringFuncWithError:
			slog.Debug(
				fmt.Sprintf("valid function of FuncMap: %s", k),
				slog.String("name", fmt.Sprintf("%s", v)),
				slog.String("type", fmt.Sprintf("%T", v)),
			)

		default:
			return fmt.Errorf("invalid function of FuncMap: %s with type %T", k, v)
		}
	}

	return nil
}

func (m FuncMap) Execute(args ...string) (string, error) {
	for _, arg := range args {

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

			default:
				return "", fmt.Errorf("invalid function of FuncMap: %s with type %T", prefix, fn)
			}
		}

	}

	return "", fmt.Errorf("not found")
}

type AsyncFuncMap struct {
	Map FuncMap
	Ctx context.Context
}

func (m FuncMap) BindContext(ctx context.Context) *AsyncFuncMap {
	return &AsyncFuncMap{
		Map: m,
		Ctx: ctx,
	}
}

func (m *AsyncFuncMap) Validate() error {
	for k, v := range m.Map {
		switch v.(type) {
		case StringFunc, StringFuncWithError, StringFuncWithContext, StringFuncWithContextError:
			slog.Debug(
				fmt.Sprintf("valid function of AsyncFuncMap: %s", k),
				slog.String("name", fmt.Sprintf("%s", v)),
				slog.String("type", fmt.Sprintf("%T", v)),
			)
		default:
			return fmt.Errorf("invalid function of AsyncFuncMap: %s with type %T", k, v)
		}
	}

	return nil
}

func (m *AsyncFuncMap) Execute(args ...string) (string, error) {

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
				return "", fmt.Errorf("invalid function of AsyncFuncMap: %s with type %T", prefix, fn)
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

	return "", fmt.Errorf("not found")
}
