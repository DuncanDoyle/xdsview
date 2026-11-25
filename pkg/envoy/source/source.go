// Package source provides ways to obtain an Envoy /config_dump document.
//
// A [Source] hides the transport: [File] reads a saved dump and [Admin] fetches
// one from a reachable Envoy admin endpoint. The Kubernetes port-forward sources
// live in the kube subpackage so that this package depends only on the standard
// library.
package source

import (
	"context"
	"fmt"
	"io"
	"os"

	"github.com/DuncanDoyle/xdsview/pkg/envoy/model"
)

// Source returns a config dump as a stream. The caller must close the returned
// reader, which also releases anything the source opened to produce it.
// Capture says whether the dump is known to be complete; pass it to the
// parser in parser.Options.
type Source interface {
	ConfigDump(ctx context.Context) (io.ReadCloser, error)
	Capture() model.Capture
}

// File returns a [Source] that reads a config dump from the file at path.
func File(path string) Source {
	return fileSource(path)
}

type fileSource string

// ConfigDump opens the file. ctx is not used: opening a local file does not block.
func (f fileSource) ConfigDump(context.Context) (io.ReadCloser, error) {
	file, err := os.Open(string(f))
	if err != nil {
		return nil, fmt.Errorf("reading file %s: %w", string(f), err)
	}
	return file, nil
}

// Capture reports [model.CaptureUnverified]: a saved dump may have been filtered.
func (f fileSource) Capture() model.Capture { return model.CaptureUnverified }
