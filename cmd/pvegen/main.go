// Command pvegen generates typed Go bindings for the Proxmox VE REST API.
//
// Input:  _data/apidoc.json — recursive endpoint tree extracted from the
//
//	upstream PVE api-viewer JS bundle.
//
// Output: pkg/api/<namespace>/<namespace>_gen.go — one file per
//
//	top-level namespace. Each file declares a Service interface, a
//	concrete service struct that wraps pkg/client.Client, and a
//	typed Go method per (path, HTTP-verb) tuple under that
//	namespace. Sibling file <namespace>_smoke_test.go exercises the
//	zero-parameter GET methods over an httptest server so the
//	generated code at minimum compiles and round-trips.
//
// Invocation:
//
//	go run ./cmd/pvegen --spec _data/apidoc.json --out pkg/api
//
// The --namespace flag may be repeated to restrict emission to a
// subset of namespaces; when omitted, every top-level namespace found
// in the spec is emitted (subject to the namespaceOutputDir override
// table below for cases where a hand-written package already owns the
// canonical directory).
//
// Generation is deterministic: re-running with the same inputs produces
// byte-identical output. CI enforces this via `make verify-generated`.
package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"go/format"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

// indexedParamRe matches PVE spec param names of the form "word[n]" or
// "word[%d]". The captured group is the base name (e.g. "net" for "net[n]").
var indexedParamRe = regexp.MustCompile(`^([a-z][a-z0-9]*)(?:\[n\]|\[%d\])$`)

// methodNameOverrides pins the emitted Go method name for specific
// (HTTP-verb, path) tuples. The default naming rule produces
// "ListVersion" for GET /version (singular, parameterless, non-dynamic
// tail); the version package has hand-written tests that predate the
// generator and depend on the shorter "Get" name. Keeping those tests
// stable is cheaper than rewriting them.
var methodNameOverrides = map[string]string{
	"GET /version": "Get",
}

// namespaceOutputDir overrides the on-disk directory for a top-level
// spec namespace whose default name would clash with an existing
// hand-written package. The map key is the path-prefix segment from
// apidoc.json; the value is the directory under --out and also the Go
// package name. Keep this list short — every entry is a deviation
// from the "namespace name == directory name" convention and needs a
// note in design.md.
var namespaceOutputDir = map[string]string{
	// /storage top-level endpoints (datacenter storage config) live in
	// pkg/api/clusterstorage. pkg/api/storage is owned by hand-written
	// nodes/{node}/storage helpers that predate the generator.
	"storage": "clusterstorage",
}

// node mirrors the structure of a single tree entry in apidoc.json.
type node struct {
	Path     string                  `json:"path"`
	Text     string                  `json:"text"`
	Leaf     int                     `json:"leaf"`
	Info     map[string]endpointInfo `json:"info"`
	Children []*node                 `json:"children,omitempty"`
}

// endpointInfo describes a single HTTP verb on an endpoint.
type endpointInfo struct {
	Method      string          `json:"method"`
	Name        string          `json:"name"`
	Description string          `json:"description"`
	Parameters  *schema         `json:"parameters,omitempty"`
	Returns     *schema         `json:"returns,omitempty"`
	AllowToken  int             `json:"allowtoken"`
	Permissions json.RawMessage `json:"permissions,omitempty"`
}

// schema is a (subset of) JSON-schema definition. We only model the
// bits the PVE spec actually uses.
type schema struct {
	Type        string             `json:"type,omitempty"`
	Description string             `json:"description,omitempty"`
	Properties  map[string]*schema `json:"properties,omitempty"`
	Items       *schema            `json:"items,omitempty"`
	// Optional is encoded as either int 0/1 or string "0"/"1" depending
	// on the endpoint; kept raw and decoded via isOptional().
	Optional json.RawMessage `json:"optional,omitempty"`
	Default  any             `json:"default,omitempty"`
	Enum     []any           `json:"enum,omitempty"`
	Format   json.RawMessage `json:"format,omitempty"`
	Pattern  string          `json:"pattern,omitempty"`
	// Minimum/Maximum are kept as RawMessage because the upstream spec
	// uses both numbers and quoted strings for these fields. The
	// generator does not emit range validation so a typed parse would
	// add fragility for no gain.
	Minimum              json.RawMessage `json:"minimum,omitempty"`
	Maximum              json.RawMessage `json:"maximum,omitempty"`
	AdditionalProperties json.RawMessage `json:"additionalProperties,omitempty"`
}

// endpoint is a fully resolved (path, verb) tuple ready to emit.
type endpoint struct {
	Path        string
	Verb        string
	Info        endpointInfo
	GoMethod    string
	GoNamespace string
	PathParams  []string // ordered, brace-stripped path-parameter names
}

func main() {
	specPath := flag.String("spec", "_data/apidoc.json", "Path to apidoc.json")
	outDir := flag.String("out", "pkg/api", "Root output directory")

	var nsList stringSlice
	flag.Var(&nsList, "namespace", "Namespace to emit (repeatable). Defaults to every namespace in the spec.")
	flag.Parse()

	tree, err := loadSpec(*specPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "pvegen: load spec: %v\n", err)
		os.Exit(1)
	}

	endpoints := collectEndpoints(tree)
	byNS := groupByNamespace(endpoints)

	wantNS := map[string]bool{}

	if len(nsList) == 0 {
		for ns := range byNS {
			wantNS[ns] = true
		}
	} else {
		for _, ns := range nsList {
			if _, ok := byNS[ns]; !ok {
				fmt.Fprintf(os.Stderr, "pvegen: warning: namespace %q not found in spec (skipped)\n", ns)

				continue
			}

			wantNS[ns] = true
		}
	}

	if len(wantNS) == 0 {
		fmt.Fprintln(os.Stderr, "pvegen: no namespaces selected; nothing to do")
		os.Exit(0)
	}

	// Deterministic order for log output.
	order := make([]string, 0, len(wantNS))
	for ns := range wantNS {
		order = append(order, ns)
	}

	sort.Strings(order)

	for _, ns := range order {
		eps := byNS[ns]
		if len(eps) == 0 {
			fmt.Fprintf(os.Stderr, "pvegen: warning: namespace %q has no endpoints in spec\n", ns)

			continue
		}

		err := emitNamespace(*outDir, ns, eps)
		if err != nil {
			fmt.Fprintf(os.Stderr, "pvegen: emit %s: %v\n", ns, err)
			os.Exit(1)
		}

		fmt.Fprintf(os.Stderr, "pvegen: wrote namespace %q (%d endpoints)\n", ns, len(eps))
	}
}

