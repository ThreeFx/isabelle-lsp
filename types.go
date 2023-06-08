package main

type Position struct {
	Line      int `json:"line"`
	Character int `json:"character"`
}

type Diagnostic struct {
	//JsonRpc  string `json:"jsonrpc"` // must be "2.0"
	Source   string `json:"source,omitempty"`
	Severity int    `json:"severity,omitempty"`
	Range    Range  `json:"range"`
	Msg      string `json:"message"`
}

type Capabilities struct {
	DefinitionProvider        bool        `json:"definitionProvider"`
	HoverProvider             bool        `json:"hoverProvider"`
	CompletionProvider        interface{} `json:"completionProvider"`
	DocumentHighlightProvider bool        `json:"documentHighlightProvider"`
	CodeActionProvider        bool        `json:"codeActionProvider"`
	TextDocumentSync          int         `json:"textDocumentSync"`
}

type PublishDiagnostics struct {
	Uri         string       `json:"uri"`
	Diagnostics []Diagnostic `json:"diagnostics"`
}

type Range struct {
	Start Position `json:"start"`
	End   Position `json:"end"`
}

type PIDERange struct {
	Range [4]int `json:"range"`
}

type TextEdit struct {
	Range   Range  `json:"range"`
	NewText string `json:"newText"`
}

type WorkspaceEdit struct {
	Changes map[string][]TextEdit `json:"changes"`
}

type CodeAction struct {
	Title         string        `json:"title"`
	Kind          string        `json:"kind"`
	WorkspaceEdit WorkspaceEdit `json:"edit"`
}
