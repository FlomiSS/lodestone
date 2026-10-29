package node

import (
	"bytes"
	"errors"
	"fmt"
	"io/fs"
	"math"
	"strings"

	"github.com/pelletier/go-toml/v2"
	"gopkg.in/yaml.v3"
)

// SetOp setzt einen Wert an einem Pfad (bei Properties genau ein Schlüssel).
type SetOp struct {
	Path  []string `json:"path"`
	Value any      `json:"value"`
}

// Edit ändert eine Konfigurationsdatei des Servers.
type Edit struct {
	File   string     `json:"file"`
	Format string     `json:"format"` // properties, yaml, toml
	Create bool       `json:"create,omitempty"`
	Set    []SetOp    `json:"set,omitempty"`
	Delete [][]string `json:"delete,omitempty"`
}

// FileWrite legt eine Datei mit festem Inhalt an oder ersetzt sie.
type FileWrite struct {
	Path    string `json:"path"`
	Content string `json:"content"`
}

// Autoconfig ist ein Satz von Dateiänderungen, den das Panel für einen Server vorgibt. Der Daemon wendet ihn an, sobald
// die Dateien da sind (beim ersten Start also nach dem Hochfahren), und bei Änderungen sofort.
type Autoconfig struct {
	Version int         `json:"version"`
	Edits   []Edit      `json:"edits,omitempty"`
	Writes  []FileWrite `json:"writes,omitempty"`
	// Restart startet den Server einmal neu, wenn die Änderungen beim Hochfahren angewendet wurden und etwas
	// geändert haben (Paper liest seine Config nur beim Start).
	Restart bool `json:"restart,omitempty"`
	// Reload ist ein Konsolenbefehl, der laufende Server die Änderung einlesen lässt (z. B. "velocity reload").
	Reload string `json:"reload,omitempty"`
}

const (
	maxEdits      = 32
	maxSets       = 256
	maxWrites     = 8
	maxWriteBytes = 64 << 10
	maxConfigSize = 5 << 20
)

// Validate prüft den Satz auf Größe und Form.
func (a *Autoconfig) Validate() error {
	if len(a.Edits) > maxEdits || len(a.Writes) > maxWrites {
		return errors.New("zu viele Dateiänderungen")
	}
	for _, edit := range a.Edits {
		if edit.File == "" || strings.ContainsRune(edit.File, 0) {
			return errors.New("jede Änderung braucht einen Dateinamen")
		}
		switch edit.Format {
		case "properties", "yaml", "toml":
		default:
			return fmt.Errorf("unbekanntes Format %q (properties, yaml, toml)", edit.Format)
		}
		if len(edit.Set) > maxSets || len(edit.Delete) > maxSets {
			return errors.New("zu viele Werte in einer Datei")
		}
		for _, op := range edit.Set {
			if len(op.Path) == 0 || (edit.Format == "properties" && len(op.Path) != 1) {
				return fmt.Errorf("ungültiger Pfad in %s", edit.File)
			}
			for _, part := range op.Path {
				if part == "" {
					return fmt.Errorf("ungültiger Pfad in %s", edit.File)
				}
			}
		}
	}
	for _, write := range a.Writes {
		if write.Path == "" || len(write.Content) > maxWriteBytes {
			return errors.New("ungültige Datei zum Schreiben")
		}
	}
	if len(a.Reload) > 200 || strings.ContainsAny(a.Reload, "\r\n") {
		return errors.New("ungültiger Reload-Befehl")
	}
	return nil
}

// ApplyAutoconfig wendet den Satz an. missing nennt Dateien, die es noch nicht gibt (dann ist der Satz noch offen).
func ApplyAutoconfig(files *FS, a Autoconfig) (changed bool, missing []string, err error) {
	for _, edit := range a.Edits {
		did, editErr := applyEdit(files, edit)
		if errors.Is(editErr, fs.ErrNotExist) {
			missing = append(missing, edit.File)
			continue
		}
		if editErr != nil {
			return changed, missing, fmt.Errorf("%s: %w", edit.File, editErr)
		}
		changed = changed || did
	}
	for _, write := range a.Writes {
		current, readErr := files.Read(write.Path, maxConfigSize)
		if readErr == nil && string(current) == write.Content {
			continue
		}
		if readErr != nil && !errors.Is(readErr, fs.ErrNotExist) {
			return changed, missing, fmt.Errorf("%s: %w", write.Path, readErr)
		}
		if err := writeFile(files, write.Path, write.Content); err != nil {
			return changed, missing, fmt.Errorf("%s: %w", write.Path, err)
		}
		changed = true
	}
	return changed, missing, nil
}