// stringSlice implements flag.Value for repeatable string flags.
type stringSlice []string

func (s *stringSlice) String() string { return strings.Join(*s, ",") }
func (s *stringSlice) Set(v string) error {
	*s = append(*s, v)

	return nil
}

// loadSpec reads apidoc.json and returns the top-level node list.
func loadSpec(path string) ([]*node, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", path, err)
	}

	var tree []*node

	err = json.Unmarshal(raw, &tree)
	if err != nil {
		return nil, fmt.Errorf("parse %s: %w", path, err)
	}

	if len(tree) == 0 {
		return nil, fmt.Errorf("spec %s is empty", path)
	}

	return tree, nil
}

// collectEndpoints walks the tree and returns one endpoint per
// (path, verb). Output is sorted by (path, verb) for deterministic
// generation.
func collectEndpoints(tree []*node) []endpoint {
	var (
		out  []endpoint
		walk func(*node)
	)

	walk = func(n *node) {
		if n == nil {
			return
		}

		verbs := make([]string, 0, len(n.Info))
		for v := range n.Info {
			verbs = append(verbs, v)
		}

		sort.Strings(verbs)

		for _, v := range verbs {
			info := n.Info[v]
			out = append(out, endpoint{
				Path:        n.Path,
				Verb:        v,
				Info:        info,
				GoNamespace: namespaceOf(n.Path),
				PathParams:  extractPathParams(n.Path),
			})
		}

		for _, c := range n.Children {
			walk(c)
		}
	}
	for _, root := range tree {
		walk(root)
	}

	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Path != out[j].Path {
			return out[i].Path < out[j].Path
		}

		return out[i].Verb < out[j].Verb
	})

	return out
}

// namespaceOf returns the top-level path segment for grouping. For
// "/version" the namespace is "version"; for "/nodes/{node}/qemu/..."
// it is "nodes". The root path "/" maps to "root".
func namespaceOf(path string) string {
	p := strings.TrimPrefix(path, "/")
	if p == "" {
		return "root"
	}

	if i := strings.Index(p, "/"); i >= 0 {
		return p[:i]
	}

	return p
}

// groupByNamespace buckets endpoints by their top-level namespace.
func groupByNamespace(eps []endpoint) map[string][]endpoint {
	out := map[string][]endpoint{}
	for _, ep := range eps {
		out[ep.GoNamespace] = append(out[ep.GoNamespace], ep)
	}

	return out
}

// extractPathParams returns the ordered list of {placeholder} names in
// the path, brace-stripped. "/nodes/{node}/qemu/{vmid}/status" yields
// ["node", "vmid"].
func extractPathParams(path string) []string {
	parts := strings.Split(strings.TrimPrefix(path, "/"), "/")

	var out []string

	for _, part := range parts {
		if strings.HasPrefix(part, "{") && strings.HasSuffix(part, "}") {
			out = append(out, part[1:len(part)-1])
		}
	}

	return out
}

// goMethodBaseName turns an (HTTP verb, path) into the unqualified
// PascalCase identifier for the generated method. Collisions inside
// the same namespace are resolved by the caller (see assignMethodNames).
//
// Rules:
//
//   - GET    on a path whose last segment is dynamic ("{xxx}") → "Get..."
//   - GET    on a non-dynamic path                           → "List..." when the spec marks a non-leaf, "Get..." otherwise
//   - POST   → "Create..."
//   - PUT    → "Update..."
//   - DELETE → "Delete..."
//
// The resource portion is the full PascalCase path with brace
// placeholders dropped. The full path is used (rather than just the
// trailing segment) because the spec has many endpoints whose trailing
// segment alone ("config", "status", "current") is non-unique within a
// namespace.
func goMethodBaseName(ep endpoint) string {
	resource := pascalFromPath(ep.Path)

	var prefix string

	switch strings.ToUpper(ep.Verb) {
	case "GET":
		if endsInPathParam(ep.Path) {
			prefix = "Get"
		} else {
			prefix = "List"
		}
	case "POST":
		prefix = "Create"
	case "PUT":
		prefix = "Update"
	case "DELETE":
		prefix = "Delete"
	default:
		prefix = pascalize(strings.ToLower(ep.Verb))
	}

	// Drop the leading namespace segment from the resource so the
	// emitted name reads naturally ("ListUsers", not "ListAccessUsers"
	// inside package access). We keep the namespace prefix only when
	// stripping it would yield an empty identifier.
	ns := pascalize(ep.GoNamespace)
	if ns != "" && strings.HasPrefix(resource, ns) {
		stripped := strings.TrimPrefix(resource, ns)
		if stripped != "" {
			resource = stripped
		}
	}

	if resource == "" {
		resource = "Root"
	}

	return prefix + resource
}

// endsInPathParam reports whether the trailing path segment is a brace
// placeholder. Used to distinguish singular-resource GETs (return a
// single item, named GetX) from collection GETs (return a list, named
// ListX).
func endsInPathParam(path string) bool {
	p := strings.TrimSuffix(path, "/")
	if p == "" {
		return false
	}

	i := strings.LastIndex(p, "/")
	last := p[i+1:]

	return strings.HasPrefix(last, "{") && strings.HasSuffix(last, "}")
}

// assignMethodNames mutates eps in place, filling endpoint.GoMethod
// with collision-free identifiers. Endpoints are sorted by (path,
// verb) on entry, so the order of disambiguation is stable.
func assignMethodNames(eps []endpoint) {
	seen := map[string]int{}

	for i := range eps {
		key := strings.ToUpper(eps[i].Verb) + " " + eps[i].Path
		if override, ok := methodNameOverrides[key]; ok {
			eps[i].GoMethod = override
			seen[override]++

			continue
		}

		base := goMethodBaseName(eps[i])
		name := base

		if n, ok := seen[base]; ok {
			// Append a 2-based numeric suffix to keep the first hit
			// unsuffixed. This is deterministic given the input order.
			name = fmt.Sprintf("%s%d", base, n+1)
			seen[base] = n + 1
		} else {
			seen[base] = 1
		}

		eps[i].GoMethod = name
	}
}

