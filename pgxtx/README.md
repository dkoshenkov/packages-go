# pgxtx

`ReadSnapshot` выполняет несколько запросов PostgreSQL в одной нативной
транзакции pgx с режимом `READ ONLY REPEATABLE READ`. Подходит, например,
для чтения количества строк и страницы результатов в одном снимке.

```go
names, err := pgxtx.ReadSnapshot(ctx, pool, time.Second, func(tx pgx.Tx) ([]string, error) {
    rows, err := tx.Query(ctx, "SELECT name FROM items ORDER BY name")
    if err != nil {
        return nil, err
    }
    return pgx.CollectRows(rows, pgx.RowTo[string])
})
```

Callback завершает все запросы и закрывает открытые rows до возврата.
Транзакцией владеет пакет. Освобождение через rollback имеет собственный
положительный таймаут и выполняется также при отмене или panic. Значение
возвращается только после успешного освобождения. При любой ошибке возвращается
нулевое значение типа. Если ошибками завершились и callback, и rollback,
вызывающий получает исходную ошибку callback.

Интеграционные тесты создают и удаляют тестовую таблицу. Для проверки на
выделенной базе задайте `PGXTX_TEST_DATABASE_URL` и выполните
`go test -race -count=1 -v ./pgxtx`.