func writeFile(files *FS, name, content string) error {
	err := files.Write(name, strings.NewReader(content), maxConfigSize, true)
	if errors.Is(err, fs.ErrExist) {
		err = files.Write(name, strings.NewReader(content), maxConfigSize, false)
	}
	return err
}

func applyEdit(files *FS, edit Edit) (bool, error) {
	data, err := files.Read(edit.File, maxConfigSize)
	if errors.Is(err, fs.ErrNotExist) {
		if !edit.Create {
			return false, fs.ErrNotExist
		}
		if dir := parentDir(edit.File); dir != "" {
			if err := files.Mkdir(dir); err != nil {
				return false, err
			}
		}
		data, err = nil, nil
	}
	if err != nil {
		return false, err
	}
	var updated []byte
	var changed bool
	switch edit.Format {
	case "properties":
		updated, changed = editProperties(data, edit)
	case "yaml":
		updated, changed, err = editYAML(data, edit)
	case "toml":
		updated, changed, err = editTOML(data, edit)
	}
	if err != nil {
		return false, err
	}
	created := data == nil
	if !changed && !created {
		return false, nil
	}
	return true, writeFile(files, edit.File, string(updated))
}

func parentDir(name string) string {
	name = strings.ReplaceAll(name, "\\", "/")
	if i := strings.LastIndex(name, "/"); i > 0 {
		return name[:i]
	}
	return ""
}

// ---------------------------------------------------------------- Werte

// normalize macht aus JSON-Werten Werte, die YAML und TOML richtig schreiben (ganze Zahlen ohne ".0").
func normalize(value any) any {
	switch v := value.(type) {
	case float64:
		if v == math.Trunc(v) && math.Abs(v) < 1e15 {
			return int64(v)
		}
		return v
	case []any:
		out := make([]any, len(v))
		for i, item := range v {
			out[i] = normalize(item)
		}
		return out
	case map[string]any:
		out := make(map[string]any, len(v))
		for key, item := range v {
			out[key] = normalize(item)
		}
		return out
	}
	return value
}

func scalarString(value any) string {
	switch v := normalize(value).(type) {
	case string:
		return v
	case bool:
		if v {
			return "true"
		}
		return "false"
	case int64:
		return fmt.Sprint(v)
	case nil:
		return ""
	}
	return fmt.Sprint(value)
}

// ---------------------------------------------------------------- Properties

func editProperties(data []byte, edit Edit) ([]byte, bool) {
	text := string(data)
	newline := "\n"
	if strings.Contains(text, "\r\n") {
		newline = "\r\n"
	}
	lines := strings.Split(strings.ReplaceAll(text, "\r\n", "\n"), "\n")
	if len(lines) > 0 && lines[len(lines)-1] == "" {
		lines = lines[:len(lines)-1]
	}
	changed := false
	for _, op := range edit.Set {
		key := op.Path[0]
		want := key + "=" + scalarString(op.Value)
		found := false
		for i, line := range lines {
			trimmed := strings.TrimSpace(line)
			if trimmed == "" || strings.HasPrefix(trimmed, "#") || strings.HasPrefix(trimmed, "!") {
				continue
			}
			name := trimmed
			if cut := strings.IndexAny(trimmed, "=:"); cut >= 0 {
				name = strings.TrimSpace(trimmed[:cut])
			}
			if name != key {
				continue
			}
			found = true
			if line != want {
				lines[i] = want
				changed = true
			}
			break
		}
		if !found {
			lines = append(lines, want)
			changed = true
		}
	}
	for _, path := range edit.Delete {
		if len(path) != 1 {
			continue
		}
		kept := lines[:0:0]
		for _, line := range lines {
			trimmed := strings.TrimSpace(line)
			name := trimmed
			if cut := strings.IndexAny(trimmed, "=:"); cut >= 0 {
				name = strings.TrimSpace(trimmed[:cut])
			}
			if !strings.HasPrefix(trimmed, "#") && name == path[0] {
				changed = true
				continue
			}
			kept = append(kept, line)
		}
		lines = kept
	}
	return []byte(strings.Join(lines, newline) + newline), changed
}

// ---------------------------------------------------------------- YAML

