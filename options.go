package koanf

// options contains options to modify the behavior of Koanf.Load.
type options struct {
	merge func(a, b map[string]any) error

	// layer is an optional human-readable name for the load/merge
	// event, recorded in the provenance of every key it introduces
	// or overrides.
	layer string
}

// newOptions creates a new options instance.
func newOptions(opts []Option) *options {
	o := new(options)
	o.apply(opts)
	return o
}

// Option is a generic type used to modify the behavior of Koanf.Load.
type Option func(*options)

// apply the given options.
func (o *options) apply(opts []Option) {
	for _, opt := range opts {
		opt(o)
	}
}

// WithMergeFunc is an option to modify the merge behavior of Koanf.Load.
// If unset, the default merge function is used.
//
// The merge function is expected to merge map src into dest (left to right).
func WithMergeFunc(merge func(src, dest map[string]any) error) Option {
	return func(o *options) {
		o.merge = merge
	}
}

// WithLayer is an option to attach a human-readable name to a
// load/merge event (a "layer", eg: "defaults", "base.yaml", "env",
// "flags"). The name is recorded in the provenance of every key the
// layer introduces or overrides and can be inspected with
// Koanf.Provenance() / Koanf.SprintProvenance(). It is purely
// observational and does not change merge behavior in any way.
func WithLayer(name string) Option {
	return func(o *options) {
		o.layer = name
	}
}
