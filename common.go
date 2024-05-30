package tempurability

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
)

type StringFunc func(val string) string

type StringFuncWithError func(val string) (string, error)

type StringFuncWithContext func(ctx context.Context, val string) string

type StringFuncWithContextError func(ctx context.Context, val string) (string, error)

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
				return fn(suffix), nil

			case StringFuncWithError:
				slog.Debug(fmt.Sprintf("executing StringFuncWithError for %s", arg))
				return fn(suffix)
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
	src StringsMux
	ctx context.Context
}

func (m StringsMux) BindContext(ctx context.Context) *AsyncStringsMux {
	return &AsyncStringsMux{
		src: m,
		ctx: ctx,
	}
}

func (m *AsyncStringsMux) Validate() error {
	for k, v := range m.src {
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

// TODO: Implement Execute method for AsyncStringsMux
