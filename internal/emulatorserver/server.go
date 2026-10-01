// Package emulatorserver serves the EmulatorJS player page and a ROM file.
//
// EmulatorJS ships no server of its own, so narthex re-execs its own binary
// (the hidden `emulator-serve` subcommand) which runs this server in a child
// process. The card reaches it either directly on its own port or through
// the same-origin /emulator/ reverse proxy, so the generated page uses
// relative URLs and works unchanged behind both.
package emulatorserver

import (
	"html"
	"io"
	"net"
	"net/http"
	"strconv"
)

// Options configures the child server.
type Options struct {
	// ROM is the path of the ROM file to play.
	ROM string
	// Core is the EmulatorJS core name (e.g. fceumm).
	Core string
	// Name is the display / save-state name of the game.
	Name string
	// Hostname and Port are the listen address.
	Hostname string
	Port     int
	// DataDir is a local EmulatorJS data/ directory served at /data/; when
	// empty the page loads the loader and cores from the public CDN.
	DataDir string
	// CDNVersion selects the CDN build when DataDir is empty
	// (stable/latest/nightly).
	CDNVersion string
}

// Run starts the server and blocks until it exits.
func Run(o Options) error {
	ln, err := net.Listen("tcp", net.JoinHostPort(o.Hostname, strconv.Itoa(o.Port)))
	if err != nil {
		return err
	}
	srv := &http.Server{Handler: Handler(o)}
	return srv.Serve(ln)
}

// Handler builds the EmulatorJS player handler: "/" serves the generated
// page, "/rom" the ROM file and "/data/..." the optional local data
// directory.
func Handler(o Options) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Header().Set("Cache-Control", "no-store")
		io.WriteString(w, playerHTML(o))
	})
	mux.HandleFunc("/rom", func(w http.ResponseWriter, r *http.Request) {
		// Range requests are handled by ServeFile, so EmulatorJS can stream
		// and seek within the ROM.
		w.Header().Set("Cache-Control", "private, max-age=3600")
		http.ServeFile(w, r, o.ROM)
	})
	if o.DataDir != "" {
		mux.Handle("/data/", http.StripPrefix("/data/", http.FileServer(http.Dir(o.DataDir))))
	}
	return mux
}

// playerHTML generates the EmulatorJS embed page. All injected values are
// HTML-escaped; the ROM and data paths stay relative so the page works both
// directly and through the same-origin proxy.
func playerHTML(o Options) string {
	dataURL := "data/"
	if o.DataDir == "" {
		version := o.CDNVersion
		if version == "" {
			version = "stable"
		}
		dataURL = "https://cdn.emulatorjs.org/" + version + "/data/"
	}
	esc := html.EscapeString
	name := esc(o.Name)
	return `<!doctype html>
<html lang="en">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<title>` + name + `</title>
<style>
html,body{margin:0;height:100%;background:#000;overflow:hidden}
#game{width:100vw;height:100vh}
</style>
</head>
<body>
<div id="game"></div>
<script>
window.EJS_player = "#game";
window.EJS_core = "` + esc(o.Core) + `";
window.EJS_gameUrl = "rom";
window.EJS_gameName = "` + name + `";
window.EJS_pathtodata = "` + esc(dataURL) + `";
</script>
<script src="` + esc(dataURL) + `loader.js"></script>
</body>
</html>`
}
