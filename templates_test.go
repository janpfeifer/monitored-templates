package montemplates

import (
	"bytes"
	"github.com/stretchr/testify/require"
	"k8s.io/klog/v2"
	"os"
	"path"
	"testing"
	"time"
)

func init() {
	klog.InitFlags(nil)
}

// TestDynamicTemplates creates a couple of temporary template files,
// checks that they are correctly parsed, then dyncamically changes one of them,
// and makes sure the `templates.Collection` updates the parsed file.
func TestDynamicTemplates(t *testing.T) {
	// Create a temporary directory and files `a.html` and `s/b.html`
	tmpDir, err := os.MkdirTemp("", "warmap_templates_test")
	require.NoError(t, err)
	klog.Infof("TestDynamicTemplates: temporary directory %s", tmpDir)

	// Create test files `a.html` and `s/b.html` and something other file not used `foo.bar`.
	require.NoError(t, os.WriteFile(path.Join(tmpDir, "a.html"), []byte(`A({{template "s/b.html"}})`), 0755))
	require.NoError(t, os.Mkdir(path.Join(tmpDir, "s"), 0755))
	require.NoError(t, os.WriteFile(path.Join(tmpDir, "s", "b.html"), []byte(`B()`), 0755))
	require.NoError(t, os.WriteFile(path.Join(tmpDir, "foo.bar"), []byte(`What!?`), 0755))

	// Parse templates and checks correct files are there.
	c, err := New(tmpDir, []string{"*.html", "*.blah"}, true)
	require.NoError(t, err)
	klog.Infof("Templates found: %s", c.Template().DefinedTemplates())
	require.Equal(t, 2, len(c.Template().Templates()), "Only 2 template files match the patterns.")

	tmpl, err := c.Get("a.html")
	require.NoError(t, err)
	_, err = c.Get("s/b.html")
	require.NoError(t, err)
	_, err = c.Get("foo.html") // Not there.
	require.Error(t, err, "Template foo.html shouldn't exist")

	// Checks that templates were parsed and execute correctly.
	var b bytes.Buffer
	require.NoError(t, tmpl.Execute(&b, nil))
	require.Equal(t, "A(B())", b.String())

	// If we Get again, no updates should have happened.
	tmpl, err = c.Get("a.html")
	require.NoError(t, err)
	b.Reset()
	require.NoError(t, tmpl.Execute(&b, nil))
	require.Equal(t, "A(B())", b.String())

	// Changing "a.html" should update the execution accordingly:
	time.Sleep(10 * time.Millisecond) // Allow file modification time to change.
	require.NoError(t, os.WriteFile(path.Join(tmpDir, "a.html"),
		[]byte(`A(foo, {{template "s/b.html"}})`), 0755))
	tmpl, err = c.Get("a.html")
	require.NoError(t, err)
	b.Reset()
	require.NoError(t, tmpl.Execute(&b, nil))
	require.Equal(t, "A(foo, B())", b.String())

	// Changing "s/b.html" should also update the execution of "a.html" accordingly:
	time.Sleep(10 * time.Millisecond) // Allow file modification time to change.
	require.NoError(t, os.WriteFile(path.Join(tmpDir, "s/b.html"),
		[]byte(`B(bar)`), 0755))
	tmpl, err = c.Get("a.html")
	require.NoError(t, err)
	b.Reset()
	require.NoError(t, tmpl.Execute(&b, nil))
	require.Equal(t, "A(foo, B(bar))", b.String())

	require.NoError(t, os.RemoveAll(tmpDir)) // clean up
}
