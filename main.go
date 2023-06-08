package main

import (
	"context"
	"encoding/json"
	"io"
	"log"
	"os"
	"os/exec"
	"regexp"
	"strings"
	"sync"

	"go.lsp.dev/jsonrpc2"
)

var (
	mu            = &sync.Mutex{}
	dynamicOutput string
	initID        jsonrpc2.ID
)

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
	dynamicOutput = ""
	log.Printf("starting Isabelle LSP: %v", strings.Join(append([]string{"isabelle"}, os.Args[1:]...), " "))
	cmd := exec.Command("isabelle", os.Args[1:]...)
	lspIn, err := cmd.StdinPipe()
	if err != nil {
		log.Fatal(err)
	}

	lspOut, err := cmd.StdoutPipe()
	if err != nil {
		log.Fatal(err)
	}

	go func() { cmd.Run() }()

	clientSide := stream{
		s: jsonrpc2.NewStream(stub{r: os.Stdin, w: os.Stdout}),
		l: &sync.Mutex{},
	}
	serverSide := stream{
		s: jsonrpc2.NewStream(stub{r: lspOut, w: lspIn}),
		l: &sync.Mutex{},
	}

	log.Printf("intercepting messages")
	go func() {
		for {
			interceptClientMessage(clientSide, serverSide)
		}
	}()

	for {
		interceptServerMessage(serverSide, clientSide)
	}
}

func interceptClientMessage(client, server stream) {
	msg, _, err := client.s.Read(context.TODO())
	if err != nil {
		log.Printf("error receiving message: %v", err)
	}

	switch msg.(type) {
	case *jsonrpc2.Call:
		c := msg.(*jsonrpc2.Call)
		switch c.Method() {
		case "textDocument/codeAction":
			handleCodeAction(client, c)
		case "initialize":
			initID = c.ID()
			server.s.Write(context.TODO(), msg)
		default:
			server.s.Write(context.TODO(), msg)
		}
	default:
		server.s.Write(context.TODO(), msg)
	}
}

const Proof = 0
const ProofOutline = 1

type fix struct {
	ty      int
	content string
}

