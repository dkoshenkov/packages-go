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

Готовый провайдер (передаешь только креды):

```go
configx.WithVaultCredentials(configx.VaultCredentials{
	Address:   "https://vault.example.com",
	Token:     os.Getenv("VAULT_TOKEN"),
	Namespace: "team-a",            // optional
	Path:      "secret/data/myapp", // KV path
})
```

Те же креды можно передать через флаги в `ParseFlags(...)`:

- `--cfgx-vault-address=...`
- `--cfgx-vault-token=...`
- `--cfgx-vault-namespace=...`
- `--cfgx-vault-path=...`

Пример подключения:

```go
package main

import (
	"context"
	"fmt"

	"github.com/hashicorp/vault/api"

	"github.com/dkoshenkov/packages-go/configx"
)

type vaultReader struct {
	client *api.Client
	path   string // например: "secret/data/my-app"
}

func (v vaultReader) Get(_ context.Context, key string) (string, bool, error) {
	secret, err := v.client.Logical().Read(v.path)
	if err != nil {
		return "", false, err
	}
	if secret == nil || secret.Data == nil {
		return "", false, nil
	}

	// KV v2: значения лежат в поле data.
	rawData, ok := secret.Data["data"].(map[string]any)
	if !ok {
		return "", false, nil
	}
	raw, ok := rawData[key]
	if !ok {
		return "", false, nil
	}

	return fmt.Sprint(raw), true, nil
}

type Config struct {
	Token string `cfgx:"token"`
}

func loadCfg() (Config, error) {
	client, err := api.NewClient(api.DefaultConfig())
	if err != nil {
		return Config{}, err
	}

	var cfg Config
	err = configx.Load(context.Background(), &cfg,
		configx.ParseFlags(),
		configx.WithVault(vaultReader{
			client: client,
			path:   "secret/data/my-app",
		}),
		configx.WithProfile("dev"),
	)

	return cfg, err
}
```

Какие ключи читать из Vault:

- по `cfgx:"token"` ищется `TOKEN`
- с профилем `dev` сначала `DEV_TOKEN`, затем `TOKEN`
- профиль может читаться из Vault ключа `ENV`, если `WithProfile(...)` не передан

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
- `WithVaultCredentials(configx.VaultCredentials)`
- `WithYAMLFile(path string)`
- `WithProfile("prod"|"dev")`
- `WithResolveMode(configx.StrictGroup|configx.OverlayDefaultHigh|configx.OverlayDefaultLow)`
- `WithAllowMissing()` — глобально отключает ошибку на `required` поля без значений
- `ParseFlags(args ...string)` — автогенерация+парсинг флагов внутри `Load`
- `SeedDefaults(targets ...string)` — заполнение defaults/placeholders в `vault|yaml|env`
- `SeedForce()` — перезапись существующих значений при seed
- `SeedYAMLFile(path string)` / `SeedENVFile(path string)` — пути для seed-файлов
- `SeedOnly()` — только заполнить/сгенерировать значения и не выполнять резолв

## Дополнительно

- `BindFlags(*pflag.FlagSet, &Cfg{})` — автогенерация флагов из `cfgx`
- `BindGlobalFlags(&Cfg{})` — автогенерация глобальных флагов (`pflag.CommandLine`)

## Seed Через Флаги

В `ParseFlags(...)` доступны внутренние флаги:

- `--cfgx-seed-defaults` — включить seed
- `--cfgx-seed-targets=vault,yaml,env` — куда писать
- `--cfgx-seed-force` — перезаписывать существующие значения
- `--cfgx-seed-yaml-file=./config.yaml` — путь yaml-файла для seed
- `--cfgx-seed-env-file=./.env` — путь env-файла для seed
- `--cfgx-seed-only` — только seed (без загрузки значений в struct)
- `--cfgx-vault-address` — адрес Vault
- `--cfgx-vault-token` — токен Vault
- `--cfgx-vault-namespace` — namespace Vault
- `--cfgx-vault-path` — путь секрета (например `secret/data/myapp`)

Пример:

```bash
./app \
  --cfgx-seed-defaults \
  --cfgx-seed-targets=yaml,env \
  --cfgx-seed-yaml-file=./config.yaml \
  --cfgx-seed-env-file=./.env \
  --cfgx-seed-only
```