func editYAML(data []byte, edit Edit) ([]byte, bool, error) {
	var doc yaml.Node
	if len(bytes.TrimSpace(data)) > 0 {
		if err := yaml.Unmarshal(data, &doc); err != nil {
			return nil, false, fmt.Errorf("keine gültige YAML-Datei: %w", err)
		}
	}
	if doc.Kind == 0 {
		doc = yaml.Node{Kind: yaml.DocumentNode, Content: []*yaml.Node{{Kind: yaml.MappingNode, Tag: "!!map"}}}
	}
	root := doc.Content[0]
	if root.Kind != yaml.MappingNode {
		return nil, false, errors.New("die Datei ist keine YAML-Tabelle")
	}
	before, err := marshalYAML(&doc)
	if err != nil {
		return nil, false, err
	}
	for _, op := range edit.Set {
		if err := setYAML(root, op.Path, normalize(op.Value)); err != nil {
			return nil, false, err
		}
	}
	for _, path := range edit.Delete {
		deleteYAML(root, path)
	}
	after, err := marshalYAML(&doc)
	if err != nil {
		return nil, false, err
	}
	return after, !bytes.Equal(before, after), nil
}

func marshalYAML(node *yaml.Node) ([]byte, error) {
	var buffer bytes.Buffer
	encoder := yaml.NewEncoder(&buffer)
	encoder.SetIndent(2)
	if err := encoder.Encode(node); err != nil {
		return nil, err
	}
	if err := encoder.Close(); err != nil {
		return nil, err
	}
	return buffer.Bytes(), nil
}

func yamlChild(mapping *yaml.Node, key string) (index int) {
	for i := 0; i+1 < len(mapping.Content); i += 2 {
		if mapping.Content[i].Value == key {
			return i
		}
	}
	return -1
}

func setYAML(mapping *yaml.Node, path []string, value any) error {
	for _, key := range path[:len(path)-1] {
		i := yamlChild(mapping, key)
		if i < 0 {
			mapping.Content = append(mapping.Content, &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: key}, &yaml.Node{Kind: yaml.MappingNode, Tag: "!!map"})
			i = len(mapping.Content) - 2
		} else if mapping.Content[i+1].Kind != yaml.MappingNode {
			mapping.Content[i+1] = &yaml.Node{Kind: yaml.MappingNode, Tag: "!!map"}
		}
		mapping = mapping.Content[i+1]
	}
	last := path[len(path)-1]
	var node yaml.Node
	if err := node.Encode(value); err != nil {
		return err
	}
	if i := yamlChild(mapping, last); i >= 0 {
		node.HeadComment, node.LineComment, node.FootComment = mapping.Content[i+1].HeadComment, mapping.Content[i+1].LineComment, mapping.Content[i+1].FootComment
		mapping.Content[i+1] = &node
		return nil
	}
	mapping.Content = append(mapping.Content, &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: last}, &node)
	return nil
}

func deleteYAML(mapping *yaml.Node, path []string) {
	for _, key := range path[:len(path)-1] {
		i := yamlChild(mapping, key)
		if i < 0 || mapping.Content[i+1].Kind != yaml.MappingNode {
			return
		}
		mapping = mapping.Content[i+1]
	}
	if i := yamlChild(mapping, path[len(path)-1]); i >= 0 {
		mapping.Content = append(mapping.Content[:i], mapping.Content[i+2:]...)
	}
}

// ---------------------------------------------------------------- TOML

func editTOML(data []byte, edit Edit) ([]byte, bool, error) {
	table := map[string]any{}
	if len(bytes.TrimSpace(data)) > 0 {
		if err := toml.Unmarshal(data, &table); err != nil {
			return nil, false, fmt.Errorf("keine gültige TOML-Datei: %w", err)
		}
	}
	before, err := toml.Marshal(table)
	if err != nil {
		return nil, false, err
	}
	for _, op := range edit.Set {
		setTOML(table, op.Path, normalize(op.Value))
	}
	for _, path := range edit.Delete {
		deleteTOML(table, path)
	}
	after, err := toml.Marshal(table)
	if err != nil {
		return nil, false, err
	}
	return after, !bytes.Equal(before, after), nil
}

func setTOML(table map[string]any, path []string, value any) {
	for _, key := range path[:len(path)-1] {
		next, ok := table[key].(map[string]any)
		if !ok {
			next = map[string]any{}
			table[key] = next
		}
		table = next
	}
	table[path[len(path)-1]] = value
}

func deleteTOML(table map[string]any, path []string) {
	for _, key := range path[:len(path)-1] {
		next, ok := table[key].(map[string]any)
		if !ok {
			return
		}
		table = next
	}
	delete(table, path[len(path)-1])
}
