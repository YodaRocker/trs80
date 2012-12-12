package main

import (
	"bufio"
	"code.google.com/p/go.net/websocket"
	"fmt"
	"headcode.com/webutil"
	"log"
	"net/http"
	"time"
)

func generateIndex(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html")
	bw := bufio.NewWriter(w)
	fmt.Fprint(bw, `<!DOCTYPE html>
<html>
    <head>
        <title>TRS-80 Model III Emulator</title>
        <script src="static/jquery-1.8.2.min.js"></script>
        <script src="static/home.js"></script>
        <link rel="stylesheet" href="static/home.css"/>
        <link rel="stylesheet" href="font.css"/>
	</head>
	<body>
	</body>
</html>`)
	bw.Flush()
}

func generateFontCss(w http.ResponseWriter, r *http.Request) {
    // Image is 512x480
    // 10 rows of glyphs, but last two are different page.
    // Use first 8 rows.
    // 32 chars across (32*8 = 256)
    // For thin font:
    //     256px wide.
    //     Chars are 8px wide (256/32 = 8)
    //     Chars are 24px high (480/2/10 = 24), with doubled rows.
	w.Header().Set("Content-Type", "text/css")
	bw := bufio.NewWriter(w)
	fmt.Fprint(bw, `.char {
		display: inline-block;
		width: 8px;
		height: 24px;
		background-image: url("static/TRS80CharacterGen.png");
		background-position: -248px -24px; /* ? = 31*8, 1*24 */
		background-repeat: no-repeat;
}
`)
	for ch := 0; ch < 256; ch++ {
		fmt.Fprintf(bw, ".char-%d { background-position: %dpx %dpx; }\n",
			ch, -(ch % 32)*8, -(ch / 32)*24)
	}
	bw.Flush()
}

func homeHandler(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path == "/" {
		generateIndex(w, r)
	} else if r.URL.Path == "/font.css" {
		generateFontCss(w, r)
	} else {
		http.NotFound(w, r)
	}
}

func updatesWsHandler(ws *websocket.Conn, updateCmdCh chan<- interface{}) {
	log.Printf("updatesWsHandler")
	updateCh := make(chan cpuUpdate)
	updateCmdCh <- startUpdates{updateCh}

	for update := range updateCh {
		err := websocket.JSON.Send(ws, update)
		if err != nil {
			log.Printf("websocket.JSON.Send: %s", err)
			break
		}
	}

	updateCmdCh <- stopUpdates{}
}

type Timeouter interface {
	Timeout() bool
}

func commandsWsHandler(ws *websocket.Conn, cpuCmdCh chan<- cpuCommand) {
	log.Printf("commandsWsHandler")

	for {
		var message cpuCommand
		err := websocket.JSON.Receive(ws, &message)
		if err != nil {
			timeoutErr, ok := err.(Timeouter)
			if ok && timeoutErr.Timeout() {
				continue
			}
			log.Printf("websocket.JSON.Receive: %s", err)
			break
		}
		cpuCmdCh <- message
	}
}

func serveWebsite(updateCmdCh chan<- interface{}, cpuCmdCh chan<- cpuCommand) {
	port := 8080

	// Create handlers.
	handlers := http.NewServeMux()
	handlers.Handle("/", webutil.GetHandler(http.HandlerFunc(homeHandler)))
	handlers.Handle("/updates.ws", websocket.Handler(func(ws *websocket.Conn) {
		updatesWsHandler(ws, updateCmdCh)
	}))
	handlers.Handle("/commands.ws", websocket.Handler(func(ws *websocket.Conn) {
		commandsWsHandler(ws, cpuCmdCh)
	}))
	handlers.Handle("/static/", http.StripPrefix("/static/",
		http.FileServer(http.Dir("static"))))

	// Create server.
	address := fmt.Sprintf(":%d", port)
	server := http.Server{
		Addr:           address,
		Handler:        webutil.LoggingHandler(handlers),
		ReadTimeout:    10 * time.Second,
		WriteTimeout:   10 * time.Second,
		MaxHeaderBytes: http.DefaultMaxHeaderBytes,
	}

	// Start serving.
	log.Printf("Serving website on %s", address)
	err := server.ListenAndServe()
	if err != nil {
		log.Fatal(err)
	}
}