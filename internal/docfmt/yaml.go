package docfmt

// yaml.go - the repository's ONE YAML writer. See CONTRIBUTE.md, "Encoders are
// 2-space": the two tests in encoders_test.go beside it fail on a second call site.

import (
	"bytes"

	"go.yaml.in/yaml/v4"
)

// yamlIndent is the indentation every encoder here writes, YAML and JSON alike.
// encoders_test.go accepts the literal 2 or this identifier at a SetIndent call.
const yamlIndent = 2

// MarshalYAML encodes v in this repository's shape, which yaml.Marshal cannot:
// CompactSeqIndent makes "- " count as indentation, so the dash sits flush.
func MarshalYAML(v any) ([]byte, error) { return marshalYAML(v) }

// marshalYAML is the single yaml.NewEncoder call site in the repository, and
// encoders_test.go identifies it BY NAME, so renaming it fails that test.
func marshalYAML(v any) ([]byte, error) {
	var buf bytes.Buffer
	enc := yaml.NewEncoder(&buf)
	enc.CompactSeqIndent()
	enc.SetIndent(yamlIndent)
	if err := enc.Encode(v); err != nil {
		// Close is still called, because the encoder holds a libyaml emitter; the
		// encode error is the one worth reporting.
		_ = enc.Close()
		return nil, err
	}
	if err := enc.Close(); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}
