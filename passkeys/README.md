# passkeys

`passkeys` формирует options и проверяет ответы WebAuthn: регистрацию ключа, вход известного пользователя и вход без имени пользователя. Пакет находится в модуле `github.com/dkoshenkov/packages-go`, версия Go для сборки всего модуля задаётся корневым `go.mod` (1.27.3 в текущем main). Ядро отдельно проверялось на Go 1.25.7 в исходной локальной копии с `go 1.25.6`.

Статус: **experimental**, реализация от 2026-10-31, ещё не опубликована отдельным релизом. Перед production нужны независимое security review и проверки целевых платформ. Результаты текущих проверок — в [матрице приёмки](acceptance.md).

Протокол реализован внутри пакета. Криптография использует стандартную библиотеку Go, CBOR — `github.com/fxamacker/cbor/v2 v2.10.0`; проверка public suffix использует уже имеющийся `golang.org/x/net/publicsuffix`. Сетевых запросов, встроенного логирования и глобального изменяемого состояния в ядре нет.

## Подключение

```go
import "github.com/dkoshenkov/packages-go/passkeys"

rp, err := passkeys.New(passkeys.Config{
    RPID:    "example.com",
    RPName:  "Example",
    Origins: []string{"https://example.com"},
})
if err != nil {
    return err
}
```

Внутри packages-go достаточно импорта. В другом локальном модуле до публикации можно добавить `replace github.com/dkoshenkov/packages-go => /absolute/path/to/packages-go` и требование модуля. Версия зависимости для опубликованной установки появится после отдельного релиза.

`New` проверяет конфигурацию и копирует origins. Один `RP` можно использовать параллельно. По умолчанию операция действует пять минут; `Config.Timeout` меняет срок. `Config.Now` позволяет подставить часы в тестах, по умолчанию используется `time.Now`. Переданная функция часов должна быть безопасна для параллельных вызовов.

RP ID задаётся ASCII-доменом в нижнем регистре, без схемы, порта, завершающей точки и wildcard. Для Unicode-домена передайте его ASCII/punycode-форму. Public suffix, включая частные suffix вроде `github.io`, запрещён. Origins — точные сериализованные HTTPS origins без пути, query, fragment, userinfo и явного стандартного порта. Host должен совпадать с RP ID либо быть его поддоменом. Разрешён нестандартный порт. Не выводите origins из входящих HTTP-заголовков.

Для разработки: `RPID: "localhost"`, `Origins: []string{"http://localhost:8080"}`, `AllowLocalhost: true`. IP-адреса и HTTP на других доменах не поддерживаются.

## Регистрация

```go
options, state, err := rp.BeginRegistration(passkeys.RegistrationRequest{
    FlowID: flowID,
    User: passkeys.User{
        ID:          userHandle,
        Name:        "alice",
        DisplayName: "Alice",
    },
    Exclude: existingKeys,
})
```

`userHandle` — стабильные случайные 1–64 байта, принадлежащие аккаунту. Имя и отображаемое имя нужны браузерному диалогу; email и телефон не требуются. `existingKeys` — `[]passkeys.CredentialDescriptor`, допускается `nil`.

Сервис сохраняет `state` на сервере и связывает поток с браузером. Клиент получает **только `options`**. Options сериализуются через `encoding/json`; бинарные браузерные поля уже представлены строками base64url. Их можно передать в `PublicKeyCredential.parseCreationOptionsFromJSON` или преобразовать в `Uint8Array`, как делает [браузерный пример](example/index.html).

После ответа браузера:

```go
credential, err := rp.FinishRegistration(state, flowID, responseBody)
```

`responseBody` — исходный JSON credential с `id`, `rawId`, `type`, `response.clientDataJSON` и `response.attestationObject`. Поддерживается представление `PublicKeyCredential.toJSON()`; пример содержит преобразование для браузеров без этого метода.

До сохранения сервис повторно проверяет право добавить ключ и актуальность аккаунта. Credential ID должен быть уникален среди всех аккаунтов: `excludeCredentials` не заменяет ограничение уникальности в БД. Создание аккаунта и добавление ключа завершаются вместе с расходованием state одной транзакцией.

## Вход

Для discoverable login:

```go
options, state, err := rp.BeginLogin(passkeys.LoginRequest{FlowID: flowID})
```

Для известного пользователя задайте оба поля `UserID` и `Allow`:

```go
options, state, err := rp.BeginLogin(passkeys.LoginRequest{
    FlowID: flowID,
    UserID: userHandle,
    Allow:  []passkeys.CredentialDescriptor{{ID: credentialID}},
})
```

Один `UserID` без ключей или список ключей без пользователя отклоняется. Состояние сохраняет ожидаемого владельца и allow list.

Порядок завершения:

```go
id, err := passkeys.CredentialID(responseBody)
// Обработать err. id — недоверенная подсказка для поиска в БД.
// Загрузить credential и версии credential/account из доверенного хранилища.

result, err := rp.FinishLogin(state, flowID, credential, responseBody)
// Обработать err. Проверить CounterAnomaly согласно политике сервиса.
// Атомарно расходовать state, сохранить metadata и создать сессию.
```

