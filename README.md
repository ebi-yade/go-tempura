# go-tempura

🍤 ebi の天ぷら、あるいは Go の template を使いやすくする仲間たち

en: Go template utilities

## Installation

```sh
go get github.com/ebi-yade/go-tempura
```

## Usage 1: `tempura.MultiLookup`

`template.FuncMap` の値として代入可能な以下のユーティリティを `FuncMapValue` メソッドとして提供します。

1. テンプレート側の記述で引数に指定された string の列を受け取ります。
2. Prefix に応じたコールバックを呼び出し、同期または非同期で値を探索します。
3. 一番最初のキーで見つかった（関数が返す bool が true になった）値を返します。

論よりコード:

```go
package main

import (
	"fmt"
	"log"
	"os"
	"os/signal"
	"text/template"

	tempura "github.com/ebi-yade/go-tempura"
)

const configYAML = `# This is example: please load from file via embed/os package
db_user: {{ param "env/DB_USER" "env/MYSQL_USER" "default/root" }}
db_pass: {{ secret "manager.DB_PASS" "sops.DB_PASS" "default.p@ssword!" }}
`

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()

	lookupParams := tempura.MultiLookup{
		tempura.SlashPrefix("env"):     tempura.Func(getNonEmptyEnv),
		tempura.SlashPrefix("default"): tempura.Func(getKeyAsValue),
	}
	if err := lookupParams.Validate(); err != nil {
		log.Fatalf("failed to validate lookupParams: %+v", err)
	}

	lookupSecrets := tempura.MultiLookup{
		tempura.DotPrefix("manager"): tempura.FuncWithContextError(fetchCloudSecret),
		tempura.DotPrefix("sops"):    tempura.FuncWithError(getSopsSecret),
		tempura.DotPrefix("default"): tempura.Func(getKeyAsValue),
	}.BindContext(ctx) // DO NOT FORGET TO USE context.Context
	if err := lookupSecrets.Validate(); err != nil {
		log.Fatalf("failed to validate lookupSecrets: %+v", err)
	}

	tpl := template.Must(
		template.New("").Funcs(template.FuncMap{
			"param":  lookupParams.FuncMapValue,
			"secret": lookupSecrets.FuncMapValue,
		}).Parse(configYAML),
	)

	if err := tpl.Execute(os.Stdout, nil); err != nil {
		log.Fatalf("failed to execute template: %+v", err)
	}
}

// ======================================================================
// IMPORTANT NOTE:
//   探索関数が第二返り値で true を返すと、値が見つかったことを意味します。
//   そのため、第一返り値が "" などのゼロ値であっても、tempura はそれを採用します。
// ======================================================================

func getNonEmptyEnv(key string) (string, bool) {
	val := os.Getenv(key)
	if val == "" {
		return "", false
	}
	return val, true
}

func getKeyAsValue(key string) (string, bool) {
	return key, true
}

func fetchCloudSecret(ctx context.Context, key string) (string, bool, error) {
	return "", false, fmt.Errorf("not implemented")
}

func getSopsSecret(key string) (string, bool, error) {
	return "", false, fmt.Errorf("not implemented")
}

```
