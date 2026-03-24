# logx

`logx` - обертка над `zerolog` для быстрого создания сервисного логера.

По умолчанию логер добавляет:

- `time`
- `caller`
- `service`

## Быстрый старт

```go
package main

import (
	"os"

	"github.com/dkoshenkov/packages-go/logx"
)

func main() {
	logger, err := logx.New("payments-api",
		logx.WithWriter(os.Stdout),
		logx.WithLevelText("debug"),
		logx.WithField("env", "dev"),
	)
	if err != nil {
		panic(err)
	}

	logger.Info().Msg("service started")
}
```

## Опции

- `WithWriter(io.Writer)`
- `WithServiceName(string)`
- `WithServiceFieldName(string)`
- `WithLevel(zerolog.Level)`
- `WithLevelText(string)`
- `WithCallerSkipFrameCount(int)`
- `WithoutCaller()`
- `WithoutTimestamp()`
- `WithPretty()`
- `WithTimeFormat(string)` (для `WithPretty`)
- `WithField(string, any)`
- `WithFields(map[string]any)`
