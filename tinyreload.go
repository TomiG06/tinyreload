package main

import (
	"embed"
	"flag"
	"fmt"
	"io/fs"
	"log"
	"mime"
	"net/http"
	"os"
	pathlib "path"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/PuerkitoBio/goquery"
	"github.com/fsnotify/fsnotify"
	"github.com/gorilla/websocket"
)

const (
	script       = `<script src="/tinyreload.js"></script>`
	debounceTime = 100 * time.Millisecond
	sep          = "\x1F"
)

const (
	Red    = "\033[31m"
	Green  = "\033[32m"
	Yellow = "\033[33m"
	Blue   = "\033[34m"
	Cyan   = "\033[38;2;0;173;216m"
	Reset  = "\033[0m"
)

const banner string = `
████████╗██╗███╗   ██╗██╗   ██╗██████╗ ███████╗██╗      ██████╗  █████╗ ██████╗ 
╚══██╔══╝██║████╗  ██║╚██╗ ██╔╝██╔══██╗██╔════╝██║     ██╔═══██╗██╔══██╗██╔══██╗
   ██║   ██║██╔██╗ ██║ ╚████╔╝ ██████╔╝█████╗  ██║     ██║   ██║███████║██║  ██║
   ██║   ██║██║╚██╗██║  ╚██╔╝  ██╔══██╗██╔══╝  ██║     ██║   ██║██╔══██║██║  ██║
   ██║   ██║██║ ╚████║   ██║   ██║  ██║███████╗███████╗╚██████╔╝██║  ██║██████╔╝
   ╚═╝   ╚═╝╚═╝  ╚═══╝   ╚═╝   ╚═╝  ╚═╝╚══════╝╚══════╝ ╚═════╝ ╚═╝  ╚═╝╚═════╝ 
                                                                                
`

//go:embed injectable/tinyreload.js
var injectable embed.FS

var upgrader = websocket.Upgrader{
	ReadBufferSize:  16,
	WriteBufferSize: 16,
}

func prefix(color, name string) string {
	return fmt.Sprintf("%s%-*s%s", color, 15, name, Reset)
}

var (
	mainLog   = log.New(os.Stdout, prefix(Cyan, "[TINYRELOAD]"), log.LstdFlags)
	serverLog = log.New(os.Stdout, prefix(Green, "[HTTP]"), log.LstdFlags)
	fsLog     = log.New(os.Stdout, prefix(Blue, "[FS]"), log.LstdFlags)
	wsLog     = log.New(os.Stdout, prefix(Yellow, "[WS]"), log.LstdFlags)
	// errLog    = log.New(os.Stderr, Red+"[ERROR] "+Reset, log.LstdFlags)
)

var connections SafeConnections

type SafeConnections struct {
	mu          sync.Mutex
	connections []*websocket.Conn
}

func (sc *SafeConnections) Set(newConnections []*websocket.Conn) {
	sc.mu.Lock()
	defer sc.mu.Unlock()
	sc.connections = newConnections
}

func (sc *SafeConnections) Len() int {
	sc.mu.Lock()
	defer sc.mu.Unlock()
	return len(sc.connections)
}

func (sc *SafeConnections) Broadcast(msg []byte) int {
	var successful int = 0
	for _, c := range sc.connections {
		if err := c.WriteMessage(websocket.TextMessage, msg); err == nil {
			successful++
		}
	}
	return successful
}

type TinyHandler struct {
	root string
}

func NewTinyHandler(root string) *TinyHandler {
	return &TinyHandler{root}
}

func (th *TinyHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	cleanPath := filepath.Clean(r.URL.Path)

	absRoot, _ := filepath.Abs(th.root)
	path := filepath.Join(th.root, cleanPath)
	absPath, _ := filepath.Abs(path)

	if !strings.HasPrefix(absPath, absRoot) {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}

	info, err := os.Stat(path)
	if err != nil {
		http.NotFound(w, r)
		return
	}

	if info.IsDir() {
		// Redirect to index.html
		redirectURL := pathlib.Join(r.URL.Path, "index.html")
		http.Redirect(w, r, redirectURL, http.StatusMovedPermanently)
		return
	}

	serverLog.Println("GET " + cleanPath)

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

	html, err := doc.Html()
	if err != nil {
		panic(err)
	}

	reader := strings.NewReader(html)
	http.ServeContent(w, r, path, info.ModTime(), reader)
}

