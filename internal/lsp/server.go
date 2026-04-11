package lsp

import (
	"fmt"
	"net/url"
	"path/filepath"
	"strings"

	"lore/internal/lore"

	"github.com/tliron/commonlog"
	"github.com/tliron/glsp"
	protocol "github.com/tliron/glsp/protocol_3_16"
	glspserver "github.com/tliron/glsp/server"
)

const serverName = "lore"

// Server is the lore LSP server.
type Server struct {
	root    string // absolute filesystem path
	project *lore.Project
	index   *Index
	notify  glsp.NotifyFunc // set during initialize
}

// NewServer creates a new LSP server.
func NewServer() *Server {
	return &Server{
		index: NewIndex(),
	}
}

// Run starts the server on stdin/stdout.
func (s *Server) Run() error {
	commonlog.Configure(0, nil) // suppress logging to stderr
	handler := s.newHandler()
	srv := glspserver.NewServer(handler, serverName, false)
	return srv.RunStdio()
}

func (s *Server) newHandler() *protocol.Handler {
	handler := &protocol.Handler{}
	handler.Initialize = s.initialize
	handler.Initialized = s.initialized
	handler.Shutdown = s.shutdown
	handler.TextDocumentDidOpen = s.didOpen
	handler.TextDocumentDidChange = s.didChange
	handler.TextDocumentDidSave = s.didSave
	handler.TextDocumentDidClose = s.didClose
	handler.TextDocumentHover = s.hover
	handler.TextDocumentDefinition = s.definition
	handler.TextDocumentReferences = s.references
	handler.TextDocumentCompletion = s.completion
	handler.TextDocumentSemanticTokensFull = s.semanticTokensFull
	return handler
}

func (s *Server) initialize(ctx *glsp.Context, params *protocol.InitializeParams) (any, error) {
	s.notify = ctx.Notify
	s.root = uriToPath(params.RootURI)

	s.logInfo("initialising with root: %s", s.root)
	s.loadProject()

	syncKind := protocol.TextDocumentSyncKindFull
	return protocol.InitializeResult{
		Capabilities: protocol.ServerCapabilities{
			TextDocumentSync: &protocol.TextDocumentSyncOptions{
				OpenClose: boolPtr(true),
				Change:    &syncKind,
				Save:      &protocol.True,
			},
			HoverProvider:      true,
			DefinitionProvider: true,
			ReferencesProvider: true,
			CompletionProvider: &protocol.CompletionOptions{},
			SemanticTokensProvider: &protocol.SemanticTokensOptions{
				Legend: semanticTokensLegend(),
				Full:   true,
			},
		},
		ServerInfo: &protocol.InitializeResultServerInfo{
			Name: serverName,
		},
	}, nil
}

func (s *Server) initialized(_ *glsp.Context, _ *protocol.InitializedParams) error {
	return nil
}

func (s *Server) shutdown(_ *glsp.Context) error {
	return nil
}

// loadProject (re)reads the lore.toml and every on-disk file into the index.
// Called once at initialize time; editor buffers arrive via didOpen/didChange.
func (s *Server) loadProject() {
	if s.root == "" {
		return
	}
	project, err := lore.FindAndLoadFrom(s.root)
	if err != nil {
		s.logWarning("failed to load project: %v", err)
		return
	}
	s.project = project
	if err := s.index.LoadProject(project); err != nil {
		s.logWarning("failed to index project: %v", err)
		return
	}
	world := s.index.World()
	s.logInfo("indexed %d entities from %d files", len(world.Entities), s.index.FileCount())
}

// world returns the current merged world for query handlers.
func (s *Server) world() *lore.World {
	return s.index.World()
}

// logInfo sends an informational message to the client's output channel.
func (s *Server) logInfo(format string, args ...any) {
	s.sendLog(protocol.MessageTypeInfo, format, args...)
}

// logWarning sends a warning message to the client's output channel.
func (s *Server) logWarning(format string, args ...any) {
	s.sendLog(protocol.MessageTypeWarning, format, args...)
}

func (s *Server) sendLog(level protocol.MessageType, format string, args ...any) {
	if s.notify == nil {
		return
	}
	s.notify(string(protocol.ServerWindowLogMessage), protocol.LogMessageParams{
		Type:    level,
		Message: fmt.Sprintf(format, args...),
	})
}

// fileToURI converts a project-relative path to a file:// URI.
func (s *Server) fileToURI(relPath string) string {
	abs := filepath.Join(s.root, relPath)
	return "file://" + abs
}

// uriToRelPath converts a file:// URI to a project-relative path using
// forward slashes (matching fs.FS conventions).
func (s *Server) uriToRelPath(uri string) string {
	abs := uriToPath(&uri)
	rel, err := filepath.Rel(s.root, abs)
	if err != nil {
		return abs
	}
	return filepath.ToSlash(rel)
}

// getLine returns the line at the given 0-based index from a document URI.
func (s *Server) getLine(uri string, line uint32) string {
	content := s.getDocumentContent(uri)
	lines := strings.Split(content, "\n")
	if int(line) >= len(lines) {
		return ""
	}
	return lines[line]
}

// getDocumentContent returns the current content for a URI straight from
// the index (which is the authoritative store for both open buffers and
// on-disk files).
func (s *Server) getDocumentContent(uri string) string {
	content, _ := s.index.Content(s.uriToRelPath(uri))
	return content
}

func uriToPath(uri *string) string {
	if uri == nil {
		return ""
	}
	u, err := url.Parse(*uri)
	if err != nil {
		// Best effort: strip the scheme.
		return strings.TrimPrefix(*uri, "file://")
	}
	return u.Path
}

func boolPtr(b bool) *bool { return &b }

func lineRange(line int) protocol.Range {
	l := uint32(line - 1) // lore uses 1-based, LSP uses 0-based
	return protocol.Range{
		Start: protocol.Position{Line: l, Character: 0},
		End:   protocol.Position{Line: l, Character: 0},
	}
}

func ptrStr(s string) *string { return &s }

func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}

// formatEntityHover builds markdown hover content for an entity.
func formatEntityHover(ent *lore.Entity) string {
	var b strings.Builder
	if ent.Type != "" {
		fmt.Fprintf(&b, "**%s** (%s)", ent.Name, ent.Type)
	} else {
		fmt.Fprintf(&b, "**%s**", ent.Name)
	}
	if len(ent.Aliases) > 0 {
		fmt.Fprintf(&b, "\n\nAlso known as: %s", strings.Join(ent.Aliases, ", "))
	}
	if len(ent.Descriptions) > 0 {
		b.WriteString("\n\n---\n\n")
		texts := make([]string, len(ent.Descriptions))
		for i, d := range ent.Descriptions {
			texts[i] = d.Text
		}
		b.WriteString(truncate(strings.Join(texts, "\n\n"), 2000))
	}
	return b.String()
}
