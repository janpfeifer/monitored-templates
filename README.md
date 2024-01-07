# Monitored Templates

Go library that parses HTML templates from a file tree.

During construction it parses all the templates under a root directory, traversing subdirectories
for files with the given suffixes.

At execution time, if `dynamic` is set to true, at every request (`Get()` method) it checks whether 
files have changed, and re-parses them accordingly.
This is very useful during development, but you want to turn it off during production because
of the cost of checking whether the files changed in the filesystem.

If `dynamic==true` it does proper serialization (`sync.Mutex`) to prevent concurrency conflicts. 
If `dyncamic==false` it is read-only and there is no contention.

## Example

```go
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
		[]string{"*.html", "*.js", "*.css"},  // File patterns to be read into templates.
		*flagDynamicTemplates)  // If true, files are monitored for changes and re-parsed accordingly.
	...
	h := func (w http.ResponseWriter, req *http.Request) {
		t, err := templateSet.Get("nav/login.html")  // Will re-read the file if changed
		t.Execute(w, ...)
	}
	...
}
```