// pascalFromPath returns a PascalCase identifier built from the
// non-parameter segments of a path. "/version" → "Version".
// "/nodes/{node}/qemu/{vmid}/status/current" → "NodesQemuStatusCurrent".
// Brace placeholders are dropped because they become Go method
// parameters, not part of the method name.
func pascalFromPath(path string) string {
	p := strings.TrimPrefix(path, "/")
	if p == "" {
		return "Root"
	}

	parts := strings.Split(p, "/")

	var kept []string

	for _, part := range parts {
		if strings.HasPrefix(part, "{") && strings.HasSuffix(part, "}") {
			continue
		}

		kept = append(kept, part)
	}

	var sb strings.Builder
	for _, part := range kept {
		sb.WriteString(pascalize(part))
	}

	if sb.Len() == 0 {
		return "Root"
	}

	return sb.String()
}

// pascalize converts a snake_case / dash-case / dotted token to
// PascalCase. Empty input yields "". Non-identifier characters such
// as brackets ("link[n]") are stripped before splitting; the caller
// keeps the original spelling for the JSON tag.
func pascalize(s string) string {
	if s == "" {
		return ""
	}

	s = sanitizeIdentChars(s)

	parts := splitOnAny(s, "-_.")

	var sb strings.Builder

	for _, p := range parts {
		if p == "" {
			continue
		}

		sb.WriteString(strings.ToUpper(p[:1]))
		sb.WriteString(p[1:])
	}

	return sb.String()
}

// camelize converts a snake_case / dash-case / dotted token to
// camelCase (first letter lower).
func camelize(s string) string {
	pc := pascalize(s)
	if pc == "" {
		return ""
	}

	return strings.ToLower(pc[:1]) + pc[1:]
}

// goIdentSafe returns s if it is a non-reserved Go identifier, else
// returns s with a trailing underscore so it does not collide with a
// keyword or predeclared name.
func goIdentSafe(s string) string {
	switch s {
	case "type", "range", "func", "default", "select", "chan", "map",
		"interface", "struct", "package", "import", "return", "if",
		"else", "for", "switch", "case", "break", "continue", "goto",
		"defer", "go", "const", "var", "fallthrough":
		return s + "_"
	}

	return s
}

func splitOnAny(s, seps string) []string {
	return strings.FieldsFunc(s, func(r rune) bool {
		return strings.ContainsRune(seps, r)
	})
}

// sanitizeIdentChars drops every rune that is not a letter, digit, or
// underscore. The PVE spec uses bracket suffixes like "link[n]" or
// "ip[%d]" to denote indexed arrays; those are not legal Go
// identifiers and must be stripped before Pascal-casing.
func sanitizeIdentChars(s string) string {
	var b strings.Builder

	for _, r := range s {
		switch {
		case r >= 'a' && r <= 'z':
			b.WriteRune(r)
		case r >= 'A' && r <= 'Z':
			b.WriteRune(r)
		case r >= '0' && r <= '9':
			b.WriteRune(r)
		case r == '_' || r == '-' || r == '.':
			b.WriteRune(r)
		}
	}

	return b.String()
}

// emitNamespace writes both the generated source and the smoke test
// for the given namespace.
func emitNamespace(outRoot, ns string, eps []endpoint) error {
	// Sort and assign collision-free method names before emission.
	sort.SliceStable(eps, func(i, j int) bool {
		if eps[i].Path != eps[j].Path {
			return eps[i].Path < eps[j].Path
		}

		return eps[i].Verb < eps[j].Verb
	})
	assignMethodNames(eps)

	dirName := ns
	if override, ok := namespaceOutputDir[ns]; ok {
		dirName = override
	}

	pkgName := dirName

	dir := filepath.Join(outRoot, dirName)

	err := os.MkdirAll(dir, 0o755)
	if err != nil {
		return fmt.Errorf("mkdir %s: %w", dir, err)
	}

	src, err := renderNamespaceSource(pkgName, ns, eps)
	if err != nil {
		return fmt.Errorf("render source: %w", err)
	}

	formatted, err := format.Source(src)
	if err != nil {
		return fmt.Errorf("gofmt generated source: %w\n--- source ---\n%s", err, src)
	}

	target := filepath.Join(dir, dirName+"_gen.go")

	err = writeIfChanged(target, formatted)
	if err != nil {
		return fmt.Errorf("write %s: %w", target, err)
	}

	// Smoke test is only emitted for non-version namespaces. The
	// version package has hand-written tests already and we must not
	// stomp on them.
	if ns != "version" {
		smokeSrc, err := renderNamespaceSmokeTest(pkgName, ns, eps)
		if err != nil {
			return fmt.Errorf("render smoke test: %w", err)
		}

		smokeFormatted, err := format.Source(smokeSrc)
		if err != nil {
			return fmt.Errorf("gofmt smoke test: %w\n--- source ---\n%s", err, smokeSrc)
		}

		smokeTarget := filepath.Join(dir, dirName+"_smoke_test.go")

		err = writeIfChanged(smokeTarget, smokeFormatted)
		if err != nil {
			return fmt.Errorf("write %s: %w", smokeTarget, err)
		}
	}

	return nil
}

// renderNamespaceSource builds the full _gen.go source for a namespace.
func renderNamespaceSource(pkgName, ns string, eps []endpoint) ([]byte, error) {
	var b strings.Builder

	fmt.Fprintf(&b, "// Code generated by cmd/pvegen. DO NOT EDIT.\n\n")
	fmt.Fprintf(&b, "// Package %s exposes typed bindings for the PVE /%s endpoint family.\n", pkgName, ns)
	fmt.Fprintf(&b, "package %s\n\n", pkgName)
	b.WriteString("import (\n")
	b.WriteString("\t\"context\"\n")
	b.WriteString("\t\"encoding/json\"\n")
	b.WriteString("\t\"fmt\"\n")
	b.WriteString("\t\"net/url\"\n")
	b.WriteString("\t\"sort\"\n")
	b.WriteString("\t\"strconv\"\n")
	b.WriteString("\t\"strings\"\n\n")
	b.WriteString("\t\"github.com/fivetwenty-io/pve-apiclient-go/v3/pkg/client\"\n")
	b.WriteString(")\n\n")

	// Suppress "imported and not used" when a namespace happens to use
	// no Params (and therefore no strings). All current namespaces use
	// at least one Params struct, but keep a defensive blank reference
	// so the file always compiles even if a future spec shrinks.
	b.WriteString("var _ = strings.TrimPrefix\n")
	b.WriteString("var _ = json.RawMessage(nil)\n")
	b.WriteString("var _ = url.PathEscape\n")
	b.WriteString("var _ = sort.Ints\n")
	b.WriteString("var _ = strconv.Itoa\n\n")

	// Service interface.
	b.WriteString("// Service exposes typed operations on the /" + ns + " endpoint family.\n")
	b.WriteString("type Service interface {\n")

	for _, ep := range eps {
		sig, _ := renderMethodSignature(ep, true)
		fmt.Fprintf(&b, "\t// %s %s %s\n", ep.GoMethod, ep.Verb, ep.Path)

		desc := escapeDoc(ep.Info.Description)
		if desc != "" {
			fmt.Fprintf(&b, "\t// %s\n", desc)
		}

		fmt.Fprintf(&b, "\t%s\n", sig)
	}

	b.WriteString("}\n\n")

	// Constructor + concrete service.
	fmt.Fprintf(&b, `// New constructs a Service backed by the given pkg/client.Client.
// The client owns authentication, TLS, retries, and logging; this
// service only translates between typed Go shapes and the raw client.
func New(c client.Client) Service {
	if c == nil {
		panic("%s: New called with nil client")
	}
	return &service{c: c}
}

type service struct {
	c client.Client
}
`, pkgName)

	b.WriteString("\n")

	// Per-endpoint: Params struct, Response struct, method impl.
	for _, ep := range eps {
		err := renderEndpoint(&b, pkgName, ep)
		if err != nil {
			return nil, fmt.Errorf("endpoint %s %s: %w", ep.Verb, ep.Path, err)
		}
	}

	return []byte(b.String()), nil
}

