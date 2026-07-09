package main

import (
	"encoding/json"
	"strings"
	"testing"
)

// Shared test-fixture constants. Defined once and referenced by name (rather
// than repeated as raw string literals) across helpers_test.go and
// testgen_test.go so goconst has nothing to flag; the production-facing
// verb/type-name constants (verbHTTP*, schemaType*, goType*, methodPrefixGet,
// namespaceVersion) live in main.go/testgen.go and are reused here directly.
const (
	identNode                  = "node"
	identVmid                  = "vmid"
	identPascalNode            = "Node"
	pathVersion                = "/version"
	pathAccessUsers            = "/access/users"
	pathNodesQemuStatusCurrent = "/nodes/{node}/qemu/{vmid}/status/current"
	methodListUsers            = "ListUsers"
	fieldHostname              = "hostname"
	fieldForce                 = "force"
	fieldUserid                = "userid"
	indexedNetParam            = "net[n]"
	namespaceNodes             = "nodes"
	namespaceAccess            = "access"
	fieldDev0                  = "dev0"
	fieldDev1                  = "dev1"
	fieldDev255                = "dev255"
	fieldSha256                = "sha256"
	fieldSmbios1               = "smbios1"
	fieldArch                  = "arch"
	fieldConsole               = "console"
	fieldServer1               = "server1"
	fieldServer2               = "server2"
)

