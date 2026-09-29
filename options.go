package koanf

// options contains options to modify the behavior of Koanf.Load.
type options struct {
	merge func(a, b map[string]any) error

	// source is an optional name identifying the config layer being
	// loaded. It is recorded in the provenance information.
	source string
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

// WithSource is an option to name the config layer being loaded.
// The name is recorded in the provenance information (see
// Koanf.Provenance) to identify which layer a value came from.
// If unset, layers are named load#0, load#1, ... in load order.
// This is purely observational and does not change merge behavior.
func WithSource(name string) Option {
	return func(o *options) {
		o.source = name
	}
}
