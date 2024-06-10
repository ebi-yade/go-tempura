# go-tempura

🍤 ebi の天ぷら、あるいは Go の template を使いやすくする仲間たち

en: Go template utilities

## Installation

```sh
go get github.com/ebi-yade/go-tempura
```

## Usage 1: `tempura.FuncMux`

引数に指定された string の列を受け取り、Prefix に応じた関数を呼び出し、値を探索します。

論よりコード:

```go
package main

import (
	"log"
	"os"
	"os/signal"
	"text/template"

	tempura "github.com/ebi-yade/go-tempura"
)

const configYAML = `# This is example: please load from file via embed/os package
db_user: {{ param "env/DB_USER" "env/MYSQL_USER" "default/root" }}
db_pass: {{ secret "manager.DB_PASS" "sops.DB_PASS" }}
`

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()

	param := tempura.FuncMux{
		tempura.SlashPrefix("env"):     tempura.Func(envCallback),
		tempura.SlashPrefix("default"): tempura.Func(defaultCallback),
	}
	if err := param.Validate(); err != nil {
		log.Fatalf("failed to validate param: %v", err)
	}

	secret := tempura.FuncMux{
		tempura.DotPrefix("manager"): tempura.FuncWithContextError(fetchSecretFromCloud),
		tempura.DotPrefix("sops"):    tempura.FuncWithError(sopsCallback),
	}.BindContext(ctx) // DO NOT FORGET TO USE context.Context
	if err := secret.Validate(); err != nil {
		log.Fatalf("failed to validate secret: %v", err)
	}

	tpl := template.Must(
		template.New("").Funcs(template.FuncMap{
			"param": param.Execute,
			"secret": secret.Execute,
		}).Parse(configYAML),
	)

	if err := tpl.Execute(os.Stdout, nil); err != nil {
		log.Fatalf("failed to execute template: %v", err)
	}
}

func envCallback(key string) (string, bool) {
	val := os.Getenv(key)
	if val == "" {
		return "", false
	}
	return val, true
}

func defaultCallback(key string) (string, bool) {
	return key, true
}

func fetchSecretFromCloud(ctx context.Context, key string) (string, bool, error) {
	// TODO: check out `contrib` package before implementing this function
	panic("not implemented")
}

func sopsCallback(key string) (string, bool, error) {
	panic("not implemented")
}

```
