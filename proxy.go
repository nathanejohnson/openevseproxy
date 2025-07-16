package main

import (
	"fmt"
	"log"
	"net/http"
	"net/http/httptest"
	"net/http/httputil"
	"net/url"
	"path"
)

// wrappedResponseWriter - embeds an httptest.ResponseRecorder,
// which implements http.ResponseWriter.  This also implements
// the Unwrap method use with ResponseController, so it can
// be hijacked by the httputil.ReverseProxy handler.
type wrappedResponseWriter struct {
	*httptest.ResponseRecorder
	orig     http.ResponseWriter
	hijacked bool
}

// Unwrap - allow httputil.ReverseProxy to hijack our connection in
// case of web sockets.
func (wrw *wrappedResponseWriter) Unwrap() http.ResponseWriter {
	wrw.hijacked = true
	return wrw.orig
}

type MongooseProxy struct {
	scheme      string
	address     string
	path        string
	rp          *httputil.ReverseProxy
	length304   int
	bodyText304 string
}

func (mp *MongooseProxy) Rewrite(pr *httputil.ProxyRequest) {
	pr.Out.URL.Host = mp.address
	pr.Out.URL.Scheme = mp.scheme
	pr.Out.URL.Path = path.Join(pr.Out.URL.Path, mp.path)
	log.Printf("rewriting %s => %s", pr.In.URL.String(), pr.Out.URL.String())
	for key := range pr.Out.Header {
		log.Printf("\t%s: %s", key, pr.Out.Header.Get(key))
	}
}

// ServeHTTP - implements http.Handler interface
func (mp *MongooseProxy) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// this should record the response unless it is hijacked, in which case
	// the underlying w is unwrapped and the reverse proxy handles it.  this
	// lets websockets continue to work, but we get to play with all other
	// responses if we want.  right now we just care about 304.
	wrw := &wrappedResponseWriter{
		ResponseRecorder: httptest.NewRecorder(),
		orig:             w,
	}
	log.Printf("%s %s %s", r.Method, r.URL.Path, r.RemoteAddr)
	mp.rp.ServeHTTP(wrw, r)
	if wrw.hijacked {
		// websockets will hijack the connection
		return
	}
	rc := http.NewResponseController(w)
	conn, _, err := rc.Hijack()
	if err != nil {
		log.Printf("unexpected error hijacking connection: %s", err)
		return
	}
	defer conn.Close()

	log.Printf("got code: %d\n", wrw.Code)

	if wrw.Code == 304 {
		tmpl := "HTTP/1.1 304 Not Modified\r\n" +
			"Server: Mongoose/6.14\r\n" +
			"Connection: close\r\n" +
			"Content-Type: text/plain\r\n" +
			"Content-Length: %d\r\n" +
			"\r\n" +
			"%s"

		_, err = conn.Write([]byte(fmt.Sprintf(tmpl, mp.length304, mp.bodyText304)))
	} else {
		err = wrw.Result().Write(conn)
	}
	if err != nil {
		log.Printf("error writing response: %s", err)
	}
}

func NewMongooseProxy(transport http.RoundTripper, upstrem, bodyText304 string, length304 int) (*MongooseProxy, error) {
	u, err := url.Parse(upstrem)
	if err != nil {
		return nil, fmt.Errorf("could not parse upstream error: %w", err)
	}
	mp := &MongooseProxy{
		scheme:      u.Scheme,
		address:     u.Host,
		path:        u.Path,
		length304:   length304,
		bodyText304: bodyText304,
	}
	mp.rp = &httputil.ReverseProxy{
		Rewrite:   mp.Rewrite,
		Transport: transport,
	}
	return mp, nil
}
