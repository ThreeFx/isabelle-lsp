package main

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log"
	"os"
	"os/exec"
	"sync"

	"github.com/neovim/go-client/nvim"
	"go.lsp.dev/jsonrpc2"
)

var n *nvim.Nvim
var sb nvim.Buffer
var wd nvim.Window

type stub struct {
	r io.Reader
	w io.Writer
}

func (s stub) Read(p []byte) (int, error) {
	return s.r.Read(p)
}

func (s stub) Write(p []byte) (int, error) {
	return s.w.Write(p)
}

func (s stub) Close() error {
	var err1, err2 error
	if c, ok := s.r.(io.Closer); ok {
		err1 = c.Close()
	}

	if c, ok := s.w.(io.Closer); ok {
		err2 = c.Close()
	}

	if err1 != nil {
		return err1
	}
	return err2
}

type stream struct {
	s jsonrpc2.Stream
	l *sync.Mutex
}

func (s stream) Read(ctx context.Context) (jsonrpc2.Message, int64, error) {
	return s.s.Read(ctx)
}

func (s stream) Write(ctx context.Context, msg jsonrpc2.Message) (int64, error) {
	s.l.Lock()
	defer s.l.Unlock()
	return s.s.Write(ctx, msg)
}

func main() {
	args := os.Args
	if len(args) < 2 {
		log.Fatalf("expected argument: %s <nvim-socket>", os.Args[0])
	}
	socket := args[1]

	var err error
	n, err = nvim.Dial(socket)
	if err != nil {
		log.Fatal(err)
	}

	f, err := os.OpenFile("/tmp/loggggg", os.O_RDWR|os.O_CREATE|os.O_APPEND, 0640)
	if err != nil {
		log.Fatal(err)
	}
	log.SetOutput(f)
	g, err := os.OpenFile("/tmp/logggggg", os.O_RDWR|os.O_CREATE|os.O_APPEND, 0640)
	if err != nil {
		log.Fatal(err)
	}

	cmd := exec.Command("isabelle", "vscode_server", "-v", "-L", "/tmp/go-isa-lsp")
	lspin, err := cmd.StdinPipe()
	if err != nil {
		log.Fatal(err)
	}

	lspout, err := cmd.StdoutPipe()
	if err != nil {
		log.Fatal(err)
	}

	go func() { cmd.Run() }()

	server := stream{
		s: jsonrpc2.NewStream(stub{r: os.Stdin, w: os.Stdout}),
		l: &sync.Mutex{},
	}
	client := stream{
		s: jsonrpc2.NewStream(stub{r: lspout, w: io.MultiWriter(lspin, g)}),
		l: &sync.Mutex{},
	}

	handleCommands(server, client)
}

func periodicUpdateCaret(s stream) {
	b, err := n.CurrentBuffer()
	if err != nil {
		log.Fatal(err)
	}
	name, err := n.BufferName(b)
	if err != nil {
		log.Fatal(err)
	}
	pos, err := n.WindowCursor(0)
	if err != nil {
		log.Fatal(err)
	}
	var ft string
	err = n.BufferOption(b, "filetype", &ft)
	if err != nil {
		log.Fatal(err)
	}
	if ft != "isabelle" {
		return
	}
	sendUpdateCaret(s, pos[0]-3, pos[1], name)
}

func sendUpdateCaret(s stream, line, col int, uri string) {
	type params struct {
		Uri  string `json:"uri"`
		Line int    `json:"line"`
		Col  int    `json:"character"`
	}
	n, err := jsonrpc2.NewNotification("PIDE/caret_update", params{Uri: uri, Line: line, Col: col})
	if err != nil {
		log.Fatal(err)
	}

	log.Print("writing caret update")
	_, err = s.s.Write(context.TODO(), n)
	if err != nil {
		log.Fatal(err)
	}
}

func toIsabelleHandler(s stream) func(nvim.Buffer, ...interface{}) {
	return func(b nvim.Buffer, args ...interface{}) {
		name, err := n.BufferName(b)
		if err != nil {
			log.Fatal(err)
		}
		// get pos of last edit (good enough) (for now)
		pos, err := n.BufferMark(b, ".")
		if err != nil {
			log.Fatal(err)
		}
		log.Println("buffer: ", name)
		log.Println("cursor: ", pos)
		line := pos[0] - 1 // isabelle vscode_server is 0-indexed, plus 2 extra for other stuff
		col := pos[1]
		sendUpdateCaret(s, line, col, "file://"+name)
	}
}

