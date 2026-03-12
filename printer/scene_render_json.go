package printer

import (
	"encoding/json"
	"os"
	"path/filepath"
)

type SceneJSONRenderer struct{}

func (SceneJSONRenderer) Render(doc *PlateDocument, outPath string) error {
	if err := os.MkdirAll(filepath.Dir(outPath), 0755); err != nil {
		return err
	}
	f, err := os.Create(outPath)
	if err != nil {
		return err
	}
	defer f.Close()
	enc := json.NewEncoder(f)
	enc.SetIndent("", "  ")
	return enc.Encode(doc)
}
