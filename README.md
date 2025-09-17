## tinyreload

Tiny live-reload server for static sites. One small binary. No tooling, no config.

### Setup
after you clone the repository run
```bash
./setup.sh
```

### Run
```bash
tinyreload -path path/to/static -addr :9090
# then open http://localhost:9090
```

### How it works
- Serves files from `-path`
- Injects `<script src="/tinyreload.js"></script>` into `.html`
- Watches the directory; on change sends a websocket "reload"
- Browser reloads the page

### Flags
- `-path` (default `./`): directory to serve
- `-addr` (default `:9090`): listen address

### Notes
- Sets no-cache headers for HTML
- Recursively watches; ignores dotfiles
- Expects `index.html` for directory routes

## TODO
- Add HMR

