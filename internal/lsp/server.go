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

	hoverStateMode           HoverStateMode // set during initialize from initializationOptions
	hoverShowStateDirectives bool           // set during initialize from initializationOptions
	palette                  []string       // hex colours indexed 0..paletteSize-1, from initializationOptions; empty disables hover colouring

	open map[string]struct{} // project-relative paths with a live editor buffer
}

// NewServer creates a new LSP server.
func NewServer() *Server {
	return &Server{
		index:                    NewIndex(),
		open:                     make(map[string]struct{}),
		hoverStateMode:           HoverStateModeBoth,
		hoverShowStateDirectives: false,
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
	wrapped := &loreHandler{inner: handler, server: s}
	srv := glspserver.NewServer(wrapped, serverName, false)
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
	handler.WorkspaceSymbol = s.workspaceSymbol
	handler.WorkspaceDidChangeWatchedFiles = s.didChangeWatchedFiles
	return handler
}

func (s *Server) initialize(ctx *glsp.Context, params *protocol.InitializeParams) (any, error) {
	s.notify = ctx.Notify
	s.call = ctx.Call
	s.root = uriToPath(params.RootURI)
	s.hoverStateMode = hoverStateModeFromOptions(params.InitializationOptions)
	s.hoverShowStateDirectives = hoverShowStateDirectivesFromOptions(params.InitializationOptions)
	s.palette = paletteFromOptions(params.InitializationOptions)

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
			WorkspaceSymbolProvider: true,
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

// hoverShowStateDirectivesFromOptions reads lore.hover.showStateDirectives
// (or hoverShowStateDirectives) out of LSP initializationOptions. Defaults to
// false so hovers show cleaned prose by default.
func hoverShowStateDirectivesFromOptions(opts any) bool {
	m, ok := opts.(map[string]any)
	if !ok {
		return false
	}
	if raw, ok := m["hoverShowStateDirectives"].(bool); ok {
		return raw
	}
	if hoverRaw, ok := m["hover"].(map[string]any); ok {
		if raw, ok := hoverRaw["showStateDirectives"].(bool); ok {
			return raw
		}
	}
	return false
}

// paletteFromOptions reads lore.palette out of LSP initializationOptions, an
// array of hex colour strings. The array is indexed in parallel with the
// loreColour{A..Z} semantic-token modifier bits, so the client and server
// agree on which colour belongs to which entity. Returns nil if absent or
// malformed, in which case hover output is rendered without colour spans.
func paletteFromOptions(opts any) []string {
	m, ok := opts.(map[string]any)
	if !ok {
		return nil
	}
	raw, ok := m["palette"].([]any)
	if !ok {
		return nil
	}
	out := make([]string, 0, len(raw))
	for _, v := range raw {
		s, ok := v.(string)
		if !ok {
			return nil
		}
		out = append(out, s)
	}
	return out
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

	var body string
	switch {
	case !showAt:
		if !showLatest {
			return ""
		}
		body = lore.FormatStateBlock(ent.Tags, ent.Fields)
	case mode == HoverStateModeAtCursor:
		atTags, atFields, _ := lore.ResolveStateAt(ent.StateHistory, cursorFile, cursorLine)
		body = lore.FormatStateBlock(atTags, atFields)
	default:
		atTags, atFields, _ := lore.ResolveStateAt(ent.StateHistory, cursorFile, cursorLine)
		body = lore.FormatStateBlockMerged(atTags, ent.Tags, atFields, ent.Fields)
	}

	if body == "" {
		return ""
	}

	return "\n\n```lore-state\n" + body + "\n```"
}

// formatEntityHover builds markdown hover content for an entity. The cursor
// (file, line) is used to compute the "state at cursor" block when mode
// requests it; pass an empty cursorFile to show only the latest state. When
// showStateDirectives is false, description prose is shown with directive
// spans stripped, and descriptions that reduce to empty text are dropped.
// The colouriser wraps entity names with palette-coloured `<span>` tags so
// the hover matches the buffer; pass nil or one with an empty palette to
// disable colouring (e.g. tests, or older clients without supportHtml).
func formatEntityHover(ent *lore.Entity, cursorFile string, cursorLine int, mode HoverStateMode, showStateDirectives bool, col *colouriser) string {
	var b strings.Builder
	nameSpan := wrapEntityName(ent, col)
	if ent.Type != "" {
		fmt.Fprintf(&b, "<strong>%s</strong> (%s)", nameSpan, ent.Type)
	} else {
		fmt.Fprintf(&b, "<strong>%s</strong>", nameSpan)
	}
	if len(ent.Aliases) > 0 {
		aliases := make([]string, len(ent.Aliases))
		for i, a := range ent.Aliases {
			aliases[i] = wrapEntityAlias(ent, a, col)
		}
		fmt.Fprintf(&b, "\n\nAlso known as: %s", strings.Join(aliases, ", "))
	}
	b.WriteString(renderHoverStateBlocks(ent, cursorFile, cursorLine, mode))

	texts := make([]string, 0, len(ent.Descriptions))
	for _, d := range ent.Descriptions {
		text := d.Text
		if !showStateDirectives {
			text = d.CleanText
		}
		if text == "" {
			continue
		}
		texts = append(texts, col.Wrap(text))
	}
	if len(texts) > 0 {
		b.WriteString("\n\n---\n\n")
		b.WriteString(truncate(strings.Join(texts, "\n\n"), 2000))
	}
	return b.String()
}

// wrapEntityName returns the entity's display name wrapped in a colour span
// for the entity's own palette colour, or the plain name when colouring is
// disabled.
func wrapEntityName(ent *lore.Entity, col *colouriser) string {
	if col == nil || len(col.palette) == 0 {
		return escapeForHTML(ent.Name)
	}
	idx := int(entityColourIndex(ent))
	if idx < 0 || idx >= len(col.palette) {
		return escapeForHTML(ent.Name)
	}
	return `<span style="color:` + col.palette[idx] + `;">` + escapeForHTML(ent.Name) + `</span>`
}

// wrapEntityAlias is like wrapEntityName but for a specific alias string,
// which may not equal ent.Name. The colour comes from the parent entity.
func wrapEntityAlias(ent *lore.Entity, alias string, col *colouriser) string {
	if col == nil || len(col.palette) == 0 {
		return escapeForHTML(alias)
	}
	idx := int(entityColourIndex(ent))
	if idx < 0 || idx >= len(col.palette) {
		return escapeForHTML(alias)
	}
	return `<span style="color:` + col.palette[idx] + `">` + escapeForHTML(alias) + `</span>`
}
