package koanf

import (
	"bytes"
	"fmt"
	"reflect"
	"sort"
)

// LayerRef identifies a single load/merge event (a "layer") in a
// Koanf instance's history, eg: defaults -> file -> env -> flags.
// Index is the 0-based sequence number of the event and Name is the
// optional label attached with the WithLayer() load option.
type LayerRef struct {
	Index int
	Name  string
}

// String returns a human-readable representation of the layer,
// eg: `#2 (env)` or `#2` if the layer was loaded without a name.
func (l LayerRef) String() string {
	if l.Name != "" {
		return fmt.Sprintf("#%d (%s)", l.Index, l.Name)
	}
	return fmt.Sprintf("#%d", l.Index)
}

// Override describes a value that was replaced by a later layer.
type Override struct {
	// Source is the layer that previously set the key.
	Source LayerRef

	// Value is the value the key held before it was overridden.
	Value any
}

// Provenance describes the origin of a single leaf key in the merged
// configuration: which layer produced the current value and, if the
// key was overridden, which layer's value it replaced.
type Provenance struct {
	// Key is the flattened, delimited key path, eg: `parent.child.key`.
	Key string

	// Value is the key's current value in the merged config.
	Value any

	// Source is the layer that produced the current value.
	Source LayerRef

	// Overrode describes the value and layer this key replaced.
	// It is nil if the key was introduced (not overridden) by Source.
	Overrode *Override
}

// String returns a single-line, human-readable representation of the
// provenance entry, eg:
// `server.port -> 8080 (from layer #1 (base.yaml), overrode 80 from layer #0 (defaults))`
func (p Provenance) String() string {
	if p.Overrode != nil {
		return fmt.Sprintf("%s -> %v (from layer %s, overrode %v from layer %s)",
			p.Key, p.Value, p.Source, p.Overrode.Value, p.Overrode.Source)
	}
	return fmt.Sprintf("%s -> %v (from layer %s)", p.Key, p.Value, p.Source)
}

// Provenance returns the provenance of every leaf key in the merged
// configuration, sorted by key. It reflects the actual merged state:
// keys removed by Delete() or by a custom merge function are absent,
// and keys whose values changed are attributed to the layer that
// changed them. It is purely observational and does not affect any
// values returned by Get(), All() etc.
func (ko *Koanf) Provenance() []Provenance {
	ko.mu.RLock()
	out := make([]Provenance, 0, len(ko.provenance))
	for _, p := range ko.provenance {
		if p.Overrode != nil {
			ov := *p.Overrode
			p.Overrode = &ov
		}
		out = append(out, p)
	}
	ko.mu.RUnlock()

	sort.Slice(out, func(i, j int) bool { return out[i].Key < out[j].Key })
	return out
}

// SprintProvenance returns a printable, line-per-key representation
// of the provenance of the merged configuration, sorted by key.
func (ko *Koanf) SprintProvenance() string {
	b := bytes.Buffer{}
	for _, p := range ko.Provenance() {
		b.WriteString(p.String())
		b.WriteString("\n")
	}
	return b.String()
}

// PrintProvenance prints the provenance of the merged configuration,
// one key per line, sorted by key.
func (ko *Koanf) PrintProvenance() {
	fmt.Print(ko.SprintProvenance())
}

// observeLocked records the provenance of the current merged state.
// It diffs the freshly flattened conf map against the previously
// recorded provenance: keys that are new or whose values changed are
// attributed to the layer that just merged, keys that vanished
// (deleted by Delete() or a custom merge function) are dropped, and
// untouched keys retain their original attribution. It never mutates
// the conf map or its values. ko.mu must be held by the caller.
func (ko *Koanf) observeLocked(layer string) {
	src := LayerRef{Index: ko.layerSeq, Name: layer}
	ko.layerSeq++

	next := make(map[string]Provenance, len(ko.confMapFlat))
	for key, val := range ko.confMapFlat {
		if prev, ok := ko.provenance[key]; ok && reflect.DeepEqual(prev.Value, val) {
			// Unchanged key. Retain the original attribution.
			next[key] = prev
			continue
		}

		p := Provenance{Key: key, Value: val, Source: src}
		if prev, ok := ko.provenance[key]; ok {
			p.Overrode = &Override{Source: prev.Source, Value: prev.Value}
		}
		next[key] = p
	}
	ko.provenance = next
}

// pruneProvenanceLocked drops provenance entries for keys that no
// longer exist in the flattened conf map. ko.mu must be held by the
// caller.
func (ko *Koanf) pruneProvenanceLocked() {
	for key := range ko.provenance {
		if _, ok := ko.confMapFlat[key]; !ok {
			delete(ko.provenance, key)
		}
	}
}
