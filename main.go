package main

import (
	"fmt"
	"io"
	"log"
	"os"
	"os/exec"

	"github.com/neovim/go-client/nvim"
)

type lspProxy struct{}

var n *nvim.Nvim

func main() {
	var err error
	n, err = nvim.Dial("/tmp/nvim-socket")
	if err != nil {
		log.Fatal(err)
	}

	f, err := os.OpenFile("/tmp/loggggg", os.O_RDWR|os.O_CREATE|os.O_APPEND, 0640)
	if err != nil {
		log.Fatal(err)
	}
	log.SetOutput(f)

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

	in := os.Stdin
	out := os.Stdout
	handleCommands(in, out, lspin, lspout)
}

func handler(lspin io.Writer) func(nvim.Buffer, ...interface{}) {
	return func(b nvim.Buffer, args...interface{}) {
		log.Printf("handler args: %v %+v", b, args)
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
		line := pos[0] - 1 // isabelle vscode_server is 0-indexed
		col := pos[1]

		req := fmt.Sprintf(`{"jsonrpc":"2.0","method":"PIDE/caret_update","params":{"uri":"file://%s","line":%d,"character":%d}}`, name, line, col)
		content := []byte(fmt.Sprintf("Content-Length: %d\n\n%s", len(req), req))
		_, err = lspin.Write(content)
		if err != nil {
			log.Fatal(err)
		}
	}
}

func handleCommands(in io.Reader, out io.Writer, lspin io.Writer, lspout io.Reader) {
	req := fmt.Sprintf(`{"jsonrpc":"2.0","method":"PIDE/symbols_request"}`)
	content := []byte(fmt.Sprintf("Content-Length: %d\n\n%s", len(req), req))
	_, err := lspin.Write(content)
	if err != nil {
		log.Fatal(err)
	}

	// TODO: attach to buffer create somehow
	b, err := n.CurrentBuffer()
	if err != nil {
		log.Fatal(err)
	}

	err = n.RegisterHandler("nvim_buf_lines_event", handler(lspin))
	if err != nil {
		log.Fatal(err)
	}
	err = n.RegisterHandler("nvim_buf_changedtick_event", handler(lspin))
	if err != nil {
		log.Fatal(err)
	}
	ok, err := n.AttachBuffer(b, true, map[string]interface{}{})
	if !ok || err != nil {
		log.Print(ok)
		log.Fatal(err)
	}

	go proxyLSP(lspin, in)
	proxyLSP(out, lspout)
}

func proxyLSP(w io.Writer, r io.Reader) {
	io.Copy(w, r)
}