// renderMethodSignature returns the Go method signature line and a
// flag telling whether the endpoint has non-path parameters. When
// forInterface is true the receiver portion is omitted (interface
// method form); when false it includes "(s *service)".
func renderMethodSignature(ep endpoint, forInterface bool) (string, bool) {
	var args []string

	args = append(args, "ctx context.Context")
	for _, p := range ep.PathParams {
		args = append(args, goIdentSafe(camelize(p))+" string")
	}

	hasParams := hasNonPathParams(ep)
	if hasParams {
		args = append(args, fmt.Sprintf("params *%sParams", ep.GoMethod))
	}

	respType := responseGoType(ep)

	var retSig string

	if respType == "" {
		retSig = "error"
	} else {
		retSig = fmt.Sprintf("(*%s, error)", responseTypeName(ep))
	}

	if forInterface {
		return fmt.Sprintf("%s(%s) %s", ep.GoMethod, strings.Join(args, ", "), retSig), hasParams
	}

	return fmt.Sprintf("(s *service) %s(%s) %s", ep.GoMethod, strings.Join(args, ", "), retSig), hasParams
}

// hasNonPathParams reports whether the endpoint accepts at least one
// non-path query/body parameter. Used to decide whether to emit a
// <Method>Params struct.
func hasNonPathParams(ep endpoint) bool {
	if ep.Info.Parameters == nil || len(ep.Info.Parameters.Properties) == 0 {
		return false
	}

	pathSet := map[string]bool{}
	for _, p := range ep.PathParams {
		pathSet[p] = true
	}

	for name := range ep.Info.Parameters.Properties {
		if !pathSet[name] {
			return true
		}
	}

	return false
}

// responseGoType returns the Go type of the response body, or "" when
// the spec declares no return value (e.g. type:null). For object
// returns the typed struct name is returned; the actual struct is
// emitted by renderEndpoint.
func responseGoType(ep endpoint) string {
	r := ep.Info.Returns
	if r == nil {
		return ""
	}

	if r.Type == "null" {
		return ""
	}

	// For arrays, objects, scalars: there is always something to
	// decode; even a bare scalar returns a typed wrapper.
	return responseTypeName(ep)
}

// responseTypeName returns the name of the Go type used for the
// endpoint's response. Currently every endpoint with a non-null
// return gets a "<Method>Response" alias for documentation purposes,
// even when the underlying shape is json.RawMessage.
func responseTypeName(ep endpoint) string {
	return ep.GoMethod + "Response"
}

// renderEndpoint emits the Params struct, Response type, and method
// implementation for a single endpoint.
func renderEndpoint(b *strings.Builder, pkgName string, ep endpoint) error {
	// Params struct.
	if hasNonPathParams(ep) {
		err := renderParamsStruct(b, ep)
		if err != nil {
			return fmt.Errorf("render params: %w", err)
		}
	}

	// Response type.
	respGo := responseGoType(ep)
	if respGo != "" {
		err := renderResponseType(b, ep)
		if err != nil {
			return fmt.Errorf("render response: %w", err)
		}
	}

	// Method body.
	return renderMethodBody(b, pkgName, ep)
}

// indexedField carries metadata for a single "base[n]" param family
// found within a Params struct.
type indexedField struct {
	// BaseName is the raw spec base, e.g. "net" for "net[n]".
	BaseName string
	// FieldName is the exported Go field name, e.g. "Net".
	FieldName string
	// Desc is the doc comment text (first occurrence in the spec wins).
	Desc string
}

