const tinyreloadPayloadSep = '\x1F';

function reloadCSS(file) {
    const links = document.querySelectorAll('link[rel="stylesheet"]');
    links.forEach(link => {
        if (link.href.includes(file)) {

            // is this safe?
            const newLink = link.cloneNode();
            newLink.href = link.href.split("?")[0] + "?v=" + Date.now(); // cache-busting
            link.parentNode.replaceChild(newLink, link);
        }
    });
}

document.addEventListener("DOMContentLoaded", () => {
    if(!window.__tinyreload) {
        window.__tinyreload = new WebSocket(`ws://${document.location.host}/ws`)
        console.log('Established WS connection!')
    }

    window.__tinyreload.onerror = (err) => console.error("WS error:", err);
    
    window.__tinyreload.onmessage = (e) => {
        const changes = e.data.split(tinyreloadPayloadSep)
        let reload = changes.some(c => !c.endsWith('.css'));

        if(reload) {
            window.location.reload();
            return;
        }
        
        changes.forEach(c => reloadCSS(c))
    }
})

window.addEventListener("beforeunload", () => {
    if(window.__tinyreload) {
        window.__tinyreload.close()
    }
});
