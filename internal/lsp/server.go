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

// HoverStateMode selects which state block(s) the hover view should show.
type HoverStateMode string

const (
	HoverStateModeBoth     HoverStateMode = "both"
	HoverStateModeAtCursor HoverStateMode = "atCursor"
	HoverStateModeLatest   HoverStateMode = "latest"
)

// parseHoverStateMode returns the canonical mode, defaulting to "both" for
// empty or unrecognised input.
func parseHoverStateMode(raw string) HoverStateMode {
	switch HoverStateMode(raw) {
	case HoverStateModeAtCursor:
		return HoverStateModeAtCursor
	case HoverStateModeLatest:
		return HoverStateModeLatest
	}
	return HoverStateModeBoth
}

// Server is the lore LSP server. glsp dispatches notifications one at a time
// on a single goroutine, so handler state (including `open`) is only touched
// serially — no locking required.
type Server struct {
	root    string // absolute filesystem path
	project *lore.Project
	index   *Index
	notify  glsp.NotifyFunc // set during initialize
	call    glsp.CallFunc   // set during initialize

	hoverStateMode HoverStateMode // set during initialize from initializationOptions

	open map[string]struct{} // project-relative paths with a live editor buffer
}

// NewServer creates a new LSP server.
func NewServer() *Server {
	return &Server{
		index:          NewIndex(),
		open:           make(map[string]struct{}),
		hoverStateMode: HoverStateModeBoth,
	}
}

// markOpen records that path is being edited in a client buffer.
func (s *Server) markOpen(path string) {
	s.open[path] = struct{}{}
}

// markClosed clears the open-buffer flag for path.
func (s *Server) markClosed(path string) {
	delete(s.open, path)
}

// isOpen reports whether path is currently being edited in a client buffer.
func (s *Server) isOpen(path string) bool {
	_, ok := s.open[path]
	return ok
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
	handler.WorkspaceDidChangeWatchedFiles = s.didChangeWatchedFiles
	return handler
}

func (s *Server) initialize(ctx *glsp.Context, params *protocol.InitializeParams) (any, error) {
	s.notify = ctx.Notify
	s.call = ctx.Call
	s.root = uriToPath(params.RootURI)
	s.hoverStateMode = hoverStateModeFromOptions(params.InitializationOptions)

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
	s.registerFileWatchers()
	return nil
}

func (s *Server) shutdown(_ *glsp.Context) error {
	return nil
}

