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
	"context"
	"os"

	"github.com/dkoshenkov/packages-go/logx"
)

func main() {
	ctx, err := logx.NewLogContext(context.Background(), "payments-api",
		logx.WithWriter(os.Stdout),
		logx.WithLevelText("debug"),
		logx.WithField("env", "dev"),
	)
	if err != nil {
		panic(err)
	}

	logx.InfoMsg(ctx, "service started")
}
```

## Контекстный API

- `NewLogContext(context.Context, string, ...Option)` создает логгер через `New` и кладет его в `context`
- `WithContext(context.Context, zerolog.Logger)` кладет логгер в `context`
- `FromContext(context.Context)` возвращает логгер из `context`
- `LogMsg(context.Context, zerolog.Level, string)` пишет сообщение с явным уровнем
- `TraceMsg/DebugMsg/InfoMsg/WarnMsg/ErrorMsg/FatalMsg/PanicMsg(context.Context, string)` пишут сообщение без `.Msg(...)`
- `Trace/Debug/Info/Warn/Error/Fatal/Panic(context.Context)` возвращают `*zerolog.Event` для сложных случаев с полями

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
