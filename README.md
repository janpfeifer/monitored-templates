# Monitored Templates

Minimalistic Go library that parses HTML templates from a file tree, and monitors for changes. 

During construction, it parses all the templates under a root directory, traversing subdirectories
for files with the given patterns.

At execution time, if `dynamic` is set to true, at every request (`Get()` method) it checks whether 
files have changed, and re-parses them accordingly.
This is very useful during development or for simple/cheap "HMR" (Hot Module Reload).

Probably you want to turn it off during production because
of the cost of checking whether the files changed in the filesystem. 
But if you want other alternatives for "HMR" -- e.g.: asynchronous polling for changes every so often --
please create a feature request, it would be easy to implement. 

If `dynamic==true` it does proper serialization (`sync.Mutex`) to prevent concurrency conflicts. 
If `dyncamic==false` it is read-only and there is no contention.

## Example

```go
package main

import (
	montemplates "github.com/janpfeifer/monitored-templates"
)

flagDynamicTemplates = flag.Bool("dynamic_templates", false,
	"If set, template files are checked at every access to checks for changes. "+
	"Slow, leave this disabled for production.")

func main() {
	...
	templateSet, err := montemplates.New(
		rootTemplatesPath,  // Path searched for template files.
		[]string{"*.html", "*.js", "*.css"},  // File patterns to be read as templates.
		*flagDynamicTemplates)  // If true, files are monitored for changes and re-parsed accordingly.
	...
	loginHandler := func (w http.ResponseWriter, req *http.Request) {
		...
		t, err := templateSet.Get("nav/login.html")  // Re-parses the file if changed
		err = t.Execute(w, ...)
		if err != nil { ... }
	}
	...
	http.Handle("/login", loginHandler)
}
```

