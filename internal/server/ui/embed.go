// Package ui provides embedded HTML templates and static assets for sidequest server UIs.
package ui

import (
	"embed"
	"html/template"
	"io"
	"net/http"
)

//go:embed templates/*.html static/*.css
var content embed.FS

// Templates holds the parsed HTML templates.
var Templates *template.Template

func init() {
	Templates = template.Must(template.ParseFS(content, "templates/*.html"))
}

// StaticHandler returns an http.Handler that serves embedded static files.
func StaticHandler() http.Handler {
	return http.FileServer(http.FS(content))
}

// ServerInfo describes a running server for the landing page.
type ServerInfo struct {
	Name     string
	Protocol string
	Port     int
	Enabled  bool
	Paths    []PathInfo
}

// PathInfo describes an endpoint path.
type PathInfo struct {
	Method      string
	Path        string
	Description string
}

// LandingData holds template data for the landing page.
type LandingData struct {
	Version string
	Commit  string
	Date    string
	Servers []ServerInfo
}

// RESTExplorerData holds template data for the REST explorer.
type RESTExplorerData struct {
	Port int
}

// IdentityExplorerData holds template data for the OIDC explorer.
type IdentityExplorerData struct {
	Port   int
	Issuer string
}

// RenderLanding renders the landing page template to the writer.
func RenderLanding(w io.Writer, data LandingData) error {
	return Templates.ExecuteTemplate(w, "landing.html", data)
}

// RenderREST renders the REST explorer template to the writer.
func RenderREST(w io.Writer, data RESTExplorerData) error {
	return Templates.ExecuteTemplate(w, "rest.html", data)
}

// RenderIdentity renders the identity explorer template to the writer.
func RenderIdentity(w io.Writer, data IdentityExplorerData) error {
	return Templates.ExecuteTemplate(w, "identity.html", data)
}