`CredentialID` не проверяет подпись и не даёт права доступа. `FinishLogin` повторно проверяет ID, владельца и ограничения операции. Для discoverable login `userHandle` обязателен и должен совпадать с владельцем credential; для known-user login он может отсутствовать или быть `null`, но при наличии обязан совпасть. Неизвестный handle не создаёт аккаунт.

`LoginResult` содержит `CredentialID`, `UserID`, `SignCount`, `BackupState`, `CounterAnomaly`. Исходные credential/state не изменяются, при ошибке результат нулевой. Повторный вызов с теми же аргументами может успешно пройти криптографическую проверку — одноразовость обеспечивается транзакцией сервиса.

## Хранение и транзакции

`State` и `Credential` допускают JSON round trip. Храните их как доверенные серверные данные. Не принимайте state из тела клиентского запроса. `FlowID` должен быть непустым идентификатором защищённого браузерного потока; его значение отдельно сверяется в Finish. В примере это случайный 256-битный идентификатор в HttpOnly/SameSite cookie.

State привязан к RP ID, набору origins, TTL и фиксированной политике пакета. После изменения этих параметров старые операции отклоняются. При `now == ExpiresAt` операция уже истекла. Browser timeout только подсказывает длительность интерфейсу.

Одна транзакция должна:

1. Проверить актуальность аккаунта, credential, их версий и pending state после криптографической проверки.
2. Проверить привязку к браузеру и срок pending state на момент commit.
3. Применить политику counter anomaly.
4. Атомарно расходовать challenge, сохранить credential/metadata и создать разрешённый результат входа.
5. Выдать session cookie только после успешного commit.

Восстановление аккаунта увеличивает его generation; отзыв/изменение ключа меняет версию или удаляет запись. Все операции должны соблюдать единый порядок блокировок. [transaction.sql](example/transaction.sql) показывает контракт для PostgreSQL. Это SQL-эскиз для адаптации к схеме сервиса; он не был выполнен на реальной БД.

[Пример сервера](example/main.go) реализует эту последовательность через общий mutex и память **одного процесса**. Он предназначен для локального знакомства и тестирования; память процесса не является хранилищем для нескольких реплик, данные теряются при перезапуске.

## Профиль и ошибки

