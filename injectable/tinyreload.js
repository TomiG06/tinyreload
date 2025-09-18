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
    
    window.__tinyreload.onmessage = (e) => {
        if(e.data.endsWith('.css')) {
            reloadCSS(e.data);
            return;
        }

        window.location.reload()
    }
})

window.addEventListener("beforeunload", () => {
    if(window.__tinyreload) {
        window.__tinyreload.close()
    }
});