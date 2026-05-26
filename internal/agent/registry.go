package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
)

// ToolEntry describes a single registered tool: its identity, OpenAI schema,
// and the handler function that executes it.
//
// Toolset is a logical grouping label used by GetSchemas to filter which tools
// are exposed to the LLM on each request.  Phase 3 c4 populates the field;
// until then it is an empty string and GetSchemas returns all tools.
//
// Parameters is stored as json.RawMessage so the JSON Schema object is embedded
// verbatim — we never need to parse its contents, only forward it to the API.
//
// Handler receives the same ctx passed to Dispatch, enabling future tools to
// respect cancellation or deadlines.
type ToolEntry struct {
	Name        string
	Toolset     string // e.g. "core", "file" — populated in c4
	Description string
	Parameters  json.RawMessage
	Handler     func(context.Context, ToolCall) ToolResult
}

// Registry holds the set of registered tools and provides dispatch and schema
// lookup.  All methods are synchronous; concurrent registration is not
// supported (tools are registered at process startup via init()).
//
// The zero value is not usable — create with newRegistry().
type Registry struct {
	tools map[string]ToolEntry
}

// newRegistry returns an empty Registry ready for registration.
func newRegistry() *Registry {
	return &Registry{tools: make(map[string]ToolEntry)}
}

// Register adds e to the registry.
//
// Panics if a tool with the same Name is already registered.  Duplicate
// registration indicates a programming error (two init() calls registering the
// same tool) and should never happen in production; panic surfaces it
// immediately rather than silently overwriting the handler.
func (r *Registry) Register(e ToolEntry) {
	if _, exists := r.tools[e.Name]; exists {
		panic(fmt.Sprintf("registry: tool %q already registered", e.Name))
	}
	r.tools[e.Name] = e
}

// Dispatch looks up tc.Name in the registry and calls its Handler.
//
// If the tool is not found, Dispatch returns a ToolResult with IsError=true
// and a JSON error body — matching the convention established in Phase 2 where
// errors are returned in-band so the model can observe what went wrong.
// It never returns a Go error or panics on unknown tools.
func (r *Registry) Dispatch(ctx context.Context, tc ToolCall) ToolResult {
	e, ok := r.tools[tc.Name]
	if !ok {
		return ToolResult{
			ToolCallID: tc.ID,
			Name:       tc.Name,
			Content:    toolError("unknown_tool", "unknown tool: "+tc.Name),
			IsError:    true,
		}
	}
	return e.Handler(ctx, tc)
}

// GetSchemas returns the OpenAI wire schemas for tools whose Toolset is in
// enabledToolsets.
//
// Special case: if enabledToolsets is empty, no schemas are returned — the
// caller asked for nothing.
//
// Special case: the alias "debugging" expands to both "core" and "file",
// enabling all current tools without listing them individually.
//
// Results are sorted by tool Name for stable output (important for tests and
// deterministic wire format).
//
// Phase 3 c4 sets ToolEntry.Toolset; before that all entries have Toolset==""
// and GetSchemas([]string{}) returns an empty slice while
// GetSchemas([]string{""}) returns everything — the latter is used by the
// legacy code path in c2/c3 tests that don't set toolsets yet.
func (r *Registry) GetSchemas(enabledToolsets []string) []openAIToolSpec {
	if len(enabledToolsets) == 0 {
		return nil
	}

	// Build enabled-set, expanding the "debugging" alias.
	enabled := make(map[string]bool, len(enabledToolsets))
	for _, ts := range enabledToolsets {
		if ts == "debugging" {
			enabled["core"] = true
			enabled["file"] = true
		} else {
			enabled[ts] = true
		}
	}

	// Collect matching entries.
	var names []string
	for name, e := range r.tools {
		if enabled[e.Toolset] {
			names = append(names, name)
		}
	}
	sort.Strings(names) // stable order

	specs := make([]openAIToolSpec, 0, len(names))
	for _, name := range names {
		e := r.tools[name]
		specs = append(specs, openAIToolSpec{
			Type: "function",
			Function: openAIToolSpecBody{
				Name:        e.Name,
				Description: e.Description,
				Parameters:  e.Parameters,
			},
		})
	}
	return specs
}

// ── global registry ───────────────────────────────────────────────────────────

// globalRegistry is the process-wide singleton.  Tools are registered at
// startup by init() below.  Using a package-level var (not a sync.Once) is
// safe because Go guarantees all init() functions run before main().
var globalRegistry = newRegistry()

func init() {
	globalRegistry.Register(echoEntry())
	globalRegistry.Register(readFileEntry())
}
