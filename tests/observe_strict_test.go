package koanf_test

import (
	"sync"
	"testing"

	"github.com/knadh/koanf/providers/confmap"
	"github.com/knadh/koanf/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestProvenanceLayers verifies that the provenance of every leaf key
// (which layer produced the current value, and what it overrode) is
// tracked across multiple loaded layers.
func TestProvenanceLayers(t *testing.T) {
	k := koanf.New(".")

	require.NoError(t, k.Load(confmap.Provider(map[string]any{
		"server.host": "localhost",
		"server.port": 80,
		"db.user":     "root",
	}, "."), nil, koanf.WithSource("defaults")))

	require.NoError(t, k.Load(confmap.Provider(map[string]any{
		"server.port": 8080,
		"db.user":     "app",
	}, "."), nil, koanf.WithSource("env")))

	// Overridden key: current value comes from the later layer and the
	// overridden value is recorded in the history.
	p, ok := k.ProvenanceOf("server.port")
	require.True(t, ok)
	assert.False(t, p.Deleted)
	assert.Equal(t, 8080, p.Value)
	assert.Equal(t, "env", p.Source)
	require.Len(t, p.History, 1)
	assert.Equal(t, koanf.Origin{Source: "defaults", Value: 80}, p.History[0])

	// Untouched key: no history.
	p, ok = k.ProvenanceOf("server.host")
	require.True(t, ok)
	assert.Equal(t, "localhost", p.Value)
	assert.Equal(t, "defaults", p.Source)
	assert.Empty(t, p.History)

	// The observed provenance matches the real merged state.
	all := k.All()
	for _, prov := range k.Provenance() {
		if prov.Deleted {
			continue
		}
		assert.Equal(t, all[prov.Key], prov.Value, "provenance value mismatch for %s", prov.Key)
	}

	// Printable line-per-key output.
	s := k.SprintProvenance()
	assert.Contains(t, s, "server.port -> 8080 (from env; overrides 80 from defaults)")
	assert.Contains(t, s, "server.host -> localhost (from defaults)")
	assert.Contains(t, s, "db.user -> app (from env; overrides root from defaults)")

	// Unknown key.
	_, ok = k.ProvenanceOf("nope")
	assert.False(t, ok)
}

// TestProvenanceDefaultSourceNames verifies that unnamed layers get
// sequential load#N names.
func TestProvenanceDefaultSourceNames(t *testing.T) {
	k := koanf.New(".")
	require.NoError(t, k.Load(confmap.Provider(map[string]any{"a": 1}, "."), nil))
	require.NoError(t, k.Load(confmap.Provider(map[string]any{"a": 2}, "."), nil))

	p, ok := k.ProvenanceOf("a")
	require.True(t, ok)
	assert.Equal(t, "load#1", p.Source)
	require.Len(t, p.History, 1)
	assert.Equal(t, "load#0", p.History[0].Source)
	assert.Equal(t, 1, p.History[0].Value)
}

// TestProvenanceSet verifies that Set() is recorded as its own layer.
func TestProvenanceSet(t *testing.T) {
	k := koanf.New(".")
	require.NoError(t, k.Load(confmap.Provider(map[string]any{"a": 1}, "."), nil, koanf.WithSource("defaults")))
	require.NoError(t, k.Set("a", 42))

	p, ok := k.ProvenanceOf("a")
	require.True(t, ok)
	assert.Equal(t, "set", p.Source)
	assert.Equal(t, 42, p.Value)
	require.Len(t, p.History, 1)
	assert.Equal(t, koanf.Origin{Source: "defaults", Value: 1}, p.History[0])
}

// TestProvenanceDeleted verifies that keys removed from the merged
// config, either explicitly or by a parent being replaced, are reported
// as deleted with their last known value and source.
func TestProvenanceDeleted(t *testing.T) {
	k := koanf.New(".")
	require.NoError(t, k.Load(confmap.Provider(map[string]any{
		"a.b": 1,
		"a.c": 2,
		"x":   3,
	}, "."), nil, koanf.WithSource("defaults")))

	// Explicit delete.
	k.Delete("a.b")
	p, ok := k.ProvenanceOf("a.b")
	require.True(t, ok)
	assert.True(t, p.Deleted)
	assert.Equal(t, 1, p.Value)
	assert.Equal(t, "defaults", p.Source)
	assert.False(t, k.Exists("a.b"))
	assert.Contains(t, k.SprintProvenance(), "a.b -> <deleted> (last value 1 from defaults)")

	// Untouched sibling is unaffected.
	p, ok = k.ProvenanceOf("a.c")
	require.True(t, ok)
	assert.False(t, p.Deleted)

	// Delete everything.
	k.Delete("")
	p, ok = k.ProvenanceOf("x")
	require.True(t, ok)
	assert.True(t, p.Deleted)

	// A later layer replacing a parent map with a scalar wipes the
	// nested keys; provenance must reflect the real merged state.
	k2 := koanf.New(".")
	require.NoError(t, k2.Load(confmap.Provider(map[string]any{
		"a.b": 1,
		"a.c": 2,
	}, "."), nil, koanf.WithSource("defaults")))
	require.NoError(t, k2.Load(confmap.Provider(map[string]any{
		"a": "scalar",
	}, "."), nil, koanf.WithSource("override")))

	p, ok = k2.ProvenanceOf("a.b")
	require.True(t, ok)
	assert.True(t, p.Deleted)
	p, ok = k2.ProvenanceOf("a.c")
	require.True(t, ok)
	assert.True(t, p.Deleted)

	p, ok = k2.ProvenanceOf("a")
	require.True(t, ok)
	assert.False(t, p.Deleted)
	assert.Equal(t, "scalar", p.Value)
	assert.Equal(t, "override", p.Source)
	assert.Equal(t, "scalar", k2.Get("a"))
}

// TestProvenanceConcurrent exercises provenance reads and writes
// concurrently to catch data races under `go test -race`.
func TestProvenanceConcurrent(t *testing.T) {
	k := koanf.New(".")
	require.NoError(t, k.Load(confmap.Provider(map[string]any{"key": 0}, "."), nil))

	var wg sync.WaitGroup
	for i := 1; i <= 8; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			_ = k.Load(confmap.Provider(map[string]any{"key": i}, "."), nil)
			_ = k.Provenance()
			_, _ = k.ProvenanceOf("key")
			_ = k.SprintProvenance()
			_ = k.Get("key")
		}(i)
	}
	wg.Wait()

	p, ok := k.ProvenanceOf("key")
	require.True(t, ok)
	assert.False(t, p.Deleted)
	assert.Equal(t, k.Get("key"), p.Value)
}

