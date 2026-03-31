# packages-go

Набор переиспользуемых Go-пакетов.

## Пакеты

- `flagx` — typed-обертка над `pflag` с `Default/Validate/OneOf`.
- `configx` — загрузка конфига в struct из `flag > vault > env > yaml`.
- `logx` — обертка над `zerolog` с готовой сервисной конфигурацией.
- `consterr` — простая строковая ошибка-константа.
- `stack` — generic LIFO stack и потокобезопасная реализация.
- `middlewarex` — generic middleware core + `net/http` adapters.

## Документация

- `configx`: [README](./configx/README.md)
- `consterr`: [README](./consterr/README.md)
- `stack`: [README](./stack/README.md)
- `flagx`: [README](./flagx/README.md)
- `logx`: [README](./logx/README.md)
- `middlewarex`: [README](./middlewarex/README.md)

## Lint

Репозиторий использует строгий `golangci-lint` с конфигом в `.golangci.yml`.

Запуск:

```bash
make lint
```

Нужен актуальный `golangci-lint`, совместимый с `go 1.25`.
При необходимости бинарь можно переопределить так:

```bash
make lint GOLANGCI_LINT=/path/to/golangci-lint
```
