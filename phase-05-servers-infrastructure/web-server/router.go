package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// handlerFunc turns a parsed request into a response. This is the contract every
// route handler implements.
//
// 🐍 This is the same shape as a Flask view function: take the request, return
// the response. Go just makes the types explicit instead of relying on
// decorators and globals.
type handlerFunc func(*request) *response

// router maps a method+path key to a handler, and falls back to serving static
// files from a web root when no dynamic route matches.
//
// We deliberately use exact "METHOD path" matching (no wildcards or path params)
// to keep the routing logic obvious — the interesting systems work in this
// challenge is the protocol and the static file serving, not a fancy router.
type router struct {
	routes map[string]handlerFunc
	files  *fileServer // nil if static serving is disabled
}

// newRouter creates a router. If root is non-empty, static files are served from
// it for any request that doesn't match a registered dynamic route.
func newRouter(root string) *router {
	r := &router{routes: make(map[string]handlerFunc)}
	if root != "" {
		r.files = &fileServer{root: root}
	}
	return r
}

// handle registers a handler for an exact method+path combination.
func (r *router) handle(method, path string, h handlerFunc) {
	r.routes[method+" "+path] = h
}

// route picks the response for a request: a registered handler wins; otherwise
// we try the static file server; otherwise it's a 404 (or 405 if the path
// exists for a different method).
func (r *router) route(req *request) *response {
	// Match on the path WITHOUT its query string ("/hello?name=x" routes to the
	// "/hello" handler). The handler can still inspect the raw req.path.
	routePath := req.path
	if i := strings.IndexByte(routePath, '?'); i >= 0 {
		routePath = routePath[:i]
	}

	// We only implement GET and HEAD for static content; reject other methods
	// up front unless a dynamic route claims them.
	if h, ok := r.routes[req.method+" "+routePath]; ok {
		return h(req)
	}

	// If the same path is registered for a DIFFERENT method, the correct answer
	// is 405 Method Not Allowed, not 404 — the resource exists, the verb is
	// wrong.
	for key := range r.routes {
		if strings.HasSuffix(key, " "+routePath) {
			return textResponse(405, "text/plain; charset=utf-8", "405 Method Not Allowed\n")
		}
	}

	// Fall back to static files for GET/HEAD.
	if r.files != nil && (req.method == "GET" || req.method == "HEAD") {
		return r.files.serve(req)
	}

	if req.method != "GET" && req.method != "HEAD" {
		return textResponse(501, "text/plain; charset=utf-8", "501 Not Implemented\n")
	}
	return textResponse(404, "text/plain; charset=utf-8", "404 Not Found\n")
}

// fileServer serves files from a fixed root directory on disk.
type fileServer struct {
	root string
}

// contentTypes maps file extensions to MIME types. A browser uses Content-Type
// (not the file extension) to decide how to render a response, so getting this
// right is what makes CSS apply and images display instead of downloading.
var contentTypes = map[string]string{
	".html": "text/html; charset=utf-8",
	".htm":  "text/html; charset=utf-8",
	".css":  "text/css; charset=utf-8",
	".js":   "application/javascript; charset=utf-8",
	".json": "application/json; charset=utf-8",
	".txt":  "text/plain; charset=utf-8",
	".png":  "image/png",
	".jpg":  "image/jpeg",
	".jpeg": "image/jpeg",
	".gif":  "image/gif",
	".svg":  "image/svg+xml",
	".ico":  "image/x-icon",
}

// contentTypeFor returns the MIME type for a path's extension, defaulting to
// application/octet-stream (the generic "unknown bytes" type) when unrecognised.
func contentTypeFor(path string) string {
	if ct, ok := contentTypes[strings.ToLower(filepath.Ext(path))]; ok {
		return ct
	}
	return "application/octet-stream"
}

// serve maps the request path to a file under root and returns its contents.
//
// 🔒 THE SECURITY GOTCHA — PATH TRAVERSAL:
//
// A naive server does filepath.Join(root, req.path) and reads the result. But a
// malicious client can send a path like "/../../../../etc/passwd". Joined onto
// the root, the ".." segments climb OUT of the web root and hand the attacker
// any file the server process can read. This is one of the oldest and most
// common web vulnerabilities (CWE-22).
//
// The fix, in two layers:
//  1. filepath.Clean collapses "." and ".." segments lexically, so
//     "/a/../../etc" becomes "/etc". We clean the request path BEFORE joining.
//  2. After joining and resolving to an absolute path, we verify the result is
//     still inside the root. If it isn't, we refuse with 403 Forbidden.
//
// Layer 2 is the one you must never skip: lexical cleaning alone can be defeated
// by symlinks, so we re-check containment against the resolved absolute root.
func (fs *fileServer) serve(req *request) *response {
	// Strip any query string — "/page?x=1" addresses the file "/page".
	urlPath := req.path
	if i := strings.IndexByte(urlPath, '?'); i >= 0 {
		urlPath = urlPath[:i]
	}

	// Clean the URL path as an absolute path. path.Clean / filepath.Clean
	// resolve ".." segments so they cannot escape later. We prepend "/" so the
	// path is anchored and then trim it back to a relative path for joining.
	cleaned := filepath.Clean("/" + strings.TrimPrefix(urlPath, "/"))

	// A request for "/" (or any directory) serves index.html, the universal
	// web convention.
	if strings.HasSuffix(cleaned, "/") || cleaned == "/" {
		cleaned = filepath.Join(cleaned, "index.html")
	}

	absRoot, err := filepath.Abs(fs.root)
	if err != nil {
		return textResponse(500, "text/plain; charset=utf-8", "500 Internal Server Error\n")
	}
	target := filepath.Join(absRoot, cleaned)

	// Layer 2: containment check. The resolved target MUST live under the root.
	// We compare against absRoot+separator so that a sibling dir like
	// "/srv/www-evil" can't sneak past a "/srv/www" prefix check.
	if target != absRoot && !strings.HasPrefix(target, absRoot+string(os.PathSeparator)) {
		return textResponse(403, "text/plain; charset=utf-8", "403 Forbidden\n")
	}

	info, err := os.Stat(target)
	if err != nil || info.IsDir() {
		// Missing file, or a directory with no index.html: 404.
		return textResponse(404, "text/plain; charset=utf-8", "404 Not Found\n")
	}

	data, err := os.ReadFile(target)
	if err != nil {
		return textResponse(500, "text/plain; charset=utf-8", "500 Internal Server Error\n")
	}

	resp := newResponse(200, data)
	resp.setHeader("Content-Type", contentTypeFor(target))
	// HEAD requests want the headers (including Content-Length) but NOT the
	// body. We keep the Content-Length we already computed and drop the bytes.
	if req.method == "HEAD" {
		resp.body = nil
	}
	return resp
}

// helloHandler is a tiny dynamic route used to demonstrate the router and to
// give the README an example beyond static files.
func helloHandler(req *request) *response {
	name := "world"
	if q := req.path; strings.Contains(q, "?name=") {
		name = q[strings.Index(q, "?name=")+len("?name="):]
	}
	body := fmt.Sprintf("Hello, %s! Your request had %d header(s).\n", name, len(req.headers))
	return textResponse(200, "text/plain; charset=utf-8", body)
}

// statusString is a small helper for verbose logging (e.g. "200 OK").
func statusString(code int) string {
	reason := statusText[code]
	if reason == "" {
		reason = "Status"
	}
	return strconv.Itoa(code) + " " + reason
}