// loadProject (re)reads the lore.toml and every on-disk file into the index.
// Called at initialize time and whenever lore.toml changes on disk. Any files
// currently open in the editor keep their buffer contents — the editor stays
// authoritative until didClose.
func (s *Server) loadProject() {
	if s.root == "" {
		return
	}
	project, err := lore.FindAndLoadFrom(s.root)
	if err != nil {
		s.logWarning("failed to load project: %v", err)
		return
	}

	// Snapshot open-buffer contents before we rebuild the index from disk.
	openContents := make(map[string]string, len(s.open))
	for rel := range s.open {
		if content, ok := s.index.Content(rel); ok {
			openContents[rel] = content
		}
	}

	s.project = project
	if err := s.index.LoadProject(project); err != nil {
		s.logWarning("failed to index project: %v", err)
		return
	}

	// Re-apply buffered content so the editor's view wins over disk.
	for rel, content := range openContents {
		s.index.SetFile(rel, content)
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

// hoverStateModeFromOptions reads lore.hover.stateMode (or hoverStateMode) out
// of LSP initializationOptions. Unknown or missing values fall back to "both".
func hoverStateModeFromOptions(opts any) HoverStateMode {
	m, ok := opts.(map[string]any)
	if !ok {
		return HoverStateModeBoth
	}
	if raw, ok := m["hoverStateMode"].(string); ok {
		return parseHoverStateMode(raw)
	}
	if hoverRaw, ok := m["hover"].(map[string]any); ok {
		if raw, ok := hoverRaw["stateMode"].(string); ok {
			return parseHoverStateMode(raw)
		}
	}
	return HoverStateModeBoth
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

// publishDiagnostics sends textDocument/publishDiagnostics for the given
// file, including every state issue that belongs to that file. It is a
// no-op if the server has no notify channel (e.g. before initialize).
func (s *Server) publishDiagnostics(path, uri string) {
	if s.notify == nil {
		return
	}
	world := s.index.World()
	items := []protocol.Diagnostic{}
	for _, ent := range world.Entities {
		for _, si := range ent.StateIssues {
			if si.Span.File != path {
				continue
			}
			items = append(items, toProtocolDiagnostic(si))
		}
	}
	s.notify(protocol.ServerTextDocumentPublishDiagnostics, &protocol.PublishDiagnosticsParams{
		URI:         uri,
		Diagnostics: items,
	})
}

func toProtocolDiagnostic(si lore.StateIssue) protocol.Diagnostic {
	sev := protocol.DiagnosticSeverityInformation
	switch si.Severity {
	case lore.SeverityWarning:
		sev = protocol.DiagnosticSeverityWarning
	case lore.SeverityError:
		sev = protocol.DiagnosticSeverityError
	}
	line := uint32(0)
	if si.Span.Line > 0 {
		line = uint32(si.Span.Line - 1)
	}
	source := "lore"
	return protocol.Diagnostic{
		Range: protocol.Range{
			Start: protocol.Position{Line: line, Character: uint32(si.Span.StartByte)},
			End:   protocol.Position{Line: line, Character: uint32(si.Span.EndByte)},
		},
		Severity: &sev,
		Source:   &source,
		Message:  si.Message,
	}
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

// renderHoverStateBlocks returns the markdown for the state section of a
// hover. In "both" mode with a cursor, cursor state is rendered with
// "(latest: …)" annotations inline for any fields/tags that diverge. In
// single-view modes it just shows the selected view as one block.
func renderHoverStateBlocks(ent *lore.Entity, cursorFile string, cursorLine int, mode HoverStateMode) string {
	showLatest := mode == HoverStateModeLatest || mode == HoverStateModeBoth
	showAt := (mode == HoverStateModeAtCursor || mode == HoverStateModeBoth) && cursorFile != ""

	if !showAt {
		if !showLatest {
			return ""
		}
		latest := lore.FormatStateBlock(ent.Tags, ent.Fields)
		if latest == "" {
			return ""
		}
		return "\n\n```\n" + latest + "\n```"
	}

	atTags, atFields, _ := lore.ResolveStateAt(ent.StateHistory, cursorFile, cursorLine)

	if mode == HoverStateModeAtCursor {
		atCursor := lore.FormatStateBlock(atTags, atFields)
		if atCursor == "" {
			return ""
		}
		return "\n\n```\n" + atCursor + "\n```"
	}

	merged := lore.FormatStateBlockMerged(atTags, ent.Tags, atFields, ent.Fields)
	if merged == "" {
		return ""
	}
	return "\n\n```\n" + merged + "\n```"
}

// formatEntityHover builds markdown hover content for an entity. The cursor
// (file, line) is used to compute the "state at cursor" block when mode
// requests it; pass an empty cursorFile to show only the latest state.
func formatEntityHover(ent *lore.Entity, cursorFile string, cursorLine int, mode HoverStateMode) string {
	var b strings.Builder
	if ent.Type != "" {
		fmt.Fprintf(&b, "**%s** (%s)", ent.Name, ent.Type)
	} else {
		fmt.Fprintf(&b, "**%s**", ent.Name)
	}
	if len(ent.Aliases) > 0 {
		fmt.Fprintf(&b, "\n\nAlso known as: %s", strings.Join(ent.Aliases, ", "))
	}
	b.WriteString(renderHoverStateBlocks(ent, cursorFile, cursorLine, mode))
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
