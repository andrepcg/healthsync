package parser

import (
	"archive/zip"
	"bytes"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"strings"
)

// Export gives the parser uniform access to an Apple Health export whether it
// is a .zip, a bare export.xml or an unpacked directory. The main XML refers to
// sibling files (GPX routes) by paths relative to the export root, and ECG CSVs
// are only discoverable by listing.
type Export interface {
	// OpenMain opens the <HealthData> XML.
	OpenMain() (io.ReadCloser, error)
	// Open opens a file referenced from the XML, e.g. "/workout-routes/x.gpx".
	Open(rel string) (io.ReadCloser, error)
	// List returns the names of all files (relative to the export root) for
	// which pred returns true.
	List(pred func(name string) bool) []string
	Close() error
}

// OpenExport opens a .zip, .xml or directory as an Export.
func OpenExport(p string) (Export, error) {
	fi, err := os.Stat(p)
	if err != nil {
		return nil, fmt.Errorf("stat %s: %w", p, err)
	}
	if fi.IsDir() {
		return openDirExport(p, "")
	}
	switch strings.ToLower(filepath.Ext(p)) {
	case ".zip":
		return openZipExport(p)
	case ".xml":
		return openDirExport(filepath.Dir(p), filepath.Base(p))
	default:
		return nil, fmt.Errorf("unsupported file type: %s (expected .zip or .xml)", filepath.Ext(p))
	}
}

// --- zip ---

type zipExport struct {
	r    *zip.ReadCloser
	main *zip.File
	root string // directory of the main xml inside the archive, "" or "apple_health_export"
}

func openZipExport(p string) (*zipExport, error) {
	r, err := zip.OpenReader(p)
	if err != nil {
		return nil, fmt.Errorf("opening zip: %w", err)
	}
	main, err := findHealthExport(r.File)
	if err != nil {
		r.Close()
		return nil, err
	}
	root := path.Dir(main.Name)
	if root == "." {
		root = ""
	}
	return &zipExport{r: r, main: main, root: root}, nil
}

func (z *zipExport) OpenMain() (io.ReadCloser, error) { return z.main.Open() }

func (z *zipExport) Open(rel string) (io.ReadCloser, error) {
	want := path.Join(z.root, strings.TrimPrefix(rel, "/"))
	for _, f := range z.r.File {
		if f.Name == want {
			return f.Open()
		}
	}
	// Fall back to a suffix match: the referenced directory may be localized
	// or the archive re-packed under a different root.
	suffix := "/" + strings.TrimPrefix(rel, "/")
	for _, f := range z.r.File {
		if strings.HasPrefix(f.Name, "__MACOSX/") {
			continue
		}
		if strings.HasSuffix(f.Name, suffix) {
			return f.Open()
		}
	}
	return nil, fs.ErrNotExist
}

func (z *zipExport) List(pred func(string) bool) []string {
	var out []string
	for _, f := range z.r.File {
		if f.FileInfo().IsDir() || strings.HasPrefix(f.Name, "__MACOSX/") {
			continue
		}
		name := f.Name
		if z.root != "" {
			name = strings.TrimPrefix(name, z.root+"/")
		}
		if pred(name) {
			out = append(out, name)
		}
	}
	return out
}

func (z *zipExport) Close() error { return z.r.Close() }

// --- directory ---

type dirExport struct {
	root string
	main string // file name of the main xml relative to root
}

func openDirExport(root, main string) (*dirExport, error) {
	if main == "" {
		entries, err := os.ReadDir(root)
		if err != nil {
			return nil, err
		}
		for _, e := range entries {
			if e.IsDir() || !strings.HasSuffix(strings.ToLower(e.Name()), ".xml") {
				continue
			}
			f, err := os.Open(filepath.Join(root, e.Name()))
			if err != nil {
				continue
			}
			head := make([]byte, 32*1024)
			n, _ := io.ReadFull(f, head)
			f.Close()
			if bytes.Contains(head[:n], []byte("<HealthData ")) {
				main = e.Name()
				break
			}
		}
		if main == "" {
			return nil, fmt.Errorf("no HealthKit export XML found in %s", root)
		}
	}
	return &dirExport{root: root, main: main}, nil
}