func handleCodeAction(client stream, msg *jsonrpc2.Call) {
	var params struct {
		CodeActionContext struct {
			Diagnostics []Diagnostic `json:"diagnostics"`
		} `json:"context"`
		Range        Range `json:"range"`
		TextDocument struct {
			Uri string `json:"uri"`
		} `json:"textDocument"`
	}
	j, err := msg.Params().MarshalJSON()
	if err != nil {
		log.Fatal(err)
	}
	err = json.Unmarshal(j, &params)
	if err != nil {
		log.Fatal(err)
	}

	startc := 0
	endc := 0
	d := params.CodeActionContext.Diagnostics
	if len(d) > 0 {
		startc = d[0].Range.Start.Character
		endc = d[0].Range.End.Character
	}

	fixes := []fix{}
	mu.Lock()
	lines := strings.Split(dynamicOutput, "\n")
	for i := range lines {
		line := lines[i]
		if strings.Contains(line, "Try this: ") ||
			strings.Contains(line, "Found proof: ") {
			fixes = append(fixes, fix{ty: Proof, content: line})
		} else if strings.Contains(line, "Proof outline") {
			j := i + 1
			for lines[j] != "qed" {
				j += 1
			}
			fixes = append(fixes, fix{
				ty:      ProofOutline,
				content: strings.Join(lines[i+1:j+1], "\n"),
			})
			i = j + 1
		}
	}
	mu.Unlock()

	as := []CodeAction{}
	for _, fix := range fixes {
		switch fix.ty {
		case ProofOutline:
			ca := CodeAction{
				Title: "Insert proof outline",
				Kind:  "quickfix",
				WorkspaceEdit: WorkspaceEdit{
					Changes: map[string][]TextEdit{
						params.TextDocument.Uri: {
							{
								NewText: fix.content,
								Range: Range{
									Start: Position{
										Line:      params.Range.Start.Line + 1,
										Character: 0,
									},
									End: Position{
										Line:      params.Range.Start.Line + 1,
										Character: 0,
									},
								},
							},
						},
					},
				},
			}
			as = append(as, ca)

		case Proof:
			title := "Insert proof"
			parts := strings.Split(fix.content, ":")
			if len(parts) == 0 {
				continue
			}

			method := strings.TrimSpace(parts[0])
			methodRegexp := regexp.MustCompile(`"([0-9A-Za-z]*)"`)
			if methodRegexp.MatchString(method) {
				method := methodRegexp.FindStringSubmatch(method)[1]
				title += " by " + method
				parts = parts[1:]
			}

			// Skip the "Found proof:" or "Try this:" parts
			parts = parts[1:]

			proof := strings.TrimSpace(parts[0])

			timeRegexp := regexp.MustCompile(` \(([0-9.]* ms)\)$`)
			time := ""
			if timeRegexp.MatchString(proof) {
				time = timeRegexp.FindStringSubmatch(proof)[1]
				proof = timeRegexp.ReplaceAllString(proof, "")
			}

			if !strings.Contains(title, "by") {
				title += " " + proof
			}

			if time != "" {
				title += " (" + time + ")"
			}

			ca := CodeAction{
				Title: title,
				Kind:  "quickfix",
				WorkspaceEdit: WorkspaceEdit{
					Changes: map[string][]TextEdit{
						params.TextDocument.Uri: {
							{
								NewText: proof,
								Range: Range{
									Start: Position{
										Line:      params.Range.Start.Line,
										Character: startc,
									},
									End: Position{
										Line:      params.Range.Start.Line,
										Character: endc,
									},
								},
							},
						},
					},
				},
			}
			as = append(as, ca)
		}
	}

	resp, err := jsonrpc2.NewResponse(msg.ID(), as, nil)
	if err != nil {
		log.Fatal(err)
	}

	_, err = client.s.Write(context.TODO(), resp)
	if err != nil {
		log.Fatal(err)
	}
}

func interceptServerMessage(server, client stream) {
	msg, _, err := server.s.Read(context.TODO())
	if err != nil {
		log.Printf("error receiving message: %v", err)
	}
	switch msg.(type) {
	case *jsonrpc2.Notification:
		c := msg.(*jsonrpc2.Notification)
		switch c.Method() {
		case "PIDE/dynamic_output":
			saveDynamicOutput(c)
			client.s.Write(context.TODO(), msg)
		default:
			client.s.Write(context.TODO(), msg)
		}
	case *jsonrpc2.Response:
		r := msg.(*jsonrpc2.Response)
		switch r.ID() {
		case initID:
			adjustInitializationResponse(client, r)
		default:
			client.s.Write(context.TODO(), msg)
		}
	default:
		log.Printf("unknown message type %T", msg)
	}
}

func saveDynamicOutput(msg *jsonrpc2.Notification) {
	var params struct {
		Content string `json:"content"`
	}
	j, err := msg.Params().MarshalJSON()
	if err != nil {
		log.Fatal(err)
	}
	err = json.Unmarshal(j, &params)
	if err != nil {
		log.Fatal(err)
	}
	mu.Lock()
	dynamicOutput = params.Content
	mu.Unlock()
}

func adjustInitializationResponse(client stream, msg *jsonrpc2.Response) {
	log.Print("intercepted initial response")
	var result struct {
		Capabilities Capabilities `json:"capabilities"`
	}
	j, err := msg.Result().MarshalJSON()
	if err != nil {
		log.Fatal(err)
	}
	err = json.Unmarshal(j, &result)
	if err != nil {
		log.Fatal(err)
	}
	result.Capabilities.CodeActionProvider = true
	adjustedMsg, err := jsonrpc2.NewResponse(msg.ID(), result, nil)
	if err != nil {
		log.Fatal(err)
	}
	_, err = client.s.Write(context.TODO(), adjustedMsg)
	if err != nil {
		log.Fatal(err)
	}
}