Нормативная основа: [WebAuthn Level 3, 2026-08-25](https://www.w3.org/TR/2026/REC-webauthn-3-20260825/).

- Ключ: только EC2/P-256/ES256, полные 32-байтные координаты X/Y. Приватные и дополнительные COSE-параметры запрещены.
- Attestation: запрашивается и принимается только `none`, `attStmt` должен быть пустой картой. Это не доказывает производителя или модель аутентификатора.
- Регистрация: `residentKey: required`, `requireResidentKey: true`, `userVerification: required`. UP/UV проверяются у обоих типов ответа. Фактическое хранение discoverable credential не доказывается форматом `none`.
- Cross-origin, related origins, AppID и другие алгоритмы/форматы не поддерживаются. Отсутствие `crossOrigin` равно false; true отклоняется. При наличии `topOrigin` он должен равняться `origin`.
- Подпись ES256 проверяется в ASN.1 DER над исходными authenticatorData и SHA-256 исходного clientDataJSON. Повторной сериализации подписанных данных нет.
- BE не меняется после регистрации, BS может меняться; BS без BE запрещён. AAGUID и transports не доказывают физическую идентичность устройства.
- Если хотя бы один счётчик ненулевой и новый не больше сохранённого, возвращается `CounterAnomaly: true`. Это сигнал сервису, а не автоматическое доказательство клонирования. Пример отклоняет такие входы; другая политика требует решения сервиса.

```go
switch {
case errors.Is(err, passkeys.ErrExpired):
    // Начать новую операцию.
case errors.Is(err, passkeys.ErrUnsupported):
    // Ответ не соответствует поддерживаемому профилю.
case err != nil:
    // Операция не подтверждена.
}
```

Остальные категории: `ErrInvalidInput`, `ErrContext`, `ErrCredential`, `ErrPolicy`, `ErrConfig`. Ошибка CSPRNG также приводит к нулевым options/state и обычной ошибке, без fallback. Тексты ошибок не включают входные payloads. Для конечного пользователя обычно достаточно общей ошибки и предложения повторить операцию.

## Разбор и лимиты

| Данные | Предел |
|---|---:|
| Исходный ответ JSON | 64 KiB |
| Декодированный clientDataJSON | 8 KiB |
| authenticatorData / сохранённый COSE key | 16 KiB |
| Credential ID | 1–1023 байта |
| User handle | 1–64 байта |
| JSON-вложенность | 16 переходов от корня |
| CBOR-вложенность | 16 контейнеров |
| Элементы CBOR-массива / пары карты | 256 на контейнер |
| Credentials в allow/exclude | 64 |
| Имя, displayName, flow ID, строка transport | 1–1024 байта UTF-8 |
| Transports | 16 |

Сервис ограничивает размер HTTP body **до** чтения в `[]byte`; пакет проверяет длину уже полученных байтов. Пример использует `http.MaxBytesReader`.

JSON: дубли ключей отклоняются рекурсивно, в том числе в игнорируемых полях; имена протокольных полей чувствительны к регистру. Неизвестные optional поля допускаются в пределах лимитов, но не дают прав. Неизвестные extension outputs разбираются; они не используются как доказательство. `clientExtensionResults` должен быть объектом.

CBOR: запрещены дубли, NaN в ключах и значениях, tags, indefinite lengths, неверный UTF-8, неверные типы и trailing bytes. Запрет NaN действует рекурсивно, в том числе в неизвестных extension outputs. Неизвестные outputs расширений должны быть CBOR-картой с текстовыми ключами. При ED=false хвост запрещён; при ED=true обязательна ровно одна карта расширений. AT обязателен при регистрации и запрещён для assertion этого профиля. RFU-биты игнорируются по §6.1.

Декодер допускает различный порядок ключей и неминимальное кодирование чисел/длин в definite CBOR. Это осознанное отклонение от рекомендации SHOULD по отбрасыванию неканонического CTAP2 CBOR в §2.4: неоднозначные дубли всё равно запрещены, а исходные подписанные байты сохраняются. Требования к обязательным полям и типам не ослабляются.

## Запуск примера и проверок

Из корня packages-go:

```sh
go run ./passkeys/example
# Открыть http://localhost:8080

go test ./...
go test -race ./passkeys/...
go vet ./...
```

Каждый fuzz target можно запускать отдельно; выполненный локальный бюджет — по 10 секунд, два worker:

```sh
go test ./passkeys -run '^$' -fuzz '^FuzzJSON$' -fuzztime=10s -parallel=2
go test ./passkeys -run '^$' -fuzz '^FuzzCBOR$' -fuzztime=10s -parallel=2
go test ./passkeys -run '^$' -fuzz '^FuzzAuthenticatorData$' -fuzztime=10s -parallel=2
go test ./passkeys -run '^$' -fuzz '^FuzzFinish$' -fuzztime=10s -fuzzminimizetime=100ms -parallel=2
```

Для короткого smoke-прогона `FuzzFinish` ограничение `-fuzzminimizetime=100ms` оставляет время на новые мутации; стандартный бюджет минимизации может занять весь короткий прогон.

Семена включены в Go-тесты. `FuzzFinish` использует одинаковые фиксированные challenge и credential во всех workers и при повторных запусках; перед fuzzing каждый процесс проверяет, что оба seed проходят Finish. Фиксированный независимый вектор — [testdata/openssl-es256.json](testdata/openssl-es256.json). Его сформировал Python со своим небольшим CBOR encoder и OpenSSL; код `passkeys` для построения/подписи не использовался. [generate.py](testdata/generate.py) запускается из корня модуля и **заменяет** fixture новым случайным ключом; обычные тесты ничего не генерируют и не требуют OpenSSL.

Для автоматической проверки браузера установите `puppeteer-core` отдельно от Go-модуля, укажите Chromium и соберите demo:

```sh
go build -o /tmp/passkeys-demo ./passkeys/example
PASSKEYS_DEMO_BINARY=/tmp/passkeys-demo \
PUPPETEER_MODULE=/absolute/path/to/puppeteer-core \
CHROME_PATH=/absolute/path/to/chromium \
node passkeys/example/browser-test.cjs
```

Скрипт сам запускает и останавливает demo и headless-браузер. Порт 8080 должен быть свободен.

Проверка импорта из go-webauthn v0.18.2 использует отдельный временный модуль и Go 1.27.2; она не повышает минимальную версию Go основного пакета:

```sh
packages_dir="$PWD"
interop_dir="$(mktemp -d)"
cp passkeys/testdata/interop.go "$interop_dir/main.go"
cd "$interop_dir"
go mod init passkeys-interop
go mod edit -replace "github.com/dkoshenkov/packages-go=$packages_dir"
go get github.com/dkoshenkov/packages-go@v0.0.0 github.com/go-webauthn/webauthn@v0.18.2
go run . "$packages_dir/passkeys/testdata/openssl-es256.json"
```

Проверяются регистрация/assertion обеими реализациями, JSON round trip `go-webauthn.Credential`, импорт COSE-ключа с явным владельцем и отказ неподдерживаемому algorithm. Это проверка совместимости формата; production-данные auth-go не читались и не мигрировались.

## Изменения и миграция

До стабилизации изменения публичного API согласуются владельцем. Расширение алгоритмов, attestation или origins требует изменения профиля и новых проверок. Обновление CBOR сопровождается unit/race/fuzz и interoperability-прогоном. State содержит идентификатор политики: изменение политики должно инвалидировать старые операции.

Для будущей миграции auth-go: снять формат хранения и проверить supported subset на копии данных; сопоставить владельцев и metadata; проверить вход до переключения; включать новую проверку контролируемо; сохранить прежние записи и возможность отката. Автоматического fallback на другую проверку при отказе `passkeys` быть не должно. Переключение auth-go — отдельное согласованное изменение.
