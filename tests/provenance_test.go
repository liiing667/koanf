package koanf_test

import (
	"testing"

	"github.com/knadh/koanf/providers/confmap"
	"github.com/knadh/koanf/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// provByKey indexes a provenance slice by key for easy assertions.
func provByKey(prov []koanf.Provenance) map[string]koanf.Provenance {
	out := make(map[string]koanf.Provenance, len(prov))
	for _, p := range prov {
		out[p.Key] = p
	}
	return out
}

func TestProvenanceLayers(t *testing.T) {
	k := koanf.New(".")

	// Layer 0: defaults.
	require.NoError(t, k.Load(confmap.Provider(map[string]any{
		"server.host": "localhost",
		"server.port": 80,
		"log.level":   "info",
	}, "."), nil, koanf.WithLayer("defaults")))

	// Layer 1: file overrides port, adds tls.
	require.NoError(t, k.Load(confmap.Provider(map[string]any{
		"server.port": 8080,
		"server.tls":  true,
	}, "."), nil, koanf.WithLayer("base.yaml")))

	// Layer 2: env overrides log.level.
	require.NoError(t, k.Load(confmap.Provider(map[string]any{
		"log.level": "debug",
	}, "."), nil, koanf.WithLayer("env")))

	prov := provByKey(k.Provenance())
	require.Len(t, prov, 4)

	// Untouched key retains its original layer attribution.
	host := prov["server.host"]
	assert.Equal(t, "localhost", host.Value)
	assert.Equal(t, koanf.LayerRef{Index: 0, Name: "defaults"}, host.Source)
	assert.Nil(t, host.Overrode)

	// Overridden key reports the new layer and what it replaced.
	port := prov["server.port"]
	assert.Equal(t, 8080, port.Value)
	assert.Equal(t, koanf.LayerRef{Index: 1, Name: "base.yaml"}, port.Source)
	require.NotNil(t, port.Overrode)
	assert.Equal(t, 80, port.Overrode.Value)
	assert.Equal(t, koanf.LayerRef{Index: 0, Name: "defaults"}, port.Overrode.Source)

	// Newly introduced key has no override.
	tls := prov["server.tls"]
	assert.Equal(t, true, tls.Value)
	assert.Equal(t, koanf.LayerRef{Index: 1, Name: "base.yaml"}, tls.Source)
	assert.Nil(t, tls.Overrode)

	// Second override points at the immediately previous layer.
	lvl := prov["log.level"]
	assert.Equal(t, "debug", lvl.Value)
	assert.Equal(t, koanf.LayerRef{Index: 2, Name: "env"}, lvl.Source)
	require.NotNil(t, lvl.Overrode)
	assert.Equal(t, "info", lvl.Overrode.Value)
	assert.Equal(t, koanf.LayerRef{Index: 0, Name: "defaults"}, lvl.Overrode.Source)

	// Observation must not change any values.
	assert.Equal(t, 8080, k.Int("server.port"))
	assert.Equal(t, "localhost", k.String("server.host"))
	assert.Equal(t, "debug", k.String("log.level"))
	assert.Equal(t, map[string]any{
		"log.level":   "debug",
		"server.host": "localhost",
		"server.port": 8080,
		"server.tls":  true,
	}, k.All())
}

func TestProvenanceUnnamedAndSameValue(t *testing.T) {
	k := koanf.New(".")

	require.NoError(t, k.Load(confmap.Provider(map[string]any{
		"a.b": 1,
	}, "."), nil))

	// A layer that re-sets the same value must not take over
	// attribution; the value didn't change.
	require.NoError(t, k.Load(confmap.Provider(map[string]any{
		"a.b": 1,
		"a.c": 2,
	}, "."), nil))

	prov := provByKey(k.Provenance())
	assert.Equal(t, koanf.LayerRef{Index: 0}, prov["a.b"].Source)
	assert.Nil(t, prov["a.b"].Overrode)
	assert.Equal(t, koanf.LayerRef{Index: 1}, prov["a.c"].Source)
}

func TestProvenanceDelete(t *testing.T) {
	k := koanf.New(".")

	require.NoError(t, k.Load(confmap.Provider(map[string]any{
		"parent.a": 1,
		"parent.b": 2,
		"other":    3,
	}, "."), nil, koanf.WithLayer("defaults")))

	// Deleted keys must vanish from the observation.
	k.Delete("parent.a")
	prov := provByKey(k.Provenance())
	require.Len(t, prov, 2)
	_, ok := prov["parent.a"]
	assert.False(t, ok)

	// Delete() with no path resets everything, including the layer
	// sequence.
	k.Delete("")
	assert.Empty(t, k.Provenance())

	require.NoError(t, k.Load(confmap.Provider(map[string]any{
		"x": 1,
	}, "."), nil, koanf.WithLayer("fresh")))
	prov = provByKey(k.Provenance())
	assert.Equal(t, koanf.LayerRef{Index: 0, Name: "fresh"}, prov["x"].Source)
}

func TestProvenanceCustomMergeDelete(t *testing.T) {
	k := koanf.New(".")

	require.NoError(t, k.Load(confmap.Provider(map[string]any{
		"keep": 1,
		"drop": 2,
	}, "."), nil, koanf.WithLayer("defaults")))

	// A custom merge function that deletes a key. Provenance must
	// reflect the real merged state, not the replayed inputs.
	require.NoError(t, k.Load(confmap.Provider(map[string]any{
		"keep": 10,
	}, "."), nil, koanf.WithLayer("custom"), koanf.WithMergeFunc(
		func(src, dest map[string]any) error {
			delete(dest, "drop")
			dest["keep"] = src["keep"]
			return nil
		})))

	prov := provByKey(k.Provenance())
	require.Len(t, prov, 1)
	assert.Equal(t, 10, prov["keep"].Value)
	assert.Equal(t, koanf.LayerRef{Index: 1, Name: "custom"}, prov["keep"].Source)
	require.NotNil(t, prov["keep"].Overrode)
	assert.Equal(t, 1, prov["keep"].Overrode.Value)
}

func TestProvenanceSetAndMerge(t *testing.T) {
	k := koanf.New(".")

	require.NoError(t, k.Load(confmap.Provider(map[string]any{
		"a": 1,
	}, "."), nil, koanf.WithLayer("defaults")))

	require.NoError(t, k.Set("a", 2))
	prov := provByKey(k.Provenance())
	assert.Equal(t, 2, prov["a"].Value)
	require.NotNil(t, prov["a"].Overrode)
	assert.Equal(t, 1, prov["a"].Overrode.Value)
	assert.Equal(t, koanf.LayerRef{Index: 0, Name: "defaults"}, prov["a"].Overrode.Source)

	other := koanf.New(".")
	require.NoError(t, other.Load(confmap.Provider(map[string]any{
		"b": 3,
	}, "."), nil))
	require.NoError(t, k.Merge(other))
	prov = provByKey(k.Provenance())
	assert.Equal(t, 3, prov["b"].Value)
}

func TestProvenanceStrictMergeUnaffected(t *testing.T) {
	k := koanf.NewWithConf(koanf.Conf{Delim: ".", StrictMerge: true})

	require.NoError(t, k.Load(confmap.Provider(map[string]any{
		"a": 1,
	}, "."), nil, koanf.WithLayer("defaults")))

	// StrictMerge still errors on type mismatches and the failed
	// merge leaves provenance untouched.
	err := k.Load(confmap.Provider(map[string]any{
		"a": "string",
	}, "."), nil)
	require.Error(t, err)

	prov := provByKey(k.Provenance())
	require.Len(t, prov, 1)
	assert.Equal(t, 1, prov["a"].Value)
	assert.Equal(t, koanf.LayerRef{Index: 0, Name: "defaults"}, prov["a"].Source)
	assert.Equal(t, 1, k.Int("a"))
}

func TestSprintProvenance(t *testing.T) {
	k := koanf.New(".")

	require.NoError(t, k.Load(confmap.Provider(map[string]any{
		"server.port": 80,
	}, "."), nil, koanf.WithLayer("defaults")))
	require.NoError(t, k.Load(confmap.Provider(map[string]any{
		"server.port": 8080,
		"server.host": "example.com",
	}, "."), nil, koanf.WithLayer("base.yaml")))

	out := k.SprintProvenance()
	assert.Contains(t, out, "server.host -> example.com (from layer #1 (base.yaml))")
	assert.Contains(t, out, "server.port -> 8080 (from layer #1 (base.yaml), overrode 80 from layer #0 (defaults))")

	// Lines are sorted by key.
	assert.Less(t,
		indexOf(t, out, "server.host"),
		indexOf(t, out, "server.port"),
	)
}

func indexOf(t *testing.T, s, sub string) int {
	t.Helper()
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}
	t.Fatalf("%q not found in %q", sub, s)
	return -1
}

func TestProvenanceConcurrentReads(t *testing.T) {
	k := koanf.New(".")
	require.NoError(t, k.Load(confmap.Provider(map[string]any{
		"a": 1,
	}, "."), nil))

	done := make(chan struct{})
	go func() {
		for i := 0; i < 100; i++ {
			_ = k.Set("a", i)
		}
		close(done)
	}()
	for i := 0; i < 100; i++ {
		_ = k.Provenance()
		_ = k.SprintProvenance()
	}
	<-done

	// Provenance must always agree with the final merged state.
	prov := provByKey(k.Provenance())
	assert.Equal(t, k.Get("a"), prov["a"].Value)
}
