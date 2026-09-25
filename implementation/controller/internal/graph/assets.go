package graph

import (
	"fmt"
	"os"
	"path/filepath"
)

// AssetLoader reads the semantic assets of the implementation from disk. The
// assets are the ontology, the SPARQL queries, the SHACL constraints and the
// Turtle fixtures under implementation/semantics. Root is the directory that
// contains the semantics folder. The runtime loads the ontology and the
// candidate-protocol query at start up to check that they can be read.
type AssetLoader struct {
	Root string
}

// NewAssetLoader returns a loader rooted at the given directory.
func NewAssetLoader(root string) *AssetLoader {
	return &AssetLoader{Root: root}
}

// read returns the file at rel below Root and wraps a failure with its path.
func (l *AssetLoader) read(rel string) ([]byte, error) {
	path := filepath.Join(l.Root, rel)
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read asset %s: %w", path, err)
	}
	return data, nil
}

// LoadOntology returns the QuantumTrustKG ontology in Turtle.
func (l *AssetLoader) LoadOntology() ([]byte, error) {
	return l.read(filepath.Join("semantics", "ontology", "quantumtrustkg.ttl"))
}

// LoadQuery returns a SPARQL query file by name.
func (l *AssetLoader) LoadQuery(name string) ([]byte, error) {
	return l.read(filepath.Join("semantics", "queries", name))
}

// LoadConstraint returns a SHACL constraint file by name.
func (l *AssetLoader) LoadConstraint(name string) ([]byte, error) {
	return l.read(filepath.Join("semantics", "constraints", name))
}

// LoadFixture returns a Turtle scenario fixture by name.
func (l *AssetLoader) LoadFixture(name string) ([]byte, error) {
	return l.read(filepath.Join("semantics", "fixtures", name))
}
