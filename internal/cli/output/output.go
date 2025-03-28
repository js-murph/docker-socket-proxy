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
		return o.PrintText(data.(string))
	default:
		return fmt.Errorf("unsupported output format: %s", o.format)
	}
}

// Error prints error messages
func (o *Output) Error(err error) {
	if o.format == FormatSilent {
		return
	}
	_, _ = fmt.Fprintf(o.writer, "Error: %v\n", err)
}

// Success prints success messages
func (o *Output) Success(msg string) {
	if o.format == FormatSilent {
		return
	}
	_, _ = fmt.Fprintf(o.writer, "Success: %s\n", msg)
}

// PrintText prints data in text format
func (o *Output) PrintText(text string) error {
	if o.format == FormatSilent {
		return nil
	}
	_, err := fmt.Fprintln(o.writer, text)
	return err
}

// Writer returns the output writer
func (o *Output) Writer() *os.File {
	return o.writer
}