type strictServerConf struct {
	Host string   `koanf:"host"`
	Port int      `koanf:"port"`
	Tags []string `koanf:"tags"`
}

type strictAppConf struct {
	Server strictServerConf `koanf:"server"`
}

// TestStrictUnmarshalUnknownKey verifies that strict decoding errors on
// config keys that have no struct field, naming the full path, while the
// default mode silently ignores them.
func TestStrictUnmarshalUnknownKey(t *testing.T) {
	k := koanf.New(".")
	require.NoError(t, k.Load(confmap.Provider(map[string]any{
		"server.host":  "localhost",
		"server.port":  8080,
		"server.bogus": "nope",
	}, "."), nil))

	// Default: unknown keys are silently ignored.
	var out strictAppConf
	require.NoError(t, k.Unmarshal("", &out))
	assert.Equal(t, 8080, out.Server.Port)

	// Strict: error names the full path of the unknown key.
	out = strictAppConf{}
	err := k.UnmarshalWithConf("", &out, koanf.UnmarshalConf{Strict: true})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "server")
	assert.Contains(t, err.Error(), "bogus")
}

// TestStrictUnmarshalTypeMismatch verifies that strict decoding errors
// on type mismatches with the full path and the expected type, while the
// default weakly typed mode converts silently.
func TestStrictUnmarshalTypeMismatch(t *testing.T) {
	k := koanf.New(".")
	require.NoError(t, k.Load(confmap.Provider(map[string]any{
		"server.host": "localhost",
		"server.port": "8080", // string where an int is expected
	}, "."), nil))

	// Default: weak typing converts the string to an int.
	var out strictAppConf
	require.NoError(t, k.Unmarshal("", &out))
	assert.Equal(t, 8080, out.Server.Port)

	// Strict: error carries the full path and the expected type.
	out = strictAppConf{}
	err := k.UnmarshalWithConf("", &out, koanf.UnmarshalConf{Strict: true})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "server.port")
	assert.Contains(t, err.Error(), "int")
}

// TestStrictUnmarshalScalarToSlice verifies that a scalar given where an
// array is expected errors with the full path in strict mode.
func TestStrictUnmarshalScalarToSlice(t *testing.T) {
	k := koanf.New(".")
	require.NoError(t, k.Load(confmap.Provider(map[string]any{
		"server.host": "localhost",
		"server.port": 8080,
		"server.tags": "just-a-string", // scalar where a slice is expected
	}, "."), nil))

	// Default: weak typing wraps the scalar into a slice.
	var out strictAppConf
	require.NoError(t, k.Unmarshal("", &out))
	assert.Equal(t, []string{"just-a-string"}, out.Server.Tags)

	// Strict: error carries the full path.
	out = strictAppConf{}
	err := k.UnmarshalWithConf("", &out, koanf.UnmarshalConf{Strict: true})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "server.tags")
}

// TestStrictUnmarshalValid verifies that strict decoding succeeds on a
// config that exactly matches the struct, and that strict mode does not
// affect All()/Get().
func TestStrictUnmarshalValid(t *testing.T) {
	k := koanf.New(".")
	require.NoError(t, k.Load(confmap.Provider(map[string]any{
		"server.host": "localhost",
		"server.port": 8080,
		"server.tags": []string{"a", "b"},
	}, "."), nil))

	var out strictAppConf
	require.NoError(t, k.UnmarshalWithConf("", &out, koanf.UnmarshalConf{Strict: true}))
	assert.Equal(t, "localhost", out.Server.Host)
	assert.Equal(t, 8080, out.Server.Port)
	assert.Equal(t, []string{"a", "b"}, out.Server.Tags)

	// Getters are unaffected by strict decoding.
	assert.Equal(t, 8080, k.Get("server.port"))
	assert.Equal(t, "localhost", k.All()["server.host"])
}

// TestStrictUnmarshalDoesNotMutateConfig verifies that a failed strict
// decode leaves the loaded configuration untouched.
func TestStrictUnmarshalDoesNotMutateConfig(t *testing.T) {
	k := koanf.New(".")
	require.NoError(t, k.Load(confmap.Provider(map[string]any{
		"server.port": "8080",
	}, "."), nil))

	before := k.All()
	var out strictAppConf
	require.Error(t, k.UnmarshalWithConf("", &out, koanf.UnmarshalConf{Strict: true}))
	assert.Equal(t, before, k.All())
	assert.Equal(t, "8080", k.Get("server.port"))
}
