// Package montemplates parses HTML templates from a file tree and optionally monitors for changes.
//
// During construction, it parses all the templates under a root directory, traversing subdirectories
// for files with the given patterns.
//
// At execution time, if `dynamic` is set to true, at every request (`Get()` method) it checks whether
// files have changed, and re-parses them accordingly.
// This is very useful during development, but you want to turn it off during production because
// of the cost of checking whether the files changed in the filesystem.
//
// If `dynamic==true` it does proper serialization (`sync.Mutex`) to prevent concurrency conflicts.
// If `dynamic==false` it is read-only and there is no contention.
//
// ## Example
//
//	import (
//		montemplates "github.com/janpfeifer/monitored-templates"
//	)
//
//	flagDynamicTemplates = flag.Bool("dynamic_templates", false,
//		"If set, template files are checked at every access to checks for changes. "+
//		"Slow, leave this disabled for production.")
//
//	func main() {
//		...
//		templateSet, err := montemplates.New(
//			rootTemplatesPath,  // Path searched for template files.
//			[]string{"*.html", "*.js", "*.css"},  // File patterns to be read into templates.
//			*flagDynamicTemplates)  // If true, files are monitored for changes and re-parsed accordingly.
//		...
//		h := func (w http.ResponseWriter, req *http.Request) {
//			t, err := templateSet.Get("nav/login.html")  // Will re-read the file if changed
//			err = t.Execute(w, ...)
//		}
//		...
//	}
package montemplates

import (
	"golang.org/x/exp/slices"
	"html/template"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"sync"
	"time"

	"github.com/pkg/errors"
)

// Collection manages all templates under a certain directory.
type Collection struct {
	root     string
	patterns []string
	dynamic  bool
	current  *template.Template
	modTimes map[string]time.Time

	mu sync.Mutex // Serializes access to templates: only used if dynamic is used.
}

// New creates a Collection with parsed templates (files) from a directory.
//
// The `root` directory is recursively traversed and every file with the
// given `patterns`.
//
// Each pattern is checked against the file name (without the path) of each file under `root`,
// with the same semantics as `filepath.Match`.
// Notice this doesn't allow directory patterns to be matched (a limitation with filepath.Match).
//
// If `dynamic` is used at every call to `Get()` for a template, it will
// check whether files are changed, and update accordingly.
// This also has the side effect of running much slower, so likely something
// only used for development.
func New(root string, patterns []string, dynamic bool) (collection *Collection, err error) {
	c := &Collection{
		root:     root,
		patterns: slices.Clone(patterns),
		dynamic:  dynamic,
	}
	err = c.update()
	if err != nil {
		return
	}
	collection = c
	return
}

// update re-parses all templates from disk.
// It assumes c.mu is locked -- or during build time, when there is no contention.
func (c *Collection) update() (err error) {
	var templateSet *template.Template
	c.modTimes = make(map[string]time.Time)

	// Find files with given patterns.
	var files []string
	rootFS := os.DirFS(c.root)
	err = fs.WalkDir(rootFS, ".", func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		for _, pattern := range c.patterns {
			matched, err := filepath.Match(pattern, path.Base(p))
			if err != nil {
				return errors.WithMessagef(err, "failed matching with pattern %q", pattern)
			}
			if matched {
				files = append(files, p)
				return nil
			}
		}
		return nil
	})
	if err != nil {
		err = errors.Wrapf(err, "failed to traverse root=%q while searching for template files", c.root)
		return
	}

	// Parse files, and name them with directories:
	for _, name := range files {
		var contents []byte
		filePath := path.Join(c.root, name)
		fi, err := os.Stat(filePath)
		if err != nil {
			err = errors.WithMessagef(err, "failed to get file info for %q", filePath)
			break
		}
		modTime := fi.ModTime()
		contents, err = os.ReadFile(filePath)
		if err != nil {
			err = errors.WithMessagef(err, "failed to read template file %q", filePath)
			break
		}
		if templateSet == nil {
			templateSet = template.New(name)
		} else {
			templateSet = templateSet.New(name)
		}
		templateSet, err = templateSet.Parse(string(contents))
		if err != nil {
			err = errors.WithMessagef(err, "while parsing template %q", name)
			break
		}
		c.modTimes[name] = modTime
	}
	if err != nil {
		err = errors.Wrapf(err, "failed to parse templates under %q with patterns %q", c.root, c.patterns)
		return
	}
	if templateSet == nil || len(templateSet.Templates()) == 0 {
		err = errors.Errorf("Zero templates found under %q with patterns %q", c.root, c.patterns)
		return
	}
	c.current = templateSet
	return
}

// Get returns the named template.
//
// If `dynamic` was set during build time, and any file changed, it will re-parse all templates.
//
// Notice that since montemplates has no access to the template dependency graph, all template files need
// checking for updates.
func (c *Collection) Get(name string) (*template.Template, error) {
	if !c.dynamic {
		c.mu.Lock()
		defer c.mu.Unlock()
	}

	t := c.current.Lookup(name)
	if t == nil {
		return nil, errors.Errorf("Template %q not found in collection in root=%q, patterns=%q",
			name, c.root, c.patterns)
	}
	if !c.dynamic {
		return t, nil
	}

	// Check if any files changed -- we don't have the dependency graph of the templates, so
	// to be sure, if any template changed, re-parse everything.
	var needsUpdate bool
	for n, parsedModTime := range c.modTimes {
		filePath := path.Join(c.root, n)
		fi, err := os.Stat(filePath)
		if err != nil {
			return nil, errors.Wrapf(err, "Get(%q): failed to get file info for template %q, path %q", name, n, filePath)
		}
		modTime := fi.ModTime()
		if modTime.After(parsedModTime) {
			needsUpdate = true
			break
		}
	}
	if !needsUpdate {
		return t, nil
	}

	// File has been updated, re-parse template.
	// Since there is no way to update the definition of only one template, we need to re-parse the whole tree.
	err := c.update()
	if err != nil {
		return nil, errors.WithMessagef(err, "triggered by up-to-date template %q", name)
	}
	t = c.current.Lookup(name)
	if t == nil {
		return nil, errors.Errorf("After update, template %q no longer found in collection in root=%q, patterns=%q",
			name, c.root, c.patterns)
	}

	return t, nil
}

// Template returns one of the underlying templates.
// This can be useful to enumerate all the templates with `c.Template().Templates()`.
func (c *Collection) Template() *template.Template {
	return c.current
}