func proxyToIsabelleHandler(next proxy) proxy {
	return func(to stream, msg jsonrpc2.Message) {
		switch msg.(type) {
		case *jsonrpc2.Notification:
			next(to, msg)
		case *jsonrpc2.Call:
			c := msg.(*jsonrpc2.Call)
			switch c.Method() {
			case "textDocument/hover":
				next(to, msg)
				log.Println("got hover")
				var params struct {
					Position struct {
						Line int `json:"line"`
						Character int `json:"character"`
					} `json:"position"`
					TextDocument struct {
						Uri string `json:"uri"`
					}
				}
				j, err := c.Params().MarshalJSON()
				if err != nil {
					log.Fatal(err)
				}
				err = json.Unmarshal(j, &params)
				if err != nil {
					log.Fatal(err)
				}
				sendUpdateCaret(to, params.Position.Line, params.Position.Character, params.TextDocument.Uri)
			default:
				next(to, msg)
			}
		case *jsonrpc2.Response:
			next(to, msg)
		default:
			log.Printf("unknown message type %T", msg)
		}
	}
}

func proxyIsabelleHandler(next proxy) proxy {
	return func(to stream, msg jsonrpc2.Message) {
		switch msg.(type) {
		case *jsonrpc2.Notification:
			c := msg.(*jsonrpc2.Notification)
			switch c.Method() {
			case "PIDE/dynamic_output":
				log.Printf("got dynamic output")
				// write into scratch buffer
				var params struct {
					Content string `json:"content"`
				}
				j, err := c.Params().MarshalJSON()
				if err != nil {
					log.Fatal(err)
				}
				err = json.Unmarshal(j, &params)
				if err != nil {
					log.Fatal(err)
				}
				err = n.SetBufferLines(sb, 0, -1, false, bytes.Split([]byte(params.Content), []byte{'\n'}))
				if err != nil {
					log.Fatal(err)
				}
			default:
				next(to, msg)
			}
		case *jsonrpc2.Call:
			next(to, msg)
		case *jsonrpc2.Response:
			next(to, msg)
		default:
			log.Printf("unknown message type %T", msg)
		}
	}
}

func findOrCreateScratchBuf() nvim.Buffer {
	bs, err := n.Buffers()
	if err != nil {
		log.Fatal(err)
	}

	for _, b := range bs {
		name, err := n.BufferName(b)
		if err != nil {
			log.Fatal(err)
		}

		if name == "isabelle-output" {
			return b
		}
	}

	b, err := n.CreateBuffer(false, true)
	if err != nil {
		log.Fatal(err)
	}

	err = n.SetBufferOption(b, "textwidth", 60)
	if err != nil {
		log.Fatal(err)
	}

	n.SetBufferName(b, "isabelle-output")
	return b
}

func createWindow() nvim.Window {
	w, err := n.CurrentWindow()
	if err != nil {
		log.Fatal(err)
	}

	height, err := n.WindowHeight(w)
	if err != nil {
		log.Fatal(err)
	}
	width, err := n.WindowWidth(w)
	if err != nil {
		log.Fatal(err)
	}

	log.Printf("width: %d", width)

	var opts *nvim.WindowConfig
	//if width > height {
	opts = &nvim.WindowConfig{
		Anchor:   "NW",
		Relative: "win",
		Height:   height - 2,
		Width:    62,
		Row:      0,
		Col:      float64(width) - 70,
	}
	//} else {
	//	*opts = nvim.WindowConfig{
	//		Relative: "win",
	//		Height: 15,
	//		Width: 10,
	//	}
	//}

	wd, err := n.OpenWindow(sb, false, opts)
	if err != nil {
		log.Fatal(err)
	}

	return wd
}

func handleCommands(vim, isa stream) {
	sb = findOrCreateScratchBuf()
	wd = createWindow()
	log.Println("created temp buffer")

	// TODO: attach to buffer create somehow
	b, err := n.CurrentBuffer()
	if err != nil {
		log.Fatal(err)
	}

	err = n.RegisterHandler("nvim_buf_lines_event", toIsabelleHandler(isa))
	if err != nil {
		log.Fatal(err)
	}
	err = n.RegisterHandler("nvim_buf_changedtick_event", toIsabelleHandler(isa))
	if err != nil {
		log.Fatal(err)
	}
	err = n.RegisterHandler("CursorMoved_event", toIsabelleHandler(isa))
	if err != nil {
		log.Fatal(err)
	}
	err = n.Subscribe("CursorMoved")
	if err != nil {
		log.Fatal(err)
	}
	ok, err := n.AttachBuffer(b, true, map[string]interface{}{})
	if !ok || err != nil {
		log.Print(ok)
		log.Fatal(err)
	}
	log.Print("nvim setup done")

	go runProxy(vim, isa, proxyToIsabelleHandler(proxyLSP))
	runProxy(isa, vim, proxyIsabelleHandler(proxyLSP))
}

type proxy func(stream, jsonrpc2.Message)

func proxyLSP(to stream, msg jsonrpc2.Message) {
	_, err := to.s.Write(context.TODO(), msg)
	if err != nil {
		log.Fatal(err)
	}
}

func runProxy(from, to stream, p proxy) {
	for {
		msg, _, err := from.s.Read(context.TODO())
		if err != nil {
			log.Fatal(err)
		}
		p(to, msg)
	}
}
