# configx

`configx` загружает конфиг в Go-структуру из источников:

`flag > vault > env > yaml`

Пакет поддерживает:

- `cfgx`-теги (`key,default=...,optional`)
- вложенные структуры
- profile-группы `prod/dev`
- 3 режима резолва (`StrictGroup`, `OverlayDefaultHigh`, `OverlayDefaultLow`)
- агрегацию ошибок через `errors.Join`

## Быстрый старт

```go
package main

import (
	"context"
	"log"

	"github.com/dkoshenkov/packages-go/configx"
)

type AppConfig struct {
	Port int    `cfgx:"port,default=8080"`
	Host string `cfgx:"host"`
}

func main() {
	var cfg AppConfig
	err := configx.Load(context.Background(), &cfg,
		configx.ParseFlags(), // внутри: BindGlobalFlags + Parse(os.Args[1:])
		configx.WithYAMLFile("config.yaml"),
		configx.WithProfile("dev"),
	)
	if err != nil {
		log.Fatal(err)
	}

	log.Printf("host=%s port=%d", cfg.Host, cfg.Port)
}
```

## Теги

Базовый формат:

```go
`cfgx:"<key>[,default=<yaml_value>][,optional|required]"`
```

Правила:

- поле без `cfgx` пропускается
- по умолчанию поле `required`
- `optional` снимает обязательность
- `default=...` задается как YAML-литерал
- `env:"..."` и `yaml:"..."` переопределяют авто-ключи

Пример:

```go
type Config struct {
	Port int    `cfgx:"port,default=8080"`
	Name string `cfgx:"name,optional" env:"CUSTOM_NAME" yaml:"custom.name"`
}
```

## Вложенность и ключи

```go
type Config struct {
	Server struct {
		I int `cfgx:"i"`
	} `cfgx:"server"`
}
```

Для поля `Server.I` будут использоваться:

- env/vault/flag ключ: `SERVER_I`
- yaml ключ: `server.i`

С profile `prod`:

- env/vault/flag: `PROD_SERVER_I`
- yaml: `prod.server.i`

С profile `dev`:

- env/vault/flag: `DEV_SERVER_I`
- yaml: `dev.server.i`

## Профиль (`prod/dev`)

Определение профиля:

1. `WithProfile("prod"|"dev")`
2. `flag` ключ `env`
3. `vault` ключ `ENV`
4. `os.Getenv("ENV")`
5. `yaml` ключ `env`

Если профиль не найден или невалидный, `Load` возвращает ошибку.

## Режимы резолва

### `StrictGroup` (по умолчанию)

Для `prod` группы: `[PROD_, ""]`, для `dev`: `[DEV_, ""]`.

Если найдено хотя бы одно значение в profile-группе, используется только она. Fallback в no-prefix в этом случае запрещен.

### `OverlayDefaultHigh`

Порядок для поля:

1. profile-группа
2. `default=...`
3. no-prefix группа

### `OverlayDefaultLow`

Порядок для поля:

1. profile-группа
2. no-prefix группа
3. `default=...`

## Источники

### Flag

Через `WithFlagSet(*pflag.FlagSet)`.
Читаются только `Changed` флаги.

Флаги можно сгенерировать автоматически:

- `BindFlags(flagSet, &Cfg{})` — для произвольного `*pflag.FlagSet`
- `BindGlobalFlags(&Cfg{})` — для `pflag.CommandLine`
- `ParseFlags(args...)` в `Load(...)` — внутри делает `BindFlags` + `Parse`

### Vault

```go
type VaultReader interface {
	Get(ctx context.Context, key string) (value string, ok bool, err error)
}
```

Через `WithVault(...)`.

### Env

Через `os.LookupEnv`.

### YAML

Через `WithYAMLFile(path)` (внутри используется `viper`).

## Ошибки и panic-режим

- `Load(...) error` возвращает агрегированную ошибку (`errors.Join`)
- `MustLoad(...)` паникует при любой ошибке загрузки

## Опции

- `WithFlagSet(*pflag.FlagSet)`
- `WithVault(VaultReader)`
- `WithYAMLFile(path string)`
- `WithProfile("prod"|"dev")`
- `WithResolveMode(configx.StrictGroup|configx.OverlayDefaultHigh|configx.OverlayDefaultLow)`
- `WithAllowMissing()` — глобально отключает ошибку на `required` поля без значений
- `ParseFlags(args ...string)` — автогенерация+парсинг флагов внутри `Load`

## Дополнительно

- `BindFlags(*pflag.FlagSet, &Cfg{})` — автогенерация флагов из `cfgx`
- `BindGlobalFlags(&Cfg{})` — автогенерация глобальных флагов (`pflag.CommandLine`)
