package tempura_test

import (
	"context"
	"fmt"
	"os"
	"testing"

	"github.com/ebi-yade/go-tempura"
	"github.com/stretchr/testify/assert"
)

func TestFuncMux_Validate(t *testing.T) {
	t.Parallel()

	keyAsValue := func(val string) (string, bool) {
		return val, true
	}
	fetchSecret := func(ctx context.Context, key string) (string, bool, error) {
		return "XXXXXXXX", true, nil
	}
	allwaysError := func(val string) (string, bool, error) {
		return "", false, fmt.Errorf("this function always returns an error")
	}

	tests := []struct {
		name     string
		mux      *tempura.FuncMux
		checkErr func(t *testing.T, err error)
	}{
		// ==================== VALID CASES ====================
		{
			name: "single valid function",
			mux: &tempura.FuncMux{
				tempura.DotPrefix("env"): tempura.Func(os.LookupEnv),
			},
			checkErr: func(t *testing.T, err error) {
				assert.NoError(t, err)
			},
		},
		{
			name: "multiple valid functions",
			mux: &tempura.FuncMux{
				tempura.DotPrefix("env"):     tempura.Func(os.LookupEnv),
				tempura.DotPrefix("default"): tempura.Func(keyAsValue),
				tempura.DotPrefix("oops"):    tempura.FuncWithError(allwaysError),
			},
			checkErr: func(t *testing.T, err error) {
				assert.NoError(t, err)
			},
		},
		// ==================== INVALID CASES ====================
		{
			name: "no functions registered",
			mux:  &tempura.FuncMux{},
			checkErr: func(t *testing.T, err error) {
				assert.ErrorIs(t, err, tempura.ErrNoFunctionRegistered)
			},
		},
		{
			name: "contains invalid function that receives context",
			mux: &tempura.FuncMux{
				tempura.DotPrefix("env"):     tempura.Func(os.LookupEnv),
				tempura.DotPrefix("default"): tempura.Func(keyAsValue),
				tempura.DotPrefix("secret"):  tempura.FuncWithContextError(fetchSecret),
			},
			checkErr: func(t *testing.T, err error) {
				expected := tempura.InvalidFunctionError{}
				assert.ErrorAs(t, err, &expected)
				assert.Equal(t, "FuncMux", expected.MuxType, "MuxType mismatch")
				assert.Equal(t, tempura.DotPrefix("secret"), expected.Prefix, "Prefix mismatch")
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.mux.Validate()
			tt.checkErr(t, err)
		})
	}
}