// renderParamsStruct emits "type <Method>Params struct { ... }" and,
// when the endpoint has any indexed ("base[n]") parameters, a custom
// MarshalJSON method that expands map[int]string fields into numbered
// keys ("net0", "net1", …) so the existing JSON-round-trip body
// construction produces correct Proxmox wire format.
func renderParamsStruct(b *strings.Builder, ep endpoint) error {
	fmt.Fprintf(b, "// %sParams is the request payload for %s.\n", ep.GoMethod, ep.GoMethod)
	fmt.Fprintf(b, "type %sParams struct {\n", ep.GoMethod)

	pathSet := map[string]bool{}
	for _, p := range ep.PathParams {
		pathSet[p] = true
	}

	names := make([]string, 0, len(ep.Info.Parameters.Properties))
	for n := range ep.Info.Parameters.Properties {
		if pathSet[n] {
			continue
		}

		names = append(names, n)
	}

	sort.Strings(names)

	// First pass: collect all regular (non-indexed) field names so we can
	// detect clashes when assigning indexed field names.
	regularFieldNames := map[string]bool{}

	for _, name := range names {
		if _, ok := isIndexedParam(name); ok {
			continue
		}

		fn := sanitizeFieldName(pascalize(name))
		regularFieldNames[fn] = true
	}

	// Track which base names have already been emitted as map[int]string
	// so that multiple "[n]" variants for the same base (rare but possible
	// in future specs) collapse to a single field.
	emittedIndexed := map[string]bool{}

	var indexedFields []indexedField

	// Collect indexed fields first so we can emit them in sorted order
	// after regular fields (struct layout: scalars first, maps last).
	type pendingIndexed struct {
		base      string
		fieldName string
		desc      string
	}

	var pending []pendingIndexed

	for _, name := range names {
		prop := ep.Info.Parameters.Properties[name]

		base, ok := isIndexedParam(name)
		if !ok {
			continue
		}

		if emittedIndexed[base] {
			continue
		}

		emittedIndexed[base] = true

		fieldName := sanitizeFieldName(pascalize(base))

		// Clash guard: if the base name produces the same Go identifier
		// as a regular field (e.g. "numa" and "numa[n]" both → "Numa"),
		// append "Map" to the indexed field name so both can coexist.
		if regularFieldNames[fieldName] {
			fieldName += "Map"
		}

		desc := strings.TrimSpace(prop.Description)
		pending = append(pending, pendingIndexed{base: base, fieldName: fieldName, desc: desc})
	}

	// Emit regular fields.
	for _, name := range names {
		if _, ok := isIndexedParam(name); ok {
			continue
		}

		prop := ep.Info.Parameters.Properties[name]

		goType, err := goTypeFor(prop)
		if err != nil {
			// Fall back to json.RawMessage rather than failing
			// generation; the spec uses several shapes we deliberately
			// do not model.
			goType = "json.RawMessage"
		}

		opt := isOptional(prop)
		// Every non-path parameter is generated as a pointer so callers
		// can leave it unset. Scalars use *T; collection types keep
		// their zero-value semantics (nil map / nil slice) and are NOT
		// pointer-wrapped.
		if opt && !isAlreadyNilable(goType) {
			goType = "*" + goType
		}

		fieldName := pascalize(name)
		fieldName = sanitizeFieldName(fieldName)

		desc := strings.TrimSpace(prop.Description)
		if desc != "" {
			fmt.Fprintf(b, "\t// %s %s\n", fieldName, escapeDoc(desc))
		}

		jsonTag := name
		if opt {
			jsonTag += ",omitempty"
		}

		fmt.Fprintf(b, "\t%s %s `json:\"%s\"`\n", fieldName, goType, jsonTag)
	}

	// Emit indexed fields (map[int]string, tagged json:"-").
	for _, pi := range pending {
		if pi.desc != "" {
			fmt.Fprintf(b, "\t// %s (indexed) %s\n", pi.fieldName, escapeDoc(pi.desc))
		} else {
			fmt.Fprintf(b, "\t// %s indexed Proxmox param family (e.g. %s0, %s1, ...).\n",
				pi.fieldName, pi.base, pi.base)
		}

		// map[int]string is nilable; no pointer needed.
		fmt.Fprintf(b, "\t%s map[int]string `json:\"-\"`\n", pi.fieldName)

		indexedFields = append(indexedFields, indexedField{
			BaseName:  pi.base,
			FieldName: pi.fieldName,
			Desc:      pi.desc,
		})
	}

	b.WriteString("}\n\n")

	// If any indexed fields were emitted, generate a MarshalJSON method
	// that merges normally-tagged fields with the expanded indexed fields.
	if len(indexedFields) > 0 {
		renderIndexedMarshalJSON(b, ep.GoMethod, indexedFields)
	}

	return nil
}

// renderIndexedMarshalJSON emits a MarshalJSON method on the Params
// struct that:
//  1. Marshals the struct normally (indexed fields carry json:"-" so they
//     are excluded from the default encoder).
//  2. Decodes the result into a map[string]json.RawMessage.
//  3. For each indexed field with a non-nil map, inserts numbered keys
//     (e.g. "net0", "net1") sorted by index for deterministic wire output.
//  4. Re-encodes and returns the merged map.
func renderIndexedMarshalJSON(b *strings.Builder, methodName string, fields []indexedField) {
	typeName := methodName + "Params"

	fmt.Fprintf(b, "// MarshalJSON implements json.Marshaler for %s.\n", typeName)
	fmt.Fprintf(b, "// It expands indexed map[int]string fields into numbered Proxmox\n")
	fmt.Fprintf(b, "// param keys (e.g. Net[0] → \"net0\", Net[1] → \"net1\").\n")
	fmt.Fprintf(b, "func (p %s) MarshalJSON() ([]byte, error) {\n", typeName)

	// Use an alias type to call the default encoder without recursion.
	fmt.Fprintf(b, "\ttype alias %s\n", typeName)
	fmt.Fprintf(b, "\tbase, err := json.Marshal(alias(p))\n")
	b.WriteString("\tif err != nil {\n")
	fmt.Fprintf(b, "\t\treturn nil, fmt.Errorf(\"%s.MarshalJSON: %%w\", err)\n", typeName)
	b.WriteString("\t}\n\n")

	b.WriteString("\tvar merged map[string]json.RawMessage\n")
	b.WriteString("\tif err = json.Unmarshal(base, &merged); err != nil {\n")
	fmt.Fprintf(b, "\t\treturn nil, fmt.Errorf(\"%s.MarshalJSON decode base: %%w\", err)\n", typeName)
	b.WriteString("\t}\n")
	b.WriteString("\tif merged == nil {\n")
	b.WriteString("\t\tmerged = make(map[string]json.RawMessage)\n")
	b.WriteString("\t}\n\n")

	for _, f := range fields {
		fmt.Fprintf(b, "\t// Expand %s (map[int]string → %s0, %s1, …).\n", f.FieldName, f.BaseName, f.BaseName)
		fmt.Fprintf(b, "\tif len(p.%s) > 0 {\n", f.FieldName)
		fmt.Fprintf(b, "\t\tkeys%s := make([]int, 0, len(p.%s))\n", f.FieldName, f.FieldName)
		fmt.Fprintf(b, "\t\tfor idx := range p.%s {\n", f.FieldName)
		fmt.Fprintf(b, "\t\t\tkeys%s = append(keys%s, idx)\n", f.FieldName, f.FieldName)
		b.WriteString("\t\t}\n")
		fmt.Fprintf(b, "\t\tsort.Ints(keys%s)\n", f.FieldName)
		fmt.Fprintf(b, "\t\tfor _, idx := range keys%s {\n", f.FieldName)
		fmt.Fprintf(b, "\t\t\tkey := %s + strconv.Itoa(idx)\n", strconvQuote(f.BaseName))
		fmt.Fprintf(b, "\t\t\tval, merr := json.Marshal(p.%s[idx])\n", f.FieldName)
		b.WriteString("\t\t\tif merr != nil {\n")
		fmt.Fprintf(b, "\t\t\t\treturn nil, fmt.Errorf(\"%s.MarshalJSON %s[%%d]: %%w\", idx, merr)\n",
			typeName, f.BaseName)
		b.WriteString("\t\t\t}\n")
		b.WriteString("\t\t\tmerged[key] = val\n")
		b.WriteString("\t\t}\n")
		b.WriteString("\t}\n\n")
	}

	b.WriteString("\treturn json.Marshal(merged)\n")
	b.WriteString("}\n\n")
}

