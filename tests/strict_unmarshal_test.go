package koanf_test

import (
	"testing"
	"time"

	"github.com/knadh/koanf/providers/confmap"
	"github.com/knadh/koanf/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type strictInner struct {
	Name string `koanf:"name"`
}

type strictConf struct {
	Host    string            `koanf:"host"`
	Port    int               `koanf:"port"`
	Tags    []string          `koanf:"tags"`
	Timeout time.Duration     `koanf:"timeout"`
	Inner   strictInner       `koanf:"inner"`
	Labels  map[string]string `koanf:"labels"`
}

func loadStrictFixture(t *testing.T, mp map[string]any) *koanf.Koanf {
	t.Helper()
	k := koanf.New(".")
	require.NoError(t, k.Load(confmap.Provider(mp, "."), nil))
	return k
}

func TestStrictUnmarshalUnknownKey(t *testing.T) {
	k := loadStrictFixture(t, map[string]any{
		"host":        "localhost",
		"port":        8080,
		"inner.name":  "app",
		"inner.bogus": "nope",
		"mystery":     1,
	})

	var c strictConf
	err := k.UnmarshalWithConf("", &c, koanf.UnmarshalConf{Strict: true})
	require.Error(t, err)
	// The error must name the full paths of the unknown keys.
	assert.Contains(t, err.Error(), "mystery")
	assert.Contains(t, err.Error(), "bogus")
	assert.Contains(t, err.Error(), "inner")
}

func TestStrictUnmarshalTypeMismatch(t *testing.T) {
	k := loadStrictFixture(t, map[string]any{
		"host": "localhost",
		// A string where a number is expected.
		"port": "not-a-number",
		// A scalar where a slice is expected.
		"tags": "single",
	})

	var c strictConf
	err := k.UnmarshalWithConf("", &c, koanf.UnmarshalConf{Strict: true})
	require.Error(t, err)
	// Errors must carry the full key path and the expected type.
	assert.Contains(t, err.Error(), "'port'")
	assert.Contains(t, err.Error(), "expected type 'int'")
	assert.Contains(t, err.Error(), "'tags'")
	assert.Contains(t, err.Error(), "string")
}

func TestStrictUnmarshalNestedPath(t *testing.T) {
	k := loadStrictFixture(t, map[string]any{
		"inner.name": 123,
	})

	var c strictConf
	err := k.UnmarshalWithConf("", &c, koanf.UnmarshalConf{Strict: true})
	require.Error(t, err)
	// The nested key path must appear in the error.
	assert.Contains(t, err.Error(), "'inner.name'")
	assert.Contains(t, err.Error(), "expected type 'string'")
}

func TestStrictUnmarshalValid(t *testing.T) {
	k := loadStrictFixture(t, map[string]any{
		"host":        "localhost",
		"port":        8080,
		"tags":        []any{"a", "b"},
		"timeout":     "5s",
		"inner.name":  "app",
		"labels.env":  "prod",
		"labels.team": "core",
	})

	var c strictConf
	require.NoError(t, k.UnmarshalWithConf("", &c, koanf.UnmarshalConf{Strict: true}))
	assert.Equal(t, "localhost", c.Host)
	assert.Equal(t, 8080, c.Port)
	assert.Equal(t, []string{"a", "b"}, c.Tags)
	// The duration decode hook still applies in strict mode.
	assert.Equal(t, 5*time.Second, c.Timeout)
	assert.Equal(t, "app", c.Inner.Name)
	assert.Equal(t, map[string]string{"env": "prod", "team": "core"}, c.Labels)
}

func TestStrictUnmarshalDefaultOff(t *testing.T) {
	k := loadStrictFixture(t, map[string]any{
		"host":    "localhost",
		"port":    "8080", // weakly typed string -> int
		"mystery": 1,      // unknown key, silently ignored
	})

	// Default (non-strict) behavior is unchanged: weak conversions
	// happen and unknown keys are ignored.
	var c strictConf
	require.NoError(t, k.Unmarshal("", &c))
	assert.Equal(t, 8080, c.Port)

	var c2 strictConf
	require.NoError(t, k.UnmarshalWithConf("", &c2, koanf.UnmarshalConf{}))
	assert.Equal(t, 8080, c2.Port)
}

func TestStrictUnmarshalDoesNotAffectGetters(t *testing.T) {
	k := loadStrictFixture(t, map[string]any{
		"host":    "localhost",
		"port":    "not-a-number",
		"mystery": 1,
	})

	var c strictConf
	require.Error(t, k.UnmarshalWithConf("", &c, koanf.UnmarshalConf{Strict: true}))

	// A failed strict decode must not alter the underlying config;
	// getters keep returning the raw values.
	assert.Equal(t, "not-a-number", k.String("port"))
	assert.Equal(t, 1, k.Int("mystery"))
	assert.Equal(t, map[string]any{
		"host":    "localhost",
		"port":    "not-a-number",
		"mystery": 1,
	}, k.All())
}

func TestStrictUnmarshalSubPath(t *testing.T) {
	k := loadStrictFixture(t, map[string]any{
		"inner.name":  "app",
		"inner.extra": true,
	})

	var in strictInner
	err := k.UnmarshalWithConf("inner", &in, koanf.UnmarshalConf{Strict: true})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "extra")

	// Without the extra key, unmarshalling a sub-path works.
	k.Delete("inner.extra")
	require.NoError(t, k.UnmarshalWithConf("inner", &in, koanf.UnmarshalConf{Strict: true}))
	assert.Equal(t, "app", in.Name)
}
