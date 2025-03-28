// internal/cli/output/output.go
package output

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"gopkg.in/yaml.v3"
)

type Format string

const (
	FormatText   Format = "text"
	FormatJSON   Format = "json"
	FormatYAML   Format = "yaml"
	FormatSilent Format = "silent"
)

// Output handles formatted output
type Output struct {
	format Format
	writer *os.File
}

// New creates a new Output instance
func New(format string, writer *os.File) *Output {
	return &Output{
		format: Format(strings.ToLower(format)),
		writer: writer,
	}
}

// Print prints data in the specified format
func (o *Output) Print(data any) error {
	switch o.format {
	case FormatSilent:
		return nil
	case FormatJSON:
		return json.NewEncoder(o.writer).Encode(data)
	case FormatYAML:
		return yaml.NewEncoder(o.writer).Encode(data)
	case FormatText:
		return o.printText(data)
	default:
		return fmt.Errorf("unsupported output format: %s", o.format)
	}
}

// printText handles text formatting
func (o *Output) printText(data any) error {
	switch v := data.(type) {
	case string:
		_, err := fmt.Fprintln(o.writer, v)
		return err
	case []string:
		for _, s := range v {
			if _, err := fmt.Fprintln(o.writer, s); err != nil {
				return err
			}
		}
		return nil
	case map[string]any:
		for k, v := range v {
			if _, err := fmt.Fprintf(o.writer, "%s: %v\n", k, v); err != nil {
				return err
			}
		}
		return nil
	default:
		_, err := fmt.Fprintf(o.writer, "%v\n", data)
		return err
	}
}

// Error prints error messages
func (o *Output) Error(err error) error {
	if o.format == FormatSilent {
		return nil
	}
	_, err = fmt.Fprintf(o.writer, "Error: %v\n", err)
	return err
}

// Success prints success messages
func (o *Output) Success(msg string) error {
	if o.format == FormatSilent {
		return nil
	}
	_, err := fmt.Fprintf(o.writer, "Success: %s\n", msg)
	return err
}