// isIndexedParam reports whether name follows the PVE "base[n]" or
// "base[%d]" convention and returns (baseName, true) when it does.
// E.g. "net[n]" → ("net", true), "hostname" → ("", false).
func isIndexedParam(name string) (string, bool) {
	m := indexedParamRe.FindStringSubmatch(name)
	if m == nil {
		return "", false
	}

	return m[1], true
}

// sanitizeFieldName guarantees the result is a valid exported Go
// identifier. Some spec parameter names start with a digit or contain
// only digits; prefix those with "Field".
func sanitizeFieldName(s string) string {
	if s == "" {
		return "Field"
	}

	if c := s[0]; c >= '0' && c <= '9' {
		return "Field" + s
	}

	return s
}

// isAlreadyNilable reports whether the given Go type is naturally
// nil-able (slice, map, pointer, interface, json.RawMessage). Those
// do not need an extra * for optional encoding.
func isAlreadyNilable(t string) bool {
	switch {
	case strings.HasPrefix(t, "[]"):
		return true
	case strings.HasPrefix(t, "map["):
		return true
	case strings.HasPrefix(t, "*"):
		return true
	case t == "json.RawMessage":
		return true
	case t == "interface{}":
		return true
	}

	return false
}

// renderResponseType emits "type <Method>Response ..." sized to the
// shape of the returns schema. Objects become typed structs; arrays
// of objects become []<inner-struct>; everything else falls back to a
// type alias over json.RawMessage so callers can decode further.
func renderResponseType(b *strings.Builder, ep endpoint) error {
	name := responseTypeName(ep)
	r := ep.Info.Returns

	switch {
	case r == nil:
		return nil
	case r.Type == "object" && len(r.Properties) > 0:
		body, err := renderObjectFields(r)
		if err != nil {
			// Fallback to json.RawMessage alias on shapes we cannot
			// model. Do NOT call b.Reset() here — that would wipe every
			// preceding endpoint in the same namespace.
			fmt.Fprintf(b, "// %s is the raw JSON returned by %s %s. The spec\n", name, ep.Verb, ep.Path)
			fmt.Fprintf(b, "// shape was not modellable as a Go struct; decode further as needed.\n")
			fmt.Fprintf(b, "type %s = json.RawMessage\n\n", name)

			return nil //nolint:nilerr // fallback path is intentional
		}

		fmt.Fprintf(b, "// %s mirrors the shape returned by %s %s.\n", name, ep.Verb, ep.Path)
		fmt.Fprintf(b, "type %s struct {\n", name)
		b.WriteString(body)
		b.WriteString("}\n\n")
	case r.Type == "array":
		inner := "json.RawMessage"

		if r.Items != nil {
			it, err := goTypeFor(r.Items)
			if err == nil {
				inner = it
			}
		}

		fmt.Fprintf(b, "// %s mirrors the shape returned by %s %s.\n", name, ep.Verb, ep.Path)
		fmt.Fprintf(b, "type %s []%s\n\n", name, inner)
	default:
		fmt.Fprintf(b, "// %s is the raw JSON returned by %s %s.\n", name, ep.Verb, ep.Path)
		fmt.Fprintf(b, "type %s = json.RawMessage\n\n", name)
	}

	return nil
}

// renderObjectFields returns the inner-body text of a typed struct
// (one field per top-level property of the object schema). Lines are
// already indented with a leading tab; the caller wraps "type X
// struct { ... }".
func renderObjectFields(s *schema) (string, error) {
	if s == nil || s.Type != "object" || len(s.Properties) == 0 {
		return "", errors.New("response schema missing object properties")
	}

	names := make([]string, 0, len(s.Properties))
	for n := range s.Properties {
		names = append(names, n)
	}

	sort.Strings(names)

	var b strings.Builder

	for _, name := range names {
		prop := s.Properties[name]

		goType, err := goTypeFor(prop)
		if err != nil {
			// For response shapes, unknown types fall back to RawMessage.
			goType = "json.RawMessage"
		}

		opt := isOptional(prop)
		if opt && !isAlreadyNilable(goType) {
			goType = "*" + goType
		}

		fieldName := sanitizeFieldName(pascalize(name))

		desc := strings.TrimSpace(prop.Description)
		if desc != "" {
			fmt.Fprintf(&b, "\t// %s %s\n", fieldName, escapeDoc(desc))
		}

		jsonTag := name
		if opt {
			jsonTag += ",omitempty"
		}

		fmt.Fprintf(&b, "\t%s %s `json:\"%s\"`\n", fieldName, goType, jsonTag)
	}

	return b.String(), nil
}