// FUTURE: can use something like go-dotignore and support ignore configs

//go:inline
func ignore(name string) bool {
	return name != "." && strings.HasPrefix(name, ".")
}

func PathToURL(root string, path string) string {
	absRoot, err := filepath.Abs(root)
	if err != nil {
		return ""
	}
	absPath, err := filepath.Abs(path)
	if err != nil {
		return ""
	}

	rel, err := filepath.Rel(absRoot, absPath)
	if err != nil {
		return ""
	}

	// this should make it fs independent
	urlPath := "/" + strings.ReplaceAll(rel, string(filepath.Separator), "/")
	return urlPath
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

func watcher(ch chan string, staticPath string) {
	fsWatcher, err := fsnotify.NewWatcher()
	if err != nil {
		panic(err)
	}

	watchPath(staticPath, fsWatcher)
	fsLog.Printf("Watching: %v\n", fsWatcher.WatchList())

	// some events fire twice probably due to text editor stuff
	// we are storing the events in a map and we will forget them
	// after at least debounceTime

	var ticker = time.NewTicker(debounceTime)
	var recordedEvents = make(map[string]struct{})
	var names = make([]string, 0, 4)

	for {
		select {
		case event := <-fsWatcher.Events:
			if _, met := recordedEvents[event.Name]; met {
				ticker.Reset(debounceTime)
				break
			}

			if ignore(filepath.Base(event.Name)) {
				continue
			}

			fsLog.Println("Event: ", event.Name, " ", event.Op)
			recordedEvents[event.Name] = struct{}{}

			if event.Has(fsnotify.Create) {
				watchPath(event.Name, fsWatcher)
			}

			if event.Has(fsnotify.Rename) {
				fsWatcher.Remove(event.Name)
			}

			fsLog.Printf("Watching: %-v\n", fsWatcher.WatchList())

		case err := <-fsWatcher.Errors:
			fsLog.Fatal(err)

		case <-ticker.C:
			// it probably wont triger with multiple files but whatever
			if len(recordedEvents) == 0 {
				break
			}

			for name := range recordedEvents {
				names = append(names, PathToURL(staticPath, name))
				delete(recordedEvents, name)
			}
			payload := strings.Join(names, sep)
			ch <- payload
			names = names[:0]
		}
	}
}

//go:inline
func pathReloads(path string) bool {
	return !strings.HasSuffix(path, ".css")
}

// FUTURE: maybe send specific file that was changed (HMR)
func reload(ch chan string) {
	for p := range ch {
		//broadcast p(ath) and let the client decide what to do (reload, hmr)
		n := connections.Broadcast([]byte(p))

		// reload should close the connection so there is no point to remember it
		if pathReloads(p) {
			connections.Set(nil)
		}

		wsLog.Printf("Broadcasted %s to %d clients\n", p, n)
	}
}

func serveWs(w http.ResponseWriter, r *http.Request) {
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		panic(err)
	}
	wsLog.Println("connection established")
	connections.Set(append(connections.connections, conn))
}

func main() {
	staticPath := flag.String("path", "./", "path to watch for changes")
	addr := flag.String("addr", ":9090", "server listen address")

	flag.Parse()

	info, err := os.Stat(*staticPath)
	if os.IsNotExist(err) {
		log.Fatalf("directory %s does not exist", *staticPath)
	}

	if !info.IsDir() {
		log.Fatalf("can not serve non-directory: %s", *staticPath)
	}

	f, err := os.Open(*staticPath)
	if err != nil {
		log.Fatalf("cannot open directory %s", *staticPath)
	}
	f.Close()

	injectableData, err := injectable.ReadFile("injectable/tinyreload.js")
	if err != nil {
		mainLog.Fatal("failed to read injectable file")
	}

	fmt.Print(Cyan + banner + Reset)

	http.HandleFunc("/tinyreload.js", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/javascript")
		w.Write(injectableData)
	})
	http.Handle("/", NewTinyHandler(*staticPath))
	http.HandleFunc("/ws", serveWs)

	ch := make(chan string)
	go watcher(ch, *staticPath)
	go reload(ch)

	connections = SafeConnections{sync.Mutex{}, make([]*websocket.Conn, 0, 8)}

	mainLog.Printf("listening on http://localhost%s\n", *addr)
	mainLog.Fatal(http.ListenAndServe(*addr, nil))
}
