package pdm_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"testing"

	"github.com/fivetwenty-io/proxmox-apiclient-go/v3/pkg/client"
	"github.com/fivetwenty-io/proxmox-apiclient-go/v3/pkg/pdm"
	"github.com/fivetwenty-io/proxmox-apiclient-go/v3/pkg/pdm/autoinstall"
	"github.com/fivetwenty-io/proxmox-apiclient-go/v3/pkg/pdm/sdn"
	"github.com/fivetwenty-io/proxmox-apiclient-go/v3/pkg/pdm/subscriptions"
)

const (
	rawWireTestAPIToken = "wiretest@pdm!tok=secret"
	rawWireRespNoData   = `{"data":null}`
	rawWireNetdevFilter = "netdev-filter"
)

// rawWireCase is one table-driven shape for TestRawMessageParams_WireFormat:
// a generated call carrying json.RawMessage/[]json.RawMessage params, plus
// the exact form values the request must (or must not) carry on the wire.
type rawWireCase struct {
	name       string
	respBody   string
	call       func(ctx context.Context, cli client.Client) error
	wantForm   map[string]string
	absentForm []string
}

// TestRawMessageParams_WireFormat proves that json.RawMessage- and
// []json.RawMessage-typed request fields reach the wire as their compact
// JSON text in a single form value.
//
// Before the generator emitted the override exercised here, every generated
// POST/PUT method built its form body by marshaling the whole params
// struct to JSON and re-decoding it into a map[string]interface{}. That
// round-trip destroys json.RawMessage: an object-valued field decodes into
// a Go map, which the transport then serializes as a Proxmox
// comma-joined "k=v,k=v" option-string (encodeNestedMap); an array-valued
// field decodes into []interface{}, whose object elements have no encoder
// case and fall through to Go's default %v formatting. Neither is valid
// JSON, so a real PDM server rejects (or misparses) the request. Each case
// below matches the wire format the pmx-cli workarounds
// (internal/cli/pdm/autoinstall.go's applyCreate/applyOptional and
// internal/cli/pdm/sdn.go's encodeRemoteZonePairs/
// encodeRemoteControllerPairs) used to send by hand while this defect was
// unfixed.
func TestRawMessageParams_WireFormat(t *testing.T) {
	t.Parallel()

	for _, tc := range rawWireCases() {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			runRawWireCase(t, tc)
		})
	}
}

func rawWireCases() []rawWireCase {
	return []rawWireCase{
		rawWireCreatePreparedCase(),
		rawWireUpdatePreparedCase(),
		rawWireCreateVnetsCase(),
		rawWireCreateZonesCase(),
		rawWireCreateBulkAssignCase(),
	}
}

// rawWireCreatePreparedCase covers the required Filesystem field plus the
// four optional RawMessage fields CreatePreparedParams carries.
func rawWireCreatePreparedCase() rawWireCase {
	return rawWireCase{
		name:     "CreatePrepared: required Filesystem plus four optional RawMessage fields",
		respBody: `{"data":{"config":{}}}`,
		call: func(ctx context.Context, cli client.Client) error {
			_, err := autoinstall.New(cli).CreatePrepared(ctx, &autoinstall.CreatePreparedParams{
				Country: "us", DiskMode: "disk-list", Fqdn: "host.example.com", Id: "prep1",
				Keyboard: "en-us", Mailto: "ops@example.com", RebootMode: "reboot",
				Timezone: "UTC", NetifNamePinningEnabled: true, RebootOnError: true,
				UseDhcpFqdn: false, UseDhcpNetwork: false,
				Filesystem:       json.RawMessage(`{  "filesystem" : "zfs" , "ashift":12 }`),
				DiskFilter:       json.RawMessage(`{"ID_BUS":"ata"}`),
				NetdevFilter:     json.RawMessage(`{"ID_NET_NAME":"eth0"}`),
				TargetFilter:     json.RawMessage(`{"/foo":"bar*"}`),
				TemplateCounters: json.RawMessage(`{"a":1,"b":2}`),
			})
			if err != nil {
				return fmt.Errorf("create prepared: %w", err)
			}

			return nil
		},
		wantForm: map[string]string{
			"filesystem":        `{"filesystem":"zfs","ashift":12}`,
			"disk-filter":       `{"ID_BUS":"ata"}`,
			rawWireNetdevFilter: `{"ID_NET_NAME":"eth0"}`,
			"target-filter":     `{"/foo":"bar*"}`,
			"template-counters": `{"a":1,"b":2}`,
		},
	}
}

// rawWireUpdatePreparedCase covers a partial update where only some of the
// optional RawMessage fields are set; the untouched ones must not appear.
func rawWireUpdatePreparedCase() rawWireCase {
	return rawWireCase{
		name:     "UpdatePrepared: only the touched optional RawMessage fields are sent",
		respBody: `{"data":{"config":{}}}`,
		call: func(ctx context.Context, cli client.Client) error {
			_, err := autoinstall.New(cli).UpdatePrepared(ctx, "prep1", &autoinstall.UpdatePreparedParams{
				Filesystem: json.RawMessage(`{"filesystem":"ext4"}`),
				DiskFilter: json.RawMessage(`{"ID_BUS":"nvme"}`),
				Delete:     []string{rawWireNetdevFilter},
			})
			if err != nil {
				return fmt.Errorf("update prepared: %w", err)
			}

			return nil
		},
		wantForm: map[string]string{
			"filesystem":  `{"filesystem":"ext4"}`,
			"disk-filter": `{"ID_BUS":"nvme"}`,
			"delete":      rawWireNetdevFilter,
		},
		absentForm: []string{"target-filter", "template-counters"},
	}
}