func TestPascalize(t *testing.T) {
	t.Parallel()

	cases := []struct {
		in   string
		want string
	}{
		{"", ""},
		{identNode, identPascalNode},
		{"redirect-url", "RedirectUrl"},
		{"full_tokenid", "FullTokenid"},
		{"realm.type", "RealmType"},
		{indexedNetParam, "Netn"},
		{"ip[%d]", "Ipd"},
		{"a-b_c.d", "ABCD"},
	}

	for _, tc := range cases {
		if got := pascalize(tc.in); got != tc.want {
			t.Errorf("pascalize(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

func TestCamelize(t *testing.T) {
	t.Parallel()

	cases := []struct {
		in   string
		want string
	}{
		{"", ""},
		{identNode, identNode},
		{identVmid, identVmid},
		{"policy-id", "policyId"},
	}

	for _, tc := range cases {
		if got := camelize(tc.in); got != tc.want {
			t.Errorf("camelize(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

func TestGoIdentSafe(t *testing.T) {
	t.Parallel()

	if got := goIdentSafe("type"); got != "type_" {
		t.Errorf("goIdentSafe(%q) = %q, want %q", "type", got, "type_")
	}

	if got := goIdentSafe(identNode); got != identNode {
		t.Errorf("goIdentSafe(%q) = %q, want %q", identNode, got, identNode)
	}
}

func TestExtractPathParams(t *testing.T) {
	t.Parallel()

	cases := []struct {
		path string
		want []string
	}{
		{pathVersion, nil},
		{pathNodesQemuStatusCurrent, []string{identNode, identVmid}},
		{"/access/domains/{realm}", []string{"realm"}},
	}

	for _, tc := range cases {
		got := extractPathParams(tc.path)
		if !stringSlicesEqual(got, tc.want) {
			t.Errorf("extractPathParams(%q) = %v, want %v", tc.path, got, tc.want)
		}
	}
}

func TestEndsInPathParam(t *testing.T) {
	t.Parallel()

	cases := []struct {
		path string
		want bool
	}{
		{"/nodes/{node}", true},
		{"/nodes/{node}/qemu", false},
		{pathNodesQemuStatusCurrent, false},
		{"/", false},
	}

	for _, tc := range cases {
		if got := endsInPathParam(tc.path); got != tc.want {
			t.Errorf("endsInPathParam(%q) = %v, want %v", tc.path, got, tc.want)
		}
	}
}

func TestNamespaceOf(t *testing.T) {
	t.Parallel()

	cases := []struct {
		path string
		want string
	}{
		{pathVersion, namespaceVersion},
		{"/nodes/{node}/qemu/{vmid}", namespaceNodes},
		{"/", "root"},
	}

	for _, tc := range cases {
		if got := namespaceOf(tc.path); got != tc.want {
			t.Errorf("namespaceOf(%q) = %q, want %q", tc.path, got, tc.want)
		}
	}
}

func TestGoMethodBaseName(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		ep   endpoint
		want string
	}{
		{
			name: "GET dynamic tail on an all-brace path falls back to GetRoot",
			ep:   endpoint{Path: "/{something}", Verb: verbHTTPGet, GoNamespace: "something"},
			want: "GetRoot",
		},
		{
			name: "GET dynamic tail whose resource equals the namespace keeps the full name",
			ep:   endpoint{Path: "/nodes/{node}", Verb: verbHTTPGet, GoNamespace: namespaceNodes},
			want: "GetNodes",
		},
		{
			name: "GET static tail -> List",
			ep:   endpoint{Path: pathAccessUsers, Verb: verbHTTPGet, GoNamespace: namespaceAccess},
			want: methodListUsers,
		},
		{
			name: "POST -> Create",
			ep:   endpoint{Path: pathAccessUsers, Verb: verbHTTPPost, GoNamespace: namespaceAccess},
			want: "CreateUsers",
		},
		{
			name: "PUT -> Update",
			ep:   endpoint{Path: "/access/users/{userid}", Verb: verbHTTPPut, GoNamespace: namespaceAccess},
			want: "UpdateUsers",
		},
		{
			name: "DELETE -> Delete",
			ep:   endpoint{Path: "/access/users/{userid}", Verb: verbHTTPDelete, GoNamespace: namespaceAccess},
			want: "DeleteUsers",
		},
		{
			name: "namespace prefix stripped",
			ep:   endpoint{Path: pathAccessUsers, Verb: verbHTTPGet, GoNamespace: namespaceAccess},
			want: methodListUsers,
		},
	}

	for _, tc := range cases {
		tc.ep.PathParams = extractPathParams(tc.ep.Path)

		if got := goMethodBaseName(tc.ep); got != tc.want {
			t.Errorf("%s: goMethodBaseName() = %q, want %q", tc.name, got, tc.want)
		}
	}
}

func TestAssignMethodNamesResolvesCollisions(t *testing.T) {
	t.Parallel()

	eps := []endpoint{
		{Path: "/pools", Verb: verbHTTPPut, GoNamespace: "pools"},
		{Path: "/pools/{poolid}", Verb: verbHTTPPut, GoNamespace: "pools"},
	}
	for idx := range eps {
		eps[idx].PathParams = extractPathParams(eps[idx].Path)
	}

	assignMethodNames(eps)

	if eps[0].GoMethod != "UpdatePools" {
		t.Errorf("eps[0].GoMethod = %q, want %q", eps[0].GoMethod, "UpdatePools")
	}

	if eps[1].GoMethod != "UpdatePools2" {
		t.Errorf("eps[1].GoMethod = %q, want %q", eps[1].GoMethod, "UpdatePools2")
	}
}

func TestAssignMethodNamesAppliesOverride(t *testing.T) {
	t.Parallel()

	eps := []endpoint{{Path: pathVersion, Verb: verbHTTPGet, GoNamespace: namespaceVersion}}
	assignMethodNames(eps)

	if eps[0].GoMethod != methodPrefixGet {
		t.Errorf("GoMethod = %q, want %q", eps[0].GoMethod, methodPrefixGet)
	}
}

func TestIsOptional(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		raw  string
		want bool
	}{
		{"absent", ``, false},
		{"int 1", `1`, true},
		{"int 0", `0`, false},
		{"string 1", `"1"`, true},
		{"string 0", `"0"`, false},
		{"string true", `"true"`, true},
		{"string TRUE", `"TRUE"`, true},
		{schemaTypeNull, schemaTypeNull, false},
	}

	for _, tc := range cases {
		sch := &schema{}
		if tc.raw != "" {
			sch.Optional = json.RawMessage(tc.raw)
		}

		if got := isOptional(sch); got != tc.want {
			t.Errorf("%s: isOptional() = %v, want %v", tc.name, got, tc.want)
		}
	}

	if isOptional(nil) {
		t.Error("isOptional(nil) = true, want false")
	}
}

func TestSlotFamilyStems(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name  string
		props []string
		want  map[string]bool
	}{
		{
			"numbered family detected",
			[]string{fieldDev0, fieldDev1, fieldDev255},
			map[string]bool{"dev": true},
		},
		{
			"single suffix is not a family",
			[]string{fieldSha256},
			map[string]bool{},
		},
		{
			"single fixed numbered field is not a family",
			[]string{fieldSmbios1},
			map[string]bool{},
		},
		{
			"mixed schema only flags the repeated stem",
			[]string{fieldDev0, fieldDev1, fieldSha256, fieldConsole, fieldArch},
			map[string]bool{"dev": true},
		},
		{
			"multiple families in one schema",
			[]string{"net0", "net31", "mp0", "mp255", "unused0"},
			map[string]bool{"net": true, "mp": true},
		},
		{
			"no numbered properties",
			[]string{fieldArch, fieldConsole, fieldHostname},
			map[string]bool{},
		},
		{
			"fixed one-indexed pair without a zero slot is not a family",
			[]string{fieldServer1, fieldServer2},
			map[string]bool{},
		},
	}

	for _, tc := range cases {
		got := slotFamilyStems(tc.props)
		if len(got) != len(tc.want) {
			t.Errorf("%s: slotFamilyStems(%v) = %v, want %v", tc.name, tc.props, got, tc.want)

			continue
		}

		for stem := range tc.want {
			if !got[stem] {
				t.Errorf("%s: slotFamilyStems(%v) = %v, want %v", tc.name, tc.props, got, tc.want)
			}
		}
	}
}

func TestIsSlotFamilyMember(t *testing.T) {
	t.Parallel()

	families := slotFamilyStems([]string{fieldDev0, fieldDev1, fieldDev255, fieldSha256, fieldSmbios1})

	cases := []struct {
		name string
		want bool
	}{
		{fieldDev0, true},
		{fieldDev255, true},
		{"dev256", true}, // not enumerated above but still a family member
		{fieldSha256, false},
		{fieldSmbios1, false},
		{fieldArch, false},
		{"", false},
	}

	for _, tc := range cases {
		if got := isSlotFamilyMember(tc.name, families); got != tc.want {
			t.Errorf("isSlotFamilyMember(%q) = %v, want %v", tc.name, got, tc.want)
		}
	}
}

func TestIsIndexedParam(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name     string
		wantBase string
		wantOK   bool
	}{
		{indexedNetParam, "net", true},
		{"ip[%d]", "ip", true},
		{fieldHostname, "", false},
		{"net0", "", false},
		{"[n]", "", false},
	}

	for _, tc := range cases {
		base, ok := isIndexedParam(tc.name)
		if ok != tc.wantOK || base != tc.wantBase {
			t.Errorf("isIndexedParam(%q) = (%q, %v), want (%q, %v)", tc.name, base, ok, tc.wantBase, tc.wantOK)
		}
	}
}

func TestGoTypeFor(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name    string
		sch     *schema
		want    string
		wantErr bool
	}{
		{"nil schema", nil, "", true},
		{schemaTypeString, &schema{Type: schemaTypeString}, goTypeString, false},
		{schemaTypeInteger, &schema{Type: schemaTypeInteger}, goTypeInt64, false},
		{schemaTypeNumber, &schema{Type: schemaTypeNumber}, goTypeFloat64, false},
		{schemaTypeBoolean, &schema{Type: schemaTypeBoolean}, goTypeBool, false},
		{"array no items", &schema{Type: schemaTypeArray}, "[]" + goTypeRawMessage, false},
		{
			"array of string",
			&schema{Type: schemaTypeArray, Items: &schema{Type: schemaTypeString}},
			"[]" + goTypeString, false,
		},
		{
			"array of integer",
			&schema{Type: schemaTypeArray, Items: &schema{Type: schemaTypeInteger}},
			"[]" + goTypeInt64, false,
		},
		{"nested object", &schema{Type: schemaTypeObject}, goTypeRawMessage, false},
		{schemaTypeNull, &schema{Type: schemaTypeNull}, goTypeRawMessage, false},
		{"unknown", &schema{Type: "wat"}, goTypeRawMessage, false},
	}

	for _, tc := range cases {
		got, err := goTypeFor(tc.sch)
		if (err != nil) != tc.wantErr {
			t.Errorf("%s: goTypeFor() err = %v, wantErr %v", tc.name, err, tc.wantErr)
		}

		if got != tc.want {
			t.Errorf("%s: goTypeFor() = %q, want %q", tc.name, got, tc.want)
		}
	}
}

func TestIsAlreadyNilable(t *testing.T) {
	t.Parallel()

	cases := []struct {
		goType string
		want   bool
	}{
		{"[]" + goTypeString, true},
		{"map[int]string", true},
		{"*string", true},
		{goTypeRawMessage, true},
		{"interface{}", true},
		{goTypeString, false},
		{goTypeInt64, false},
		{goTypeBool, false},
	}

	for _, tc := range cases {
		if got := isAlreadyNilable(tc.goType); got != tc.want {
			t.Errorf("isAlreadyNilable(%q) = %v, want %v", tc.goType, got, tc.want)
		}
	}
}

func TestResponseBoolAndFloatType(t *testing.T) {
	t.Parallel()

	if got := responseBoolType(goTypeBool); got != "client.PVEBool" {
		t.Errorf("responseBoolType(bool) = %q, want client.PVEBool", got)
	}

	if got := responseBoolType("[]" + goTypeBool); got != "[]client.PVEBool" {
		t.Errorf("responseBoolType([]bool) = %q, want []client.PVEBool", got)
	}

	if got := responseBoolType(goTypeString); got != goTypeString {
		t.Errorf("responseBoolType(string) = %q, want string (unchanged)", got)
	}

	if got := responseFloatType(goTypeFloat64); got != "client.PVEFloat" {
		t.Errorf("responseFloatType(float64) = %q, want client.PVEFloat", got)
	}

	if got := responseFloatType("[]" + goTypeFloat64); got != "[]client.PVEFloat" {
		t.Errorf("responseFloatType([]float64) = %q, want []client.PVEFloat", got)
	}

	if got := responseFloatType(goTypeInt64); got != goTypeInt64 {
		t.Errorf("responseFloatType(int64) = %q, want int64 (unchanged)", got)
	}
}

func TestSanitizeFieldName(t *testing.T) {
	t.Parallel()

	cases := []struct {
		in   string
		want string
	}{
		{"", "Field"},
		{identPascalNode, identPascalNode},
		{"1Foo", "Field1Foo"},
	}

	for _, tc := range cases {
		if got := sanitizeFieldName(tc.in); got != tc.want {
			t.Errorf("sanitizeFieldName(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

func TestHasNonPathParams(t *testing.T) {
	t.Parallel()

	epNoParams := endpoint{Info: endpointInfo{}}
	if hasNonPathParams(epNoParams) {
		t.Error("hasNonPathParams() = true for endpoint with no Parameters schema")
	}

	epOnlyPath := endpoint{
		PathParams: []string{identNode},
		Info: endpointInfo{
			Parameters: &schema{Properties: map[string]*schema{
				identNode: {Type: schemaTypeString},
			}},
		},
	}
	if hasNonPathParams(epOnlyPath) {
		t.Error("hasNonPathParams() = true when every property is a path parameter")
	}

	epWithBody := endpoint{
		PathParams: []string{identNode},
		Info: endpointInfo{
			Parameters: &schema{Properties: map[string]*schema{
				identNode:     {Type: schemaTypeString},
				fieldHostname: {Type: schemaTypeString},
			}},
		},
	}
	if !hasNonPathParams(epWithBody) {
		t.Error("hasNonPathParams() = false when a non-path property exists")
	}
}

func TestBuildPathExpression(t *testing.T) {
	t.Parallel()

	noParams := endpoint{Path: pathVersion}
	if got := buildPathExpression(noParams); got != `"/version"` {
		t.Errorf("buildPathExpression(no params) = %q, want %q", got, `"/version"`)
	}

	withParams := endpoint{Path: "/nodes/{node}/qemu/{vmid}", PathParams: []string{identNode, identVmid}}

	want := `fmt.Sprintf("/nodes/%s/qemu/%s", url.PathEscape(node), url.PathEscape(vmid))`
	if got := buildPathExpression(withParams); got != want {
		t.Errorf("buildPathExpression(with params) = %q, want %q", got, want)
	}
}

func TestBuildWantNS(t *testing.T) {
	t.Parallel()

	byNS := map[string][]endpoint{namespaceAccess: {{}}, namespaceNodes: {{}}}

	all := buildWantNS(nil, byNS)
	if len(all) != 2 || !all[namespaceAccess] || !all[namespaceNodes] {
		t.Errorf("buildWantNS(nil) = %v, want both namespaces selected", all)
	}

	subset := buildWantNS(stringSlice{namespaceAccess}, byNS)
	if len(subset) != 1 || !subset[namespaceAccess] {
		t.Errorf("buildWantNS([access]) = %v, want only access selected", subset)
	}

	unknown := buildWantNS(stringSlice{"bogus"}, byNS)
	if len(unknown) != 0 {
		t.Errorf("buildWantNS([bogus]) = %v, want empty (unknown namespace skipped)", unknown)
	}
}

func TestGroupByNamespace(t *testing.T) {
	t.Parallel()

	eps := []endpoint{
		{GoNamespace: namespaceAccess, Path: pathAccessUsers},
		{GoNamespace: namespaceNodes, Path: "/nodes"},
		{GoNamespace: namespaceAccess, Path: "/access/roles"},
	}

	got := groupByNamespace(eps)
	if len(got[namespaceAccess]) != 2 {
		t.Errorf("groupByNamespace() access bucket len = %d, want 2", len(got[namespaceAccess]))
	}

	if len(got[namespaceNodes]) != 1 {
		t.Errorf("groupByNamespace() nodes bucket len = %d, want 1", len(got[namespaceNodes]))
	}
}

func TestEscapeDoc(t *testing.T) {
	t.Parallel()

	in := "line one\nline\ttwo\r\nend  "
	want := "line one line two end"

	if got := escapeDoc(in); got != want {
		t.Errorf("escapeDoc(%q) = %q, want %q", in, got, want)
	}
}

func TestIsResponseEmptyOk(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		ret  *schema
		want bool
	}{
		{"nil returns", nil, true},
		{
			"optional populated object honors schema flag",
			&schema{
				Type:     schemaTypeObject,
				Optional: json.RawMessage("1"),
				Properties: map[string]*schema{
					fieldValue: {Type: schemaTypeString},
				},
			},
			true,
		},
		{
			"required populated object stays strict",
			&schema{
				Type: schemaTypeObject,
				Properties: map[string]*schema{
					fieldValue: {Type: schemaTypeString},
				},
			},
			false,
		},
		{"array", &schema{Type: schemaTypeArray}, true},
		{"empty object", &schema{Type: schemaTypeObject}, true},
		{"scalar", &schema{Type: schemaTypeInteger}, true},
	}

	for _, tc := range cases {
		endpt := endpoint{Info: endpointInfo{Returns: tc.ret}}
		if got := isResponseEmptyOk(endpt); got != tc.want {
			t.Errorf("%s: isResponseEmptyOk() = %v, want %v", tc.name, got, tc.want)
		}
	}
}

func TestRenderObjectFieldsSlotFamilyOptionality(t *testing.T) {
	t.Parallel()

	objSchema := &schema{
		Type: schemaTypeObject,
		Properties: map[string]*schema{
			fieldDev0:   {Type: schemaTypeString},
			fieldDev1:   {Type: schemaTypeString},
			fieldDev255: {Type: schemaTypeString},
			fieldArch:   {Type: schemaTypeString, Optional: json.RawMessage("1")},
			fieldSha256: {Type: schemaTypeString},
		},
	}

	body, err := renderObjectFields(objSchema)
	if err != nil {
		t.Fatalf("renderObjectFields() error = %v", err)
	}

	// Slot-family members become pointer fields with an omitempty tag even
	// though the apidoc never marked them optional.
	for _, want := range []string{
		"Dev0 *string `json:\"dev0,omitempty\"`",
		"Dev1 *string `json:\"dev1,omitempty\"`",
		"Dev255 *string `json:\"dev255,omitempty\"`",
		"Arch *string `json:\"arch,omitempty\"`",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("renderObjectFields() missing %q in:\n%s", want, body)
		}
	}

	// A single word+digits name with no sibling suffix is not a family and
	// stays required (non-pointer, no omitempty) since the apidoc did not
	// mark it optional.
	if !strings.Contains(body, "Sha256 string `json:\"sha256\"`") {
		t.Errorf("renderObjectFields() unexpectedly treated sha256 as a slot family in:\n%s", body)
	}
}

// TestRenderObjectFieldsFixedPairIsNotASlotFamily guards against the
// AD/LDAP realm config regression found while wiring in the slot-family
// heuristic: "server1"/"server2" (primary/fallback server address) share a
// stem and two distinct suffixes but are a fixed one-indexed pair, not a
// zero-indexed slot family, and the apidoc already marks "server1" required
// on creation. Without the "0" suffix guard in slotFamilyStems this would
// wrongly become a pointer/omitempty field.
func TestRenderObjectFieldsFixedPairIsNotASlotFamily(t *testing.T) {
	t.Parallel()

	objSchema := &schema{
		Type: schemaTypeObject,
		Properties: map[string]*schema{
			fieldServer1: {Type: schemaTypeString},
			fieldServer2: {Type: schemaTypeString, Optional: json.RawMessage("1")},
		},
	}

	body, err := renderObjectFields(objSchema)
	if err != nil {
		t.Fatalf("renderObjectFields() error = %v", err)
	}

	if !strings.Contains(body, "Server1 string `json:\"server1\"`") {
		t.Errorf("renderObjectFields() unexpectedly treated server1 as a slot family in:\n%s", body)
	}

	if !strings.Contains(body, "Server2 *string `json:\"server2,omitempty\"`") {
		t.Errorf("renderObjectFields() lost server2's own apidoc-marked optionality in:\n%s", body)
	}
}

// withDialect swaps activeDialect for the duration of the calling test and
// restores the original on cleanup. Not safe to use from a t.Parallel()
// test: activeDialect is a package-level global read by other tests.
func withDialect(t *testing.T, cfg *dialectConfig) {
	t.Helper()

	orig := activeDialect
	activeDialect = cfg

	t.Cleanup(func() { activeDialect = orig })
}

//nolint:paralleltest // mutates the package-level activeDialect global via withDialect
func TestCollectEndpointsAppliesReturnsOverride(t *testing.T) {
	overridden := &schema{Type: schemaTypeArray, Items: &schema{Type: schemaTypeString}}

	withDialect(t, &dialectConfig{
		returnsOverrides: map[string]*schema{
			"GET /nodes/{node}/journal": overridden,
		},
	})

	tree := []*node{
		{
			Path: "/nodes/{node}/journal",
			Info: map[string]endpointInfo{
				verbHTTPGet: {Returns: &schema{Type: schemaTypeNull}},
			},
		},
		{
			Path: pathNodesQemuStatusCurrent,
			Info: map[string]endpointInfo{
				verbHTTPGet: {Returns: &schema{Type: schemaTypeObject, Properties: map[string]*schema{
					fieldHostname: {Type: schemaTypeString},
				}}},
			},
		},
	}

	eps := collectEndpoints(tree)
	if len(eps) != 2 {
		t.Fatalf("collectEndpoints() returned %d endpoints, want 2", len(eps))
	}

	for _, gotEndpoint := range eps {
		switch gotEndpoint.Path {
		case "/nodes/{node}/journal":
			if gotEndpoint.Info.Returns != overridden {
				t.Errorf("overridden endpoint: Info.Returns = %+v, want the override schema", gotEndpoint.Info.Returns)
			}
		case pathNodesQemuStatusCurrent:
			if gotEndpoint.Info.Returns.Type != schemaTypeObject || len(gotEndpoint.Info.Returns.Properties) != 1 {
				t.Errorf("non-overridden endpoint: Info.Returns changed unexpectedly: %+v", gotEndpoint.Info.Returns)
			}
		default:
			t.Errorf("unexpected endpoint path %q", gotEndpoint.Path)
		}
	}
}

//nolint:paralleltest // mutates the package-level activeDialect global via withDialect
func TestCollectEndpointsNoOverrideTableLeavesReturnsUntouched(t *testing.T) {
	original := &schema{Type: schemaTypeObject, Properties: map[string]*schema{
		fieldHostname: {Type: schemaTypeString},
	}}

	withDialect(t, &dialectConfig{returnsOverrides: map[string]*schema{}})

	tree := []*node{
		{
			Path: pathNodesQemuStatusCurrent,
			Info: map[string]endpointInfo{
				verbHTTPGet: {Returns: original},
			},
		},
	}

	eps := collectEndpoints(tree)
	if len(eps) != 1 || eps[0].Info.Returns != original {
		t.Errorf("collectEndpoints() with empty override table changed Info.Returns: got %+v", eps)
	}
}

func stringSlicesEqual(left, right []string) bool {
	if len(left) != len(right) {
		return false
	}

	for i := range left {
		if left[i] != right[i] {
			return false
		}
	}

	return true
}
