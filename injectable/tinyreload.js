document.addEventListener("DOMContentLoaded", () => {
    if(!window.__tinyreload) {
        window.__tinyreload = new WebSocket(`ws://${document.location.host}/ws`)
        console.log('Established WS connection!')
    }
    
    window.__tinyreload.onmessage = (e) => {
        console.log(e.data)
        if(e.data != "reload") {
            return
        }

        window.location.reload()
    }
})

window.addEventListener("beforeunload", () => {
    if(window.__tinyreload) {
        window.__tinyreload.close()
    }
});