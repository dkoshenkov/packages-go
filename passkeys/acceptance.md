# Приёмка passkeys

Версия: локальная experimental-реализация от 2026-10-31, без опубликованного релиза. ТЗ: [webauthn-library-tz.md](../docs/webauthn-library-tz.md). Production-приёмка остаётся открытой.

## Выполненные проверки

Команды выполнялись из корня packages-go с `GOWORK=off`, `GOCACHE=/private/tmp/passkeys-go-cache`, для локальных Go-тестов также `GOPROXY=off`.

| Проверка | Команда / окружение | Результат |
|---|---|---|
| Весь модуль | `go test ./...`, Go 1.27.2 | PASS |
| Совместимость Go 1.25 | `GOTOOLCHAIN=go1.25.7 go test ./...` | PASS; go.mod остаётся 1.25.6, точная patch-версия 1.25.6 не запускалась |
| Data races | `go test -race ./passkeys/...` | PASS |
| Статический анализ | `go vet ./...` | PASS |
| Форматирование | `gofmt -l passkeys` | Пустой вывод |
| Fuzz JSON | `go test ./passkeys -run '^$' -fuzz '^FuzzJSON$' -fuzztime=10s -parallel=2` | PASS, 31 592 выполнения |
| Fuzz CBOR/key | То же с `-fuzz '^FuzzCBOR$'` | PASS, 372 437 выполнений |
| Fuzz authenticatorData | То же с `-fuzz '^FuzzAuthenticatorData$'` | PASS, 525 905 выполнений |
| Fuzz Finish | То же с `-fuzz '^FuzzFinish$'` | PASS после исправления контекста: 244 837 выполнений, два worker; каждый процесс проверяет успешность обоих seeds |
| Независимый фиксированный вектор | `TestIndependentOpenSSLVector` | PASS: Python encoder + OpenSSL signature, без построения через passkeys |
| Interoperability | `testdata/interop.go`, отдельный временный модуль, go-webauthn v0.18.2 | PASS: reference verify, JSON credential round trip, импорт, вход, отказ unsupported algorithm |
| Browser | `example/browser-test.cjs`, Chrome/151.0.7922.34, CDP virtual authenticator CTAP2.1/internal/UV/resident | PASS: регистрация, discoverable login, второй ключ, вход вторым ключом |
| golangci-lint | Бинарник отсутствует | Не запускался; `go vet` его не заменяет |

Первоначальный результат `FuzzFinish` был переоценён: workers создавали случайные challenge/ключи и отклоняли seeds до глубоких проверок. Для исправленного target используются фиксированный контекст и проверка успешности обоих seeds в каждом процессе.

Fuzz-бюджет был коротким smoke-прогоном; он не является исчерпывающим исследованием парсера. Crash-regressions не найдены. Начальные семена заданы в fuzz-функциях, независимый фиксированный вектор хранится в testdata. Более длительные кампании и независимый аудит нужны до production.

## Исправления после независимого AI-review

Три ревьюера `gpt-6-astra`/high подтвердили замечания; три исполнителя `gpt-6.1-terra` внесли исправления. Это проверка кода агентами, не завершённый независимый security-аудит для production.

| Замечание | Исправление и регрессия |
|---|---|
| P2: устаревшее время при ожидании mutex в примере | `register`/`login` вызывают часы после захвата блокировки. `TestCommitClockUnderLock` проверяет порядок, `TestExpiryWhileWaitingForCommit` — отказ истёкшему flow/разрешению без частичных изменений. |
| P2: разные challenge/ключи в fuzz workers | `FuzzFinish` загружает фиксированный OpenSSL fixture, подставляет общий challenge и проверяет оба Finish до запуска fuzz callback. Настоящий 10-секундный прогон с двумя workers прошёл: 244 837 выполнений. |
| LOW: повторные NaN-ключи во вложенной CBOR-карте | Общий DecMode запрещает NaN в ключах и значениях. `TestRegistrationExtensionNaN` проверяет отказ обеим формам и принятие обычного неизвестного расширения. |