func (d *dirExport) OpenMain() (io.ReadCloser, error) {
	return os.Open(filepath.Join(d.root, d.main))
}

func (d *dirExport) Open(rel string) (io.ReadCloser, error) {
	return os.Open(filepath.Join(d.root, filepath.FromSlash(strings.TrimPrefix(rel, "/"))))
}

func (d *dirExport) List(pred func(string) bool) []string {
	var out []string
	filepath.WalkDir(d.root, func(p string, e fs.DirEntry, err error) error {
		if err != nil || e.IsDir() {
			return nil
		}
		rel, err := filepath.Rel(d.root, p)
		if err != nil {
			return nil
		}
		rel = filepath.ToSlash(rel)
		if pred(rel) {
			out = append(out, rel)
		}
		return nil
	})
	return out
}

func (d *dirExport) Close() error { return nil }

// findHealthExport locates the HealthKit export XML inside an Apple Health zip.
//
// The filename is not reliable: Apple localizes it per device language
// ("export.xml" on English, "导出.xml" on Chinese, and so on), users can
// rename the file, and re-zipped archives may include stray xml files. The
// zip also always contains a sibling "export_cda.xml" in a completely
// different schema (CDA / ClinicalDocument) which must never be parsed as
// HealthKit data.
//
// We therefore identify the export by content, not by name: any .xml entry
// whose head contains "<HealthData" is the HealthKit export. The CDA file
// starts with "<ClinicalDocument" so it is naturally rejected.
func findHealthExport(files []*zip.File) (*zip.File, error) {
	var candidates []*zip.File
	for _, f := range files {
		if f.FileInfo().IsDir() {
			continue
		}
		if !strings.HasSuffix(strings.ToLower(f.Name), ".xml") {
			continue
		}
		candidates = append(candidates, f)
	}

	for _, f := range candidates {
		ok, err := looksLikeHealthKitXML(f)
		if err != nil {
			return nil, fmt.Errorf("reading %s: %w", f.Name, err)
		}
		if ok {
			return f, nil
		}
	}

	if len(candidates) == 0 {
		return nil, fmt.Errorf("no .xml entries in zip archive")
	}
	return nil, fmt.Errorf("no HealthKit export XML found in zip archive (found %d .xml entries but none contained a <HealthData> root — is this an Apple Health export?)", len(candidates))
}

// looksLikeHealthKitXML reads the head of a zip entry and checks for the
// HealthKit root element. The match looks for "<HealthData " (with trailing
// space) rather than bare "<HealthData" so it cannot false-positive on
// "<HealthDataArchive" or similar; the root element always has at least the
// "locale" attribute. 32 KiB is well beyond the size of any real Apple
// Health DOCTYPE preamble (which is a few KiB of entity declarations) but
// cheap to read.
func looksLikeHealthKitXML(f *zip.File) (bool, error) {
	rc, err := f.Open()
	if err != nil {
		return false, err
	}
	defer rc.Close()

	head := make([]byte, 32*1024)
	n, err := io.ReadFull(rc, head)
	if err != nil && err != io.EOF && err != io.ErrUnexpectedEOF {
		return false, err
	}
	return bytes.Contains(head[:n], []byte("<HealthData ")), nil
}

// readerExport wraps a bare XML stream with no sibling files. It backs
// ParseReader, which tests and embedding callers use.
type readerExport struct{ r io.Reader }

func (e *readerExport) OpenMain() (io.ReadCloser, error)   { return io.NopCloser(e.r), nil }
func (e *readerExport) Open(string) (io.ReadCloser, error) { return nil, fs.ErrNotExist }
func (e *readerExport) List(func(string) bool) []string    { return nil }
func (e *readerExport) Close() error                       { return nil }
