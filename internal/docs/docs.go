// Package docs serves rye's docs and code at the deployed commit on docs.int.rye. The site is a git archive that
// make up pushes to the service VM. It has an llms.txt index, an llms-full.txt with every page, and each file at
// its path.
package docs

import (
	"archive/tar"
	"bytes"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"path"
	"slices"
	"strings"
	"sync"
)

// sections are the docs/ directories, in the order llms.txt lists them.
var sections = []struct{ dir, title string }{
	{"tutorial", "Tutorials"},
	{"how-to", "How-to guides"},
	{"reference", "Reference"},
	{"explanation", "Explanation"},
}

// page is a Markdown page that llms.txt lists.
type page struct {
	path, title string
}

// site is one loaded archive.
type site struct {
	files  map[string][]byte
	commit string
	// pages are README.md, then the docs pages by section and path.
	pages   []page
	summary string
}

// Handler serves the site from the archive at Source. It reads the archive on the first request after it exists.
type Handler struct {
	Source string

	mu   sync.Mutex
	site *site
}

func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	s, err := h.current()
	if err != nil {
		slog.Error("load docs", "err", err)
		http.Error(w, "no docs on the service VM; run make up on the host", http.StatusServiceUnavailable)
		return
	}
	name := strings.TrimPrefix(path.Clean("/"+r.URL.Path), "/")
	var body []byte
	switch name {
	case "", "llms.txt":
		body = s.index("http://" + r.Host)
	case "llms-full.txt":
		body = s.full()
	default:
		var ok bool
		if body, ok = s.files[name]; !ok {
			http.NotFound(w, r)
			return
		}
	}
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	_, _ = w.Write(body)
}

// current returns the site, reading the archive on the first call that finds it.
func (h *Handler) current() (*site, error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.site != nil {
		return h.site, nil
	}
	file, err := os.Open(h.Source)
	if err != nil {
		return nil, err
	}
	defer func() { _ = file.Close() }()
	s, err := load(file)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", h.Source, err)
	}
	h.site = s
	return s, nil
}

// load reads a git archive. git archive puts the commit in a global header's comment.
func load(r io.Reader) (*site, error) {
	s := &site{files: map[string][]byte{}}
	archive := tar.NewReader(r)
	for {
		header, err := archive.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return nil, err
		}
		if header.Typeflag == tar.TypeXGlobalHeader {
			s.commit = header.PAXRecords["comment"]
			if len(s.commit) > 12 {
				s.commit = s.commit[:12]
			}
			continue
		}
		if header.Typeflag != tar.TypeReg {
			continue
		}
		data, err := io.ReadAll(archive)
		if err != nil {
			return nil, err
		}
		s.files[path.Clean(header.Name)] = data
	}
	readme, ok := s.files["README.md"]
	if !ok {
		return nil, errors.New("no README.md")
	}
	s.summary = firstParagraph(readme)
	s.pages = append(s.pages, page{"README.md", title(readme, "README.md")})
	for _, section := range sections {
		for _, name := range sortedNames(s.files, "docs/"+section.dir+"/") {
			s.pages = append(s.pages, page{name, title(s.files[name], name)})
		}
	}
	return s, nil
}

// index builds llms.txt. It has a title, the README's first paragraph as the summary, and a link to each page by
// section.
func (s *site) index(base string) []byte {
	var b bytes.Buffer
	fmt.Fprintf(&b, "# rye\n\n> %s\n\n", s.summary)
	if s.commit != "" {
		fmt.Fprintf(&b, "These docs and code are rye at commit %s. Every file in the repo is at %s/<path>, and "+
			"%s/llms-full.txt has every page below in one file.\n", s.commit, base, base)
	}
	fmt.Fprintf(&b, "\n## Overview\n\n- [%s](%s/README.md)\n", s.pages[0].title, base)
	for _, section := range sections {
		prefix := "docs/" + section.dir + "/"
		started := false
		for _, p := range s.pages {
			if !strings.HasPrefix(p.path, prefix) {
				continue
			}
			if !started {
				fmt.Fprintf(&b, "\n## %s\n\n", section.title)
				started = true
			}
			fmt.Fprintf(&b, "- [%s](%s/%s)\n", p.title, base, p.path)
		}
	}
	return b.Bytes()
}

// full builds llms-full.txt. It holds every page that llms.txt lists, in its order, each after a line with its path.
func (s *site) full() []byte {
	var b bytes.Buffer
	for i, p := range s.pages {
		if i > 0 {
			b.WriteString("\n")
		}
		fmt.Fprintf(&b, "<!-- %s -->\n\n", p.path)
		b.Write(bytes.TrimRight(s.files[p.path], "\n"))
		b.WriteString("\n")
	}
	return b.Bytes()
}

// title is a page's first level-one heading, or its path without one.
func title(data []byte, name string) string {
	for _, line := range strings.Split(string(data), "\n") {
		if heading, ok := strings.CutPrefix(line, "# "); ok {
			return strings.TrimSpace(heading)
		}
	}
	return name
}

// firstParagraph is the first paragraph after the title, joined onto one line.
func firstParagraph(data []byte) string {
	var lines []string
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		switch {
		case strings.HasPrefix(line, "#"):
			continue
		case line == "" && len(lines) > 0:
			return strings.Join(lines, " ")
		case line != "":
			lines = append(lines, line)
		}
	}
	return strings.Join(lines, " ")
}

// sortedNames are the Markdown files directly in dir, sorted.
func sortedNames(files map[string][]byte, dir string) []string {
	var names []string
	for name := range files {
		rest, ok := strings.CutPrefix(name, dir)
		if ok && !strings.Contains(rest, "/") && strings.HasSuffix(rest, ".md") {
			names = append(names, name)
		}
	}
	slices.Sort(names)
	return names
}