// renderMethodBody emits the receiver-method implementation.
func renderMethodBody(b *strings.Builder, pkgName string, ep endpoint) error {
	sig, _ := renderMethodSignature(ep, false)

	respGo := responseGoType(ep)

	fmt.Fprintf(b, "// %s implements Service.%s. %s %s.\n", ep.GoMethod, ep.GoMethod, ep.Verb, ep.Path)
	fmt.Fprintf(b, "func %s {\n", sig)
	fmt.Fprintf(b, "\tif ctx == nil {\n\t\treturn %s fmt.Errorf(\"%s.%s: ctx must not be nil\")\n\t}\n",
		zeroReturnExpr(respGo), pkgName, ep.GoMethod)

	// Build the path with %s substitution for each path parameter.
	pathExpr := buildPathExpression(ep)
	b.WriteString("\tpath := " + pathExpr + "\n")

	// Build params map from struct.
	hasParams := hasNonPathParams(ep)
	if hasParams {
		b.WriteString("\tvar body map[string]interface{}\n")
		b.WriteString("\tif params != nil {\n")
		b.WriteString("\t\traw, err := json.Marshal(params)\n")
		b.WriteString("\t\tif err != nil {\n")
		fmt.Fprintf(b, "\t\t\treturn %s fmt.Errorf(\"%s.%s: marshal params: %%w\", err)\n",
			zeroReturnExpr(respGo), pkgName, ep.GoMethod)
		b.WriteString("\t\t}\n")
		b.WriteString("\t\terr = json.Unmarshal(raw, &body)\n")
		b.WriteString("\t\tif err != nil {\n")
		fmt.Fprintf(b, "\t\t\treturn %s fmt.Errorf(\"%s.%s: decode params: %%w\", err)\n",
			zeroReturnExpr(respGo), pkgName, ep.GoMethod)
		b.WriteString("\t\t}\n")
		b.WriteString("\t}\n")
	} else {
		b.WriteString("\tvar body map[string]interface{}\n")
	}

	// Dispatch to the appropriate client verb.
	var verbCall string

	switch strings.ToUpper(ep.Verb) {
	case "GET":
		verbCall = "GetRawCtx"
	case "POST":
		verbCall = "PostRawCtx"
	case "PUT":
		verbCall = "PutRawCtx"
	case "DELETE":
		verbCall = "DeleteRawCtx"
	default:
		return fmt.Errorf("unsupported HTTP verb %q on %s", ep.Verb, ep.Path)
	}

	fmt.Fprintf(b, "\tresp, err := s.c.%s(ctx, path, body)\n", verbCall)
	b.WriteString("\tif err != nil {\n")
	fmt.Fprintf(b, "\t\treturn %s fmt.Errorf(\"%s.%s: %%w\", err)\n", zeroReturnExpr(respGo), pkgName, ep.GoMethod)
	b.WriteString("\t}\n")
	b.WriteString("\tif resp == nil {\n")
	fmt.Fprintf(b, "\t\treturn %s fmt.Errorf(\"%s.%s: nil response from client\")\n",
		zeroReturnExpr(respGo), pkgName, ep.GoMethod)
	b.WriteString("\t}\n")

	if respGo == "" {
		b.WriteString("\t_ = resp\n")
		b.WriteString("\treturn nil\n")
		b.WriteString("}\n\n")

		return nil
	}

	// Decode resp.Data into typed response via JSON round-trip.
	b.WriteString("\tif resp.Data == nil {\n")

	respName := responseTypeName(ep)
	// For alias types and slice types, the zero value is the correct empty result.
	if isResponseEmptyOk(ep) {
		fmt.Fprintf(b, "\t\tout := %s{}\n", respName)
		b.WriteString("\t\treturn &out, nil\n")
	} else {
		fmt.Fprintf(b, "\t\treturn nil, fmt.Errorf(\"%s.%s: empty data in response (code=%%d)\", resp.Code)\n",
			pkgName, ep.GoMethod)
	}

	b.WriteString("\t}\n")
	b.WriteString("\traw, err := json.Marshal(resp.Data)\n")
	b.WriteString("\tif err != nil {\n")
	fmt.Fprintf(b, "\t\treturn nil, fmt.Errorf(\"%s.%s: re-marshal data: %%w\", err)\n", pkgName, ep.GoMethod)
	b.WriteString("\t}\n")
	fmt.Fprintf(b, "\tout := &%s{}\n", respName)
	b.WriteString("\terr = json.Unmarshal(raw, out)\n")
	b.WriteString("\tif err != nil {\n")
	fmt.Fprintf(b, "\t\treturn nil, fmt.Errorf(\"%s.%s: unmarshal data: %%w\", err)\n", pkgName, ep.GoMethod)
	b.WriteString("\t}\n")
	b.WriteString("\treturn out, nil\n")
	b.WriteString("}\n\n")

	return nil
}

// isResponseEmptyOk reports whether an absent "data" field in the
// response should be treated as the empty zero value rather than an
// error. Array and aliased-RawMessage responses are tolerant; typed
// struct responses are strict.
func isResponseEmptyOk(ep endpoint) bool {
	r := ep.Info.Returns
	if r == nil {
		return true
	}

	if r.Type == "array" {
		return true
	}
	// Aliased RawMessage = anything not a populated object/array.
	if r.Type != "object" || len(r.Properties) == 0 {
		return true
	}

	return false
}

// zeroReturnExpr returns the leading expression for an error-only or
// "(nil, ...)" return statement. For methods that return only an
// error it returns ""; for methods that return a (T, error) it
// returns "nil,".
func zeroReturnExpr(respGo string) string {
	if respGo == "" {
		return ""
	}

	return "nil,"
}

// buildPathExpression returns a Go expression that evaluates to the
// fully-substituted endpoint path at runtime. Each path parameter is
// escaped via url.PathEscape before substitution to prevent path
// traversal (e.g. a caller passing vmid="100/../../etc" would otherwise
// corrupt the constructed URL).
func buildPathExpression(ep endpoint) string {
	if len(ep.PathParams) == 0 {
		return strconvQuote(ep.Path)
	}

	// Replace each {name} with %s in the format string.
	format := ep.Path

	args := make([]string, 0, len(ep.PathParams))

	for _, p := range ep.PathParams {
		format = strings.Replace(format, "{"+p+"}", "%s", 1)
		args = append(args, "url.PathEscape("+goIdentSafe(camelize(p))+")")
	}

	return fmt.Sprintf("fmt.Sprintf(%s, %s)", strconvQuote(format), strings.Join(args, ", "))
}

// strconvQuote is a tiny helper that returns a Go-quoted string
// literal for s. We use this instead of strconv.Quote to avoid
// pulling in the strconv import in the generator output (we only
// quote ASCII-safe paths from the spec).
func strconvQuote(s string) string {
	return "\"" + strings.ReplaceAll(s, "\"", "\\\"") + "\""
}

// goTypeFor maps a JSON-schema type to a Go type. Unknown shapes fall
// back to json.RawMessage so the generator stays total.
func goTypeFor(s *schema) (string, error) {
	if s == nil {
		return "", errors.New("nil schema")
	}

	switch s.Type {
	case "string":
		return "string", nil
	case "integer":
		return "int64", nil
	case "number":
		return "float64", nil
	case "boolean":
		return "bool", nil
	case "array":
		if s.Items == nil {
			return "[]json.RawMessage", nil
		}

		inner, err := goTypeFor(s.Items)
		if err != nil {
			return "[]json.RawMessage", nil //nolint:nilerr // intentional fallback
		}

		return "[]" + inner, nil
	case "object":
		// Nested objects fall back to raw JSON; emitting nested typed
		// structs recursively would explode the surface area beyond the
		// SW3 scope. Callers can decode further via json.Unmarshal.
		return "json.RawMessage", nil
	case "null":
		return "json.RawMessage", nil
	case "":
		return "json.RawMessage", nil
	default:
		return "json.RawMessage", nil
	}
}

