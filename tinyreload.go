package main

import (
	"io/fs"
	"log"
	"mime"
	"net/http"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/fsnotify/fsnotify"
	"github.com/gorilla/websocket"
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
	log.Println(path)
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

	log.Println("serving " + path)

	ext := filepath.Ext(path)
	if mimeType := mime.TypeByExtension(ext); mimeType != "" {
		w.Header().Set("Content-Type", mimeType)
	} else {
		w.Header().Set("Content-Type", "application/octet-stream")
	}

	log.Println("MIME: " + mime.TypeByExtension(ext))

	if ext != ".html" {
		http.ServeFile(w, r, path)
		return
	}

	http.ServeFile(w, r, path)

	// inject script
}

// add ignore pattern
func watchPath(basePath string, fsWatcher *fsnotify.Watcher) {
	filepath.WalkDir(basePath, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}

		log.Println("Visited: " + path)
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
	log.Printf("Watching: %-v\n", fsWatcher.WatchList())

	for {
		select {
		// event fires twice probably from text editor stuff
		// add (Name, Op) array, check unique and clean after a ticker triggers
		case event := <-fsWatcher.Events:
			log.Println("New event from ", event.Name, " ", event.Op)

			if event.Has(fsnotify.Create) {
				watchPath(event.Name, fsWatcher)
			}

			if event.Has(fsnotify.Rename) {
				fsWatcher.Remove(event.Name)
			}

			log.Printf("Watching: %-v\n", fsWatcher.WatchList())

			ch <- struct{}{}
		case err := <-fsWatcher.Errors:
			panic(err)
		}
	}
}

func reload(ch chan struct{}) {
	// could use a map
	var deadConnections []*websocket.Conn = make([]*websocket.Conn, 0, 4)

	for range ch {
		log.Println("Received Signal")
		log.Printf("Updating %d clients\n", len(connections))
		for _, conn := range connections {
			if err := conn.WriteMessage(websocket.TextMessage, []byte("reload")); err != nil {
				deadConnections = append(deadConnections, conn)
				log.Printf("Invalid connection: %-v\n", conn)
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

// TODO: refuse cross origin
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
