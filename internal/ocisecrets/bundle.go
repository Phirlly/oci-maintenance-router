package ocisecrets

// Bundle keeps secret material out of ordinary formatting and JSON encoding.
// Explicit Bytes access is for transient transport use, never logs or records.
// This is not a secure-memory or zeroization guarantee.
type Bundle struct {
	version int64
	// fmt can bypass methods in private containing fields. Closure captures
	// are not traversed by its reflection fallback, including invalid verbs.
	content func() []byte
}

func (b Bundle) Version() int64 { return b.version }
func (b Bundle) Bytes() []byte {
	if b.content == nil {
		return nil
	}
	return b.content()
}
func (b Bundle) String() string   { return "secret bundle: [redacted]" }
func (b Bundle) GoString() string { return b.String() }