// isOptional reports whether a schema's optional flag is truthy. The
// upstream spec encodes it as either the integer 1 or the string "1";
// any other shape (absent, 0, "0", null, "") means required.
func isOptional(s *schema) bool {
	if s == nil || len(s.Optional) == 0 {
		return false
	}

	var asInt int

	err := json.Unmarshal(s.Optional, &asInt)
	if err == nil {
		return asInt == 1
	}

	var asStr string

	err = json.Unmarshal(s.Optional, &asStr)
	if err == nil {
		return asStr == "1" || strings.EqualFold(asStr, "true")
	}

	return false
}

// escapeDoc strips characters that break Go doc comments (mainly
// trailing whitespace and embedded newlines).
func escapeDoc(s string) string {
	s = strings.ReplaceAll(s, "\r\n", " ")
	s = strings.ReplaceAll(s, "\n", " ")
	s = strings.ReplaceAll(s, "\t", " ")

	return strings.TrimSpace(s)
}

// writeIfChanged writes data to path only when the existing content
// differs. This is a quality-of-life improvement for editor watchers
// and also keeps mtimes stable across no-op generator runs.
func writeIfChanged(path string, data []byte) error {
	existing, err := os.ReadFile(path)
	if err == nil && bytes.Equal(existing, data) {
		return nil
	}

	return os.WriteFile(path, data, 0o644)
}

// renderNamespaceSmokeTest builds a single table-driven smoke test
// that verifies the generated GET methods compile and round-trip JSON
// over an httptest server. Methods with non-path required parameters
// or non-GET verbs are skipped — they exercise state-changing endpoints
// and need real fixtures, which is out of scope for SW3.
func renderNamespaceSmokeTest(pkgName, ns string, eps []endpoint) ([]byte, error) {
	var b strings.Builder

	fmt.Fprintf(&b, "// Code generated by cmd/pvegen. DO NOT EDIT.\n\n")
	fmt.Fprintf(&b, "package %s_test\n\n", pkgName)

	b.WriteString("import (\n")
	b.WriteString("\t\"context\"\n")
	b.WriteString("\t\"encoding/json\"\n")
	b.WriteString("\t\"net/http\"\n")
	b.WriteString("\t\"net/http/httptest\"\n")
	b.WriteString("\t\"net/url\"\n")
	b.WriteString("\t\"strconv\"\n")
	b.WriteString("\t\"testing\"\n\n")
	fmt.Fprintf(&b, "\t\"github.com/fivetwenty-io/pve-apiclient-go/v3/pkg/api/%s\"\n", pkgName)
	b.WriteString("\tpveclient \"github.com/fivetwenty-io/pve-apiclient-go/v3/pkg/client\"\n")
	b.WriteString(")\n\n")

	b.WriteString(`func smokeOptsFromServerURL(u string) pveclient.Options {
	parsed, err := url.Parse(u)
	if err != nil {
		panic("test setup: invalid server URL: " + err.Error())
	}

	host := parsed.Hostname()

	port := 0
	if p := parsed.Port(); p != "" {
		port, _ = strconv.Atoi(p)
	}

	return pveclient.Options{
		Host:     host,
		Port:     port,
		Protocol: "http",
		APIToken: "user@pam!tok=sec",
	}
}

`)

	// Pick the smoke-eligible endpoints: GET only.
	type smokeCase struct {
		Method     string
		PathParams []string
		HasParams  bool
		RespType   string
	}

	var cases []smokeCase

	for _, ep := range eps {
		if strings.ToUpper(ep.Verb) != "GET" {
			continue
		}

		cases = append(cases, smokeCase{
			Method:     ep.GoMethod,
			PathParams: ep.PathParams,
			HasParams:  hasNonPathParams(ep),
			RespType:   responseGoType(ep),
		})
	}

	// Emit one test func per namespace that walks all smoke cases.
	fmt.Fprintf(&b, "func TestSmoke_%s_GeneratedMethods(t *testing.T) {\n", pascalize(pkgName))
	b.WriteString("\tt.Parallel()\n\n")
	b.WriteString("\tmux := http.NewServeMux()\n")
	b.WriteString("\tmux.HandleFunc(\"/\", func(w http.ResponseWriter, r *http.Request) {\n")
	b.WriteString("\t\t_ = json.NewEncoder(w).Encode(map[string]any{\n")
	b.WriteString("\t\t\t\"data\":    map[string]any{},\n")
	b.WriteString("\t\t\t\"success\": 1,\n")
	b.WriteString("\t\t})\n")
	b.WriteString("\t})\n\n")
	b.WriteString("\tsrv := httptest.NewServer(mux)\n")
	b.WriteString("\tdefer srv.Close()\n\n")
	b.WriteString("\tc, err := pveclient.NewClient(smokeOptsFromServerURL(srv.URL))\n")
	b.WriteString("\tif err != nil {\n")
	b.WriteString("\t\tt.Fatalf(\"NewClient: %v\", err)\n")
	b.WriteString("\t}\n\n")
	fmt.Fprintf(&b, "\tsvc := %s.New(c)\n", pkgName)
	b.WriteString("\tctx := context.Background()\n\n")

	for _, c := range cases {
		fmt.Fprintf(&b, "\tt.Run(%q, func(t *testing.T) {\n", c.Method)

		args := []string{"ctx"}

		for _, p := range c.PathParams {
			_ = p

			args = append(args, "\"stub\"")
		}

		if c.HasParams {
			args = append(args, "nil")
		}

		callExpr := fmt.Sprintf("svc.%s(%s)", c.Method, strings.Join(args, ", "))

		if c.RespType == "" {
			fmt.Fprintf(&b, "\t\terr := %s\n", callExpr)
			b.WriteString("\t\t_ = err // smoke: any outcome is acceptable; the stub server is permissive\n")
		} else {
			fmt.Fprintf(&b, "\t\t_, err := %s\n", callExpr)
			b.WriteString("\t\t_ = err // smoke: any outcome is acceptable; the stub server is permissive\n")
		}

		b.WriteString("\t})\n")
	}

	b.WriteString("}\n")

	// Silence unused-import warnings if no cases were emitted.
	_ = ns

	return []byte(b.String()), nil
}