// rawWireCreateVnetsCase covers []json.RawMessage Remotes: the whole slice
// must land as one compact JSON array form value, per sdn.go's
// encodeRemoteZonePairs oracle.
func rawWireCreateVnetsCase() rawWireCase {
	return rawWireCase{
		name:     "CreateVnets: []json.RawMessage Remotes as one compact JSON array form value",
		respBody: rawWireRespNoData,
		call: func(ctx context.Context, cli client.Client) error {
			tag := int64(100)

			_, err := sdn.New(cli).CreateVnets(ctx, &sdn.CreateVnetsParams{
				Vnet: "vnet1",
				Tag:  &tag,
				Remotes: []json.RawMessage{
					json.RawMessage(`{"remote":"alpha","zone":"zone1"}`),
					json.RawMessage(`{ "remote" : "beta" , "zone" : "zone2" }`),
				},
			})
			if err != nil {
				return fmt.Errorf("create vnets: %w", err)
			}

			return nil
		},
		wantForm: map[string]string{
			"remotes": `[{"remote":"alpha","zone":"zone1"},{"remote":"beta","zone":"zone2"}]`,
			"vnet":    "vnet1",
		},
	}
}

// rawWireCreateZonesCase covers []json.RawMessage Remotes, per sdn.go's
// encodeRemoteControllerPairs oracle (controller is optional per entry).
func rawWireCreateZonesCase() rawWireCase {
	return rawWireCase{
		name:     "CreateZones: []json.RawMessage Remotes as one compact JSON array form value",
		respBody: rawWireRespNoData,
		call: func(ctx context.Context, cli client.Client) error {
			_, err := sdn.New(cli).CreateZones(ctx, &sdn.CreateZonesParams{
				Zone: "zone1",
				Remotes: []json.RawMessage{
					json.RawMessage(`{"remote":"alpha","controller":"c1"}`),
					json.RawMessage(`{"remote":"beta"}`),
				},
			})
			if err != nil {
				return fmt.Errorf("create zones: %w", err)
			}

			return nil
		},
		wantForm: map[string]string{
			"remotes": `[{"remote":"alpha","controller":"c1"},{"remote":"beta"}]`,
			"zone":    "zone1",
		},
	}
}

// rawWireCreateBulkAssignCase covers a scalar required json.RawMessage
// object field.
func rawWireCreateBulkAssignCase() rawWireCase {
	return rawWireCase{
		name:     "CreateBulkAssign: required Proposal RawMessage object",
		respBody: rawWireRespNoData,
		call: func(ctx context.Context, cli client.Client) error {
			_, err := subscriptions.New(cli).CreateBulkAssign(ctx, &subscriptions.CreateBulkAssignParams{
				Proposal: json.RawMessage(`{"keys_digest":"abc","node_status_digest":"def","assignments":[]}`),
			})
			if err != nil {
				return fmt.Errorf("create bulk assign: %w", err)
			}

			return nil
		},
		wantForm: map[string]string{
			"proposal": `{"keys_digest":"abc","node_status_digest":"def","assignments":[]}`,
		},
	}
}

// rawWireServer captures the form values a rawWireCase's call sent, and any
// error parsing them, from a single handled request.
type rawWireServer struct {
	form    url.Values
	formErr error
}

// newRawWireTestServer starts an httptest server that replies with respBody
// to every request and records the parsed form of the last one received,
// then builds a PDM client pointed at it.
func newRawWireTestServer(t *testing.T, respBody string) (client.Client, *rawWireServer) { //nolint:ireturn // test helper mirrors pdm.NewClient's factory return type
	t.Helper()

	srv := &rawWireServer{}

	testServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		srv.formErr = r.ParseForm()
		srv.form = r.PostForm

		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(respBody))
	}))
	t.Cleanup(testServer.Close)

	parsed, err := url.Parse(testServer.URL)
	if err != nil {
		t.Fatalf("parse server URL: %v", err)
	}

	port, err := strconv.Atoi(parsed.Port())
	if err != nil {
		t.Fatalf("parse server port: %v", err)
	}

	cli, err := pdm.NewClient(client.Options{
		Host:     parsed.Hostname(),
		Port:     port,
		Protocol: "http",
		APIToken: rawWireTestAPIToken,
	})
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}

	return cli, srv
}

// runRawWireCase executes tc's call against a fresh test server and asserts
// the resulting form matches tc.wantForm/tc.absentForm exactly.
func runRawWireCase(t *testing.T, tc rawWireCase) {
	t.Helper()

	cli, srv := newRawWireTestServer(t, tc.respBody)

	err := tc.call(context.Background(), cli)
	if err != nil {
		t.Fatalf("%s: unexpected error: %v", tc.name, err)
	}

	if srv.formErr != nil {
		t.Fatalf("server: parse form: %v", srv.formErr)
	}

	for key, want := range tc.wantForm {
		if got := srv.form.Get(key); got != want {
			t.Errorf("form[%q] = %q, want %q", key, got, want)
		}
	}

	for _, key := range tc.absentForm {
		if srv.form.Has(key) {
			t.Errorf("form[%q] = %q, want key absent", key, srv.form.Get(key))
		}
	}
}
