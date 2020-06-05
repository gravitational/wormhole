package vt100

import (
	"html/template"
	"net/http"
	"sync"

	"github.com/jaguilar/vt100"
)

// Handler is a type that knows how to serve the VT100 as an HTML
// page. This is useful as a way to debug problems, or display
// the state of the terminal to a user.
//
// TODO(jaguilar): move me to a subpackage, then export to the
// default mux automatically. This seems to be the way it's done
// in the std lib.
type handler struct {
	*vt100.VT100
	sync.Locker
}

var termTemplate = template.Must(template.New("vt100_html").Parse(`
	<html><head><style>
	.mono {
		font-family: "monospace";
	}</style>
	<body>
	<p>VT100 Terminal
	<p>Dimensions {{.Height}}x{{.Width}}
	<p><span class="mono">{{.ConsoleHTML}}</span>
	</span>
	</html>
`))

// ServeHTTP is part of the http.Handler interface.
func (v handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	v.Lock()
	defer v.Unlock()

	termTemplate.Execute(w, struct {
		Height, Width int
		ConsoleHTML   template.HTML
	}{v.Height, v.Width, template.HTML(v.HTML())})
}

// Export displays a status page showing the state of v under the given
// path. This page is added to the DefaultMux in the http package.
// You must provide a mutex we can lock while doing the export. You must
// not modify v without holding the mutex yourself, or else you could
// have a data race on your hands.
func Export(pattern string, v *vt100.VT100, l sync.Locker) {
	http.Handle(pattern, handler{v, l})
}
