package xml

import (
	"encoding/xml"
	"fmt"
	"os"
	"path/filepath"
	"sort"
)

type xmlDocument[T any] struct {
	Path string
	Data T
}

func loadXMLDocuments[T any](dir, kind string) ([]xmlDocument[T], error) {
	paths, err := filepath.Glob(filepath.Join(dir, "*.xml"))
	if err != nil {
		return nil, fmt.Errorf("xml: list %s files in %s: %w", kind, dir, err)
	}
	if len(paths) == 0 {
		return nil, fmt.Errorf("xml: no %s files found in %s", kind, dir)
	}
	sort.Strings(paths)

	docs := make([]xmlDocument[T], 0, len(paths))
	for _, path := range paths {
		var doc T
		if err := readXML(path, &doc); err != nil {
			return nil, err
		}
		docs = append(docs, xmlDocument[T]{Path: path, Data: doc})
	}
	return docs, nil
}

func readXML(path string, dst any) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("xml: read %s: %w", path, err)
	}
	if err := xml.Unmarshal(data, dst); err != nil {
		return fmt.Errorf("xml: parse %s: %w", path, err)
	}
	return nil
}