Для исправленного `FuzzFinish` использовался `GOCACHE=/private/tmp/passkeys-terra-fuzz`. Общая повторная проверка: `go test -race ./passkeys/...`, `GOTOOLCHAIN=go1.25.7 go test ./...`, `go vet ./...`; результаты PASS. Повторные browser smoke (Chrome/151.0.7922.34) и interoperability с go-webauthn v0.18.2 также прошли. Публичные сигнатуры пакета не изменены. Политика `topOrigin` сохранена; её нормативное уточнение осталось отдельным неблокирующим замечанием.

## Повторное Astra-review

Три независимых `gpt-6-astra`/high ревьюера повторно проверили protocol core, API/тесты и интеграцию. Подтверждённых блокирующих замечаний для experimental PR не найдено. Unit/race проверки прошли. Дополнительные probes вне workspace подтвердили отказ NaN float16/32/64 в ключах и значениях и для регистрации, и для подписанного assertion; DER trailing bytes и чужой known-user owner также отклонены.

Для проверки регрессии времени ревьюер перенёс чтение часов до mutex в отдельной копии: `TestCommitClockUnderLock` упал для login и registration. Исходные clock regressions прошли 100 повторов под race detector. Interop с go-webauthn v0.18.2 повторно прошёл.

Повторный `FuzzFinish` с `-fuzztime=10s -fuzzminimizetime=100ms -parallel=2` выполнил 53 491 мутацию; новый процесс успешно воспроизвёл baseline 181/181 из сохранённого корпуса. Проверка выполнялась с `GOCACHE=/private/tmp/passkeys-astra-r2-tests`. Без ограничения минимизации короткий прогон тоже прошёл, но сделал лишь 4–5 выполнений; такие результаты не считаются широким fuzz evidence.

## Матрица ТЗ

Б — проверка ядра. И — интеграция локального примера. Модель транзакций использует общий store одного процесса; она проверяет алгоритм commit, но не доказывает свойства настоящей БД или отдельных OS-реплик. Р — ручная проверка реальных устройств; все такие пункты пока открыты. Browser smoke проверяет успешный вход вторым ключом, но не сравнивает идентификатор аккаунта до и после добавления; сохранение владельца проверяется модельным тестом.

