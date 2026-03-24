package logx

import (
	"bytes"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestNewAddsDefaultServiceFields(t *testing.T) {
	t.Parallel()

	var out bytes.Buffer

	logger, err := New("payments-api", WithWriter(&out))
	require.NoError(t, err)

	logger.Info().Msg("service started")
	entry := decodeJSONEntry(t, out.String())

	require.Equal(t, "info", entry["level"])
	require.Equal(t, "service started", entry["message"])
	require.Equal(t, "payments-api", entry["service"])
	require.Contains(t, entry, "time")
	require.Contains(t, entry, "caller")
}

func TestNewAllowsConfiguringFieldsAndToggles(t *testing.T) {
	t.Parallel()

	var out bytes.Buffer

	logger, err := New("users-api",
		WithWriter(&out),
		WithLevelText("debug"),
		WithField("component", "http"),
		WithFields(map[string]any{
			"instance": "a-1",
		}),
		WithoutTimestamp(),
		WithoutCaller(),
	)
	require.NoError(t, err)

	logger.Debug().Msg("request accepted")
	entry := decodeJSONEntry(t, out.String())

	require.Equal(t, "debug", entry["level"])
	require.Equal(t, "users-api", entry["service"])
	require.Equal(t, "http", entry["component"])
	require.Equal(t, "a-1", entry["instance"])
	require.NotContains(t, entry, "time")
	require.NotContains(t, entry, "caller")
}

func TestNewReturnsErrorForInvalidOptions(t *testing.T) {
	t.Parallel()

	_, err := New("billing", WithWriter(nil))
	require.Error(t, err)
	require.True(t, errors.Is(err, errWriterIsNil))

	_, err = New("billing", WithCallerSkipFrameCount(-1))
	require.Error(t, err)
	require.True(t, errors.Is(err, errCallerSkipFrameCountNegative))

	_, err = New("billing", WithServiceFieldName("   "))
	require.Error(t, err)
	require.True(t, errors.Is(err, errServiceFieldNameMustNotBeEmpty))

	_, err = New("billing", WithLevelText("not-a-level"))
	require.Error(t, err)
	require.Contains(t, err.Error(), "parse level")
}

func decodeJSONEntry(t *testing.T, raw string) map[string]any {
	t.Helper()

	var entry map[string]any
	err := json.Unmarshal([]byte(strings.TrimSpace(raw)), &entry)
	require.NoError(t, err)

	return entry
}
