package main

import (
	"bytes"
	"io/fs"
	"log"
	"mime"
	"net/http"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/PuerkitoBio/goquery"
	"github.com/fsnotify/fsnotify"
	"github.com/gorilla/websocket"
)

const (
	script      = `<script src="tinyreload.js"></script>`
	minEventTTL = 100 * time.Millisecond
)

var upgrader = websocket.Upgrader{
	ReadBufferSize:  16,
	WriteBufferSize: 16,
}

var connections []*websocket.Conn

type TinyHandler struct {
	root string
}

func NewTinyHandler(root string) *TinyHandler {
	return &TinyHandler{root}
}

func (th *TinyHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	cleanPath := filepath.Clean(r.URL.Path)

	path := filepath.Join(th.root, cleanPath)
	// log.Println(path)
	if !strings.HasPrefix(path, th.root) {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}

	info, err := os.Stat(path)
	if err != nil {
		http.NotFound(w, r)
		return
	}

	if info.IsDir() {
		path = filepath.Join(path, "index.html")
	}

	// log.Println("serving " + path)

	ext := filepath.Ext(path)
	if mimeType := mime.TypeByExtension(ext); mimeType != "" {
		w.Header().Set("Content-Type", mimeType)

		// make sure not to get cached by the browser (html files for sure)
		w.Header().Set("Cache-Control", "no-store, no-cache, must-revalidate, proxy-revalidate")
		w.Header().Set("Pragma", "no-cache")
		w.Header().Set("Expires", "0")
	} else {
		w.Header().Set("Content-Type", "application/octet-stream")
	}

	if ext != ".html" {
		http.ServeFile(w, r, path)
		return
	}

	// inject script
	f, err := os.Open(path)
	if err != nil {
		panic(err)
	}
	defer f.Close()

	doc, err := goquery.NewDocumentFromReader(f)
	if err != nil {
		panic(err)
	}

	// hope this is fine
	doc.Find("body").AppendHtml(script)

	// maybe no intermediate string?
	html, err := doc.Html()
	if err != nil {
		panic(err)
	}

	reader := bytes.NewReader([]byte(html))
	http.ServeContent(w, r, path, info.ModTime(), reader)
}

func ignore(name string) bool {
	return strings.HasPrefix(name, ".")
}

func watchPath(basePath string, fsWatcher *fsnotify.Watcher) {
	filepath.WalkDir(basePath, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}

		if ignore(d.Name()) {
			if d.IsDir() {
				return fs.SkipDir
			}
			return nil
		}

		if d.IsDir() {
			return fsWatcher.Add(path)
		}

		return nil
	})
}

func watcher(ch chan struct{}, staticPath string) {
	fsWatcher, err := fsnotify.NewWatcher()
	if err != nil {
		panic(err)
	}

	watchPath(staticPath, fsWatcher)
	log.Printf("Watching: %v\n", fsWatcher.WatchList())

	// some events fire twice probably due to text editor stuff
	// we are storing the events in a map and we will forget them
	// after at least minEventTTL
	// now except for events with delta < minEventTTL between them this is fine

	var ticker = time.NewTicker(minEventTTL)
	var seen = make(map[fsnotify.Event]struct{})

	for {
		select {
		case event := <-fsWatcher.Events:
			if _, met := seen[event]; met {
				ticker.Reset(minEventTTL)
				break
			}

			seen[event] = struct{}{}
			log.Println("New event from ", event.Name, " ", event.Op)

			if basename := filepath.Base(event.Name); ignore(basename) {
				// log.Println("ignoring " + event.Name)
				break
			}

			if event.Has(fsnotify.Create) {
				watchPath(event.Name, fsWatcher)
			}

			if event.Has(fsnotify.Rename) {
				fsWatcher.Remove(event.Name)
			}

			// log.Printf("Watching: %-v\n", fsWatcher.WatchList())

			ch <- struct{}{}
		case err := <-fsWatcher.Errors:
			panic(err)
		case <-ticker.C:
			for event := range seen {
				delete(seen, event)
			}
		}
	}
}

// FUTURE TODO: maybe send specific file that was changed
func reload(ch chan struct{}) {
	// could use a map
	var deadConnections []*websocket.Conn = make([]*websocket.Conn, 0, 4)

	for range ch {
		log.Println("Received Signal")
		log.Printf("Updating %d clients\n", len(connections))
		for _, conn := range connections {
			if err := conn.WriteMessage(websocket.TextMessage, []byte("reload")); err != nil {
				deadConnections = append(deadConnections, conn)
				log.Printf("Invalid connection: %v\n", conn)
			}
		}

		// brotha ew
		for _, conn := range deadConnections {
			idx := slices.Index(connections, conn)
			last := len(connections) - 1

			connections[idx] = connections[last]
			connections[last] = nil
			connections = connections[:last]
		}

		deadConnections = nil
	}
}

func serveWs(w http.ResponseWriter, r *http.Request) {
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		panic(err)
	}

	connections = append(connections, conn)
}

func main() {
	const staticPath string = "dev-static"
	const addr string = ":9090"

	http.Handle("/tinyreload.js", http.FileServer(http.Dir("./injectable")))
	http.Handle("/", NewTinyHandler(staticPath))
	http.HandleFunc("/ws", serveWs)

	ch := make(chan struct{}, 1)
	go watcher(ch, staticPath)
	go reload(ch)

	connections = make([]*websocket.Conn, 0, 8)

	log.Fatal(http.ListenAndServe(addr, nil))
}