| ID | Доказательство | Результат / оставшаяся граница |
|---|---|---|
| R01 | `TestRegistrationAndLogin`, `TestIndependentOpenSSLVector`, `TestHTTPFinishWithIndependentVector`, browser test | Б/И PASS |
| R02 | `TestRegistrationTransactions`, `TestFreshAuthorizationAtCommit`, `TestExpiryWhileWaitingForCommit`, browser test | Второй ключ и свежий вход PASS; реальные устройства Р открыто |
| R03 | `TestRegistrationTransactions` | Уникальность в одном/разных аккаунтах PASS в модели; реальное ограничение БД не проверено |
| R04 | `TestHTTPBoundariesAndCancellation`, обработка NotAllowedError в index.html | Abandoned flow/expiry PASS; реальная отмена/отсутствие устройства Р открыто |
| R05 | `TestRegistrationTampering`, общий clientData/checkState parser | UP/UV/RP и отказ без результата PASS; type/origin/challenge отдельно проверены общим кодом в `TestLoginTampering` |
| R06 | `TestStateChecks`, привязка flow cookie в HTTP-примере | Общий verifier контекста/срока PASS; browser-session isolation обеспечивается серверным lookup |
| R07 | `TestRegistrationTampering` | Неподдерживаемые fmt/alg PASS |
| R08 | `TestRegistrationTampering`, `TestBinaryBoundaries` | Кривая, координаты, размеры, private/optional COSE PASS |
| A01 | `TestRegistrationAndLogin` | Known-user allow list и nullable handle PASS |
| A02 | `TestRegistrationAndLogin`, `TestHTTPFinishWithIndependentVector`, browser test | Б/И/virtual PASS; реальные платформы Р открыто |
| A03 | `TestLoginTampering`, `TestKnownUserBinding` | Чужие handle, владелец и allow list PASS |
| A04 | `TestRecheckAfterVerification`, HTTP lookup в finishLogin | Удалённый key при commit PASS в модели; неизвестный key не создаёт аккаунт |
| A05 | `TestLoginTampering`, `TestCountersBackupAndOriginalBytes` | Подпись/изменение подписанных bytes PASS |
| A06 | `TestLoginTampering` | id, rawId, type, RP hash, origin, challenge проверены отдельными случаями |
| A07 | `TestLoginTampering` | UP, UV, crossOrigin PASS |
| A08 | `TestCountersBackupAndOriginalBytes`, OpenSSL vector | Пробелы/порядок исходного JSON PASS; повторная сериализация ломает подпись |
| A09 | `TestStateChecks`, `TestExpiryWhileWaitingForCommit`, `TestCommitClockUnderLock` | Ровно deadline — отказ; за 1 ns до deadline — успех; время commit читается после получения mutex |
| A10 | `TestCountersBackupAndOriginalBytes` | 0/0, рост счётчика PASS |
| A11 | `TestCountersBackupAndOriginalBytes`, `TestRecheckAfterVerification` | Anomaly signal и отказ по политике примера PASS |
| A12 | `TestCountersBackupAndOriginalBytes`, `TestLoginTampering` | Изменение BS, неизменность BE и BS без BE PASS |
| C01 | `TestSingleCommitAcrossServices`, `TestHTTPFinishWithIndependentVector` | Последовательный replay отклонён |
| C02 | `TestSingleCommitAcrossServices` | Два service instances, один общий store: ровно один commit. Реальная многопроцессная БД открыта |
| C03 | `TestRecheckAfterVerification`, `TestRegistrationTransactions` | Отзыв, recovery, version change между verify/commit отклонены в модели |
| C04 | `TestRecheckAfterVerification`, `TestRegistrationTransactions` | Инъекция сбоя до commit, без частичных изменений. Реальные ошибки БД/commit открыты |
| C05 | `TestSingleCommitAcrossServices` | Потерянный результат модели не допускает вторую сессию; новый flow работает. Сетевой fault injection открыт |
| C06 | `TestRegistrationAndLogin` | JSON round trip State + новый RP PASS. Перезапуск durable сервиса с БД открыт; demo память не сохраняет |
| C07 | `TestConcurrentIndependentOperations`, `go test -race` | Параллельная проверка и begin PASS |
| P01 | `TestStrictParsing`, `TestBinaryBoundaries`, четыре fuzz target | Усечения/неверный base64/JSON/CBOR без panic |
| P02 | `TestStrictParsing`, `TestBinaryBoundaries`, `TestRegistrationExtensionNaN` | JSON/CBOR дубли, escaped JSON key, неверные типы и NaN в ключах/значениях PASS |
| P03 | `TestBeginLimitsAndRandomFailure`, `TestBinaryBoundaries`, `TestStrictParsing` | Пределы ответа/client/auth/ID/count/depth PASS |
| P04 | `TestLoginTampering`, `TestRegistrationTampering`, `TestStrictParsing` | AT/ED и trailing data PASS |
| P05 | `TestCountersBackupAndOriginalBytes`, `TestBinaryBoundaries` | Unknown optional JSON/extension output допустимы, прав не дают |
| P06 | `TestBeginLimitsAndRandomFailure` | Ошибка reader: нулевые options/state, без fallback |
| P07 | `TestConfig` | HTTP/wildcard/public suffix/unrelated origins/неверные URL отклонены |
| P08 | `TestStrictParsing`; все категории ошибок — фиксированные строки | Payload не попадает в ошибку; логирования в ядре нет |
| D01 | — | Реальный телефон → ноутбук через QR не проверен |
| D02 | Browser test подтверждает только добавление отдельного virtual key | QR/физические устройства не проверены |
| D03 | `TestCountersBackupAndOriginalBytes` | BE/BS и нулевые счётчики PASS; реальная синхронизация не проверена |
| D04 | — | Реальный ноутбук → телефон не проверен |
| D05 | — | Реальный аппаратный ключ не проверен |
| M01 | `testdata/interop.go` | Формат go-webauthn v0.18.2 проверен; production auth-go и его записи не исследовались |
| M02 | `testdata/interop.go` + нулевые error results ядра | Unsupported algorithm отказ без изменения записи. Production-процесс миграции открыт |

