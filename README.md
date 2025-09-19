## tinyreload

Tiny live-reload server for static sites. One small binary. No tooling, no config.

### Setup
After you clone the repository run:
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
- Watches the directory; on change sends file URL via websocket
- **CSS files**: Hot-reload without page refresh
- **Other files**: Full page reload

### Flags
- `-path` (default `./`): directory to serve
- `-addr` (default `:9090`): listen address

### Features
- **Hot CSS reload**: CSS changes update without page refresh
- **Embedded assets**: JavaScript bundled into binary (no external files)
- **Smart reloading**: Different behavior for CSS vs other files
- **Directory redirects**: `/path/` → `/path/index.html`
- **No-cache headers**: Prevents browser caching of HTML
- **Recursive watching**: Monitors all subdirectories; ignores dotfiles

### NOTES:
- If you are in VIM it has some problems with not ignoring some internal vim files 

## Maybe in the future
- Add HMR for JavaScript modules
- Add configurable ignore patterns