## Соответствие нормативным операциям

Ссылки ведут на фиксированную редакцию [WebAuthn Level 3 от 2026-08-25](https://www.w3.org/TR/2026/REC-webauthn-3-20260825/).

| Правило | Код | Проверка |
|---|---|---|
| §7.1 / §7.2: серверный challenge/options и ожидаемый контекст | `BeginRegistration`, `BeginLogin`, `checkState` | `TestBeginLimitsAndRandomFailure`, `TestStateChecks` |
| §7.1 / §7.2: type, challenge, origin, crossOrigin/topOrigin | `clientData` | `TestLoginTampering`, `TestCountersBackupAndOriginalBytes` |
| §7.1: fmt/attStmt, RP hash, flags, alg, credential ID | `FinishRegistration`, `parseAuth`, `publicKey`, `checkAuth` | `TestRegistrationTampering`, `TestBinaryBoundaries` |
| §6.5.1: AAGUID/credential ID/COSE EC2 key | `parseAuth`, `publicKey` | `TestRegistrationTampering`, независимый fixture |
| §7.2: соответствие credential и пользователя, allow list | `FinishLogin`, `checkHandle` | `TestKnownUserBinding`, `TestRegistrationAndLogin`, `TestLoginTampering` |
| §7.2: signature over raw authData + SHA256(raw clientDataJSON) | `FinishLogin` | OpenSSL fixture, `TestCountersBackupAndOriginalBytes` |
| §6.1 / §7.2: UP/UV, BE/BS, signCount | `checkAuth`, `FinishLogin` | `TestCountersBackupAndOriginalBytes`, `TestLoginTampering` |
| §6.1: длины, AT/ED и RFU | `parseAuth` | `TestBinaryBoundaries`, `TestStrictParsing`, `TestLoginTampering` |
| §2.4: CBOR | `New` DecMode, `parseAuth`, `publicKey`, `FinishRegistration` | Дубли/лимиты запрещены; допустимые отклонения canonical порядка/минимальности описаны в README |
| Уникальность, расход challenge, выпуск сессии и актуальность аккаунта | Сервис: `example/store.register`, `example/store.login`, `transaction.sql` | Транзакционные model tests; live DB ещё не проверена |
| Полная attestation trust validation, другие alg, cross-origin/related origins, AppID, расширения с дополнительными правами | Исключены профилем | Unsupported ошибки или игнорирование необязательных outputs без расширения прав |

## Перед production

Нужны независимое security review с закрытием подтверждённых дефектов; длительный fuzzing; реальные транзакционные проверки выбранной БД с несколькими процессами и ошибками commit; ручная матрица ОС/браузера/провайдера/QR/синхронизации. Сертификация FIDO/WebAuthn не заявляется. Коммиты, публикация и миграция auth-go не выполнялись.

## Подготовка PR на актуальном main

База: `dkoshenkov/packages-go`, main `ec269923c64b1acd19047a7a49bfc71d2154cad5`. Сохранены Go 1.27.3 и существующие версии зависимостей upstream; добавлены CBOR v2.10.0 и его float16 dependency, существующий x/net v0.60.0 отмечен прямой зависимостью. Другие пакеты и CI не менялись.

На этой базе выполнены `go mod tidy -diff`, `go test -race ./...`, `go vet ./...` и `go run golang.org/x/vuln/cmd/govulncheck@v1.4.0 ./...` с `GOWORK=off GOTOOLCHAIN=go1.27.3 GOCACHE=/private/tmp/passkeys-pr-cache`: все команды завершились успешно. Govulncheck не нашёл уязвимых вызываемых функций или импортированных пакетов; две записи в требуемых модулях недостижимы из проверяемого кода. Это локальная проверка PR-кандидата, не статус GitHub Actions.
