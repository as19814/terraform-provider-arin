package provider

import (
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"testing"

	"github.com/as19814/terraform-provider-arin/internal/arin"
	"github.com/hashicorp/terraform-plugin-framework/providerserver"
	"github.com/hashicorp/terraform-plugin-go/tfprotov6"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
)

type rpslFake struct {
	mu               sync.Mutex
	objects          map[string]string
	writes           map[string]int
	lost, unreadable bool
}

func (f *rpslFake) serve(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if r.Header.Get("Authorization") != "ApiKey test-key" || r.Header.Get("Content-Type") != "application/rpsl" || r.Header.Get("Accept") != "application/rpsl" {
		w.WriteHeader(400)
		return
	}
	switch r.Method {
	case "GET":
		if f.unreadable && len(f.objects) > 0 {
			w.WriteHeader(503)
			return
		}
		body, ok := f.objects[r.URL.Path]
		if !ok {
			w.WriteHeader(404)
			return
		}
		fmt.Fprint(w, body)
	case "POST", "PUT":
		b, _ := io.ReadAll(r.Body)
		obj, err := arin.ParseRPSL(string(b))
		if err != nil {
			w.WriteHeader(400)
			return
		}
		endpoint := "/rest/irr/" + obj.Key.Kind + "/" + obj.Key.Name
		if obj.Key.OriginAS != "" {
			endpoint = "/rest/irr/route/" + obj.Key.Name + "/" + obj.Key.OriginAS
		}
		_, exists := f.objects[endpoint]
		if (r.Method == "POST" && exists) || (r.Method == "PUT" && !exists) {
			w.WriteHeader(409)
			return
		}
		f.writes[r.Method]++
		body := strings.ReplaceAll(obj.Text, ": ", ":          ") + "created: 2026-01-01T00:00:00Z\nlast-modified: 2026-09-23T00:00:00Z\n"
		f.objects[endpoint] = body
		if f.lost {
			w.WriteHeader(500)
			return
		}
		fmt.Fprint(w, body)
	case "DELETE":
		f.writes[r.Method]++
		delete(f.objects, r.URL.Path)
		if f.lost {
			w.WriteHeader(500)
			return
		}
		w.WriteHeader(204)
	default:
		w.WriteHeader(405)
	}
}
func setupRPSLFake(t *testing.T) *rpslFake {
	t.Helper()
	f := &rpslFake{objects: map[string]string{}, writes: map[string]int{}}
	server := httptest.NewServer(http.HandlerFunc(f.serve))
	t.Cleanup(server.Close)
	t.Setenv("ARIN_API_KEY", "test-key")
	t.Setenv("ARIN_BASE_URL", server.URL)
	t.Setenv("ARIN_RDAP_BASE_URL", "")
	return f
}
func rpslResourceConfig(kind, name, description string) string {
	origin, line := "", ""
	if kind == "route" || kind == "route6" {
		origin = "AS64496"
		line = "origin: AS64496\n"
	}
	raw := fmt.Sprintf("%s: %s\n%sdescr: %s\nmnt-by: MNT-EXAMPLE-1\nsource: ARIN\n", kind, name, line, description)
	return fmt.Sprintf("provider \"arin\" {}\nresource \"arin_irr_rpsl\" \"test\" {\nobject_type=%q\nname=%q\norigin_as=%q\norg_handle=\"EXAMPLE-1\"\nrpsl=%q\n}\n", kind, name, origin, raw)
}
func TestAccRPSLResourceLifecycle(t *testing.T) {
	for kind, name := range map[string]string{"as-set": "AS-EXAMPLE", "route-set": "RS-EXAMPLE", "aut-num": "AS64496", "route": "192.0.2.0/24", "route6": "2001:db8::/48"} {
		t.Run(kind, func(t *testing.T) {
			f := setupRPSLFake(t)
			initial := rpslResourceConfig(kind, name, "Initial policy")
			updated := rpslResourceConfig(kind, name, "Updated policy")
			resource.Test(t, resource.TestCase{ProtoV6ProviderFactories: map[string]func() (tfprotov6.ProviderServer, error){"arin": providerserver.NewProtocol6WithError(New("test")())}, Steps: []resource.TestStep{
				{Config: initial, Check: resource.TestCheckResourceAttr("arin_irr_rpsl.test", "pending_creation", "false")},
				{Config: initial, PlanOnly: true},
				{ResourceName: "arin_irr_rpsl.test", ImportState: true, ImportStateVerify: true, ImportStateVerifyIgnore: []string{"rpsl"}},
				{Config: updated},
				{Config: updated, PreConfig: func() {
					f.mu.Lock()
					defer f.mu.Unlock()
					for key, value := range f.objects {
						f.objects[key] = strings.Replace(value, "Updated policy", "External change", 1)
					}
				}},
				{Config: updated, PlanOnly: true},
			}, CheckDestroy: func(_ *terraform.State) error {
				f.mu.Lock()
				defer f.mu.Unlock()
				if len(f.objects) != 0 || f.writes["POST"] != 1 || f.writes["PUT"] != 2 || f.writes["DELETE"] != 1 {
					return fmt.Errorf("incorrect lifecycle writes: %v", f.writes)
				}
				return nil
			}})
		})
	}
}
func TestAccRPSLImmediateRecovery(t *testing.T) {
	f := setupRPSLFake(t)
	f.lost = true
	config := rpslResourceConfig("as-set", "AS-EXAMPLE", "Example")
	updated := rpslResourceConfig("as-set", "AS-EXAMPLE", "Updated")
	resource.Test(t, resource.TestCase{ProtoV6ProviderFactories: map[string]func() (tfprotov6.ProviderServer, error){"arin": providerserver.NewProtocol6WithError(New("test")())}, Steps: []resource.TestStep{{Config: config}, {Config: updated}, {Config: updated, PlanOnly: true}}, CheckDestroy: func(_ *terraform.State) error {
		f.mu.Lock()
		defer f.mu.Unlock()
		if f.writes["POST"] != 1 || f.writes["PUT"] != 1 || f.writes["DELETE"] != 1 || len(f.objects) != 0 {
			return fmt.Errorf("recovery repeated creation or leaked an object")
		}
		return nil
	}})
}
func TestAccRPSLPendingRecovery(t *testing.T) {
	f := setupRPSLFake(t)
	f.lost = true
	f.unreadable = true
	config := strings.Replace(rpslResourceConfig("as-set", "AS-EXAMPLE", "Example"), "\n}\n", "\n lifecycle { create_before_destroy = true }\n}\n", 1)
	workdir := t.TempDir()
	resource.Test(t, resource.TestCase{WorkingDir: workdir, ProtoV6ProviderFactories: map[string]func() (tfprotov6.ProviderServer, error){"arin": providerserver.NewProtocol6WithError(New("test")())}, Steps: []resource.TestStep{
		{Config: config, ExpectError: regexp.MustCompile("Could not confirm advanced IRR creation")},
		{Config: config, PreConfig: func() { f.mu.Lock(); defer f.mu.Unlock(); f.unreadable = false; f.lost = false }, ExpectError: regexp.MustCompile("Uncertain advanced IRR creation")},
		{PreConfig: func() {
			matches, err := filepath.Glob(filepath.Join(workdir, "work*", "terraform.tfstate"))
			if err != nil || len(matches) != 1 {
				t.Fatal("cannot locate isolated test state")
			}
			command := exec.Command("terraform", "-chdir="+filepath.Dir(matches[0]), "state", "rm", "arin_irr_rpsl.test")
			if output, err := command.CombinedOutput(); err != nil {
				t.Fatalf("remove test receipt: %v: %s", err, output)
			}
		}, ResourceName: "arin_irr_rpsl.test", ImportState: true, ImportStateId: "as-set/AS-EXAMPLE", ImportStatePersist: true},
		{Config: config}, // Accept configured formatting locally after import, without a PUT.
		{Config: config, PlanOnly: true},
	}, CheckDestroy: func(_ *terraform.State) error {
		f.mu.Lock()
		defer f.mu.Unlock()
		if f.writes["POST"] != 1 || f.writes["PUT"] != 0 || f.writes["DELETE"] != 1 || len(f.objects) != 0 {
			return fmt.Errorf("recovery mutated unexpectedly: %v", f.writes)
		}
		return nil
	}})
}

func TestAccRPSLIdentityReplacement(t *testing.T) {
	f := setupRPSLFake(t)
	resource.Test(t, resource.TestCase{ProtoV6ProviderFactories: map[string]func() (tfprotov6.ProviderServer, error){"arin": providerserver.NewProtocol6WithError(New("test")())}, Steps: []resource.TestStep{
		{Config: rpslResourceConfig("as-set", "AS-EXAMPLE", "Initial")},
		{Config: rpslResourceConfig("as-set", "AS-OTHER", "Replacement"), Check: resource.TestCheckResourceAttr("arin_irr_rpsl.test", "id", "as-set/AS-OTHER")},
	}, CheckDestroy: func(_ *terraform.State) error {
		f.mu.Lock()
		defer f.mu.Unlock()
		if len(f.objects) != 0 || f.writes["POST"] != 2 || f.writes["PUT"] != 0 || f.writes["DELETE"] != 2 {
			return fmt.Errorf("identity change did not replace correctly: %v", f.writes)
		}
		return nil
	}})
}
func TestAccRPSLExistingObjectRequiresImport(t *testing.T) {
	f := setupRPSLFake(t)
	f.objects["/rest/irr/as-set/AS-EXAMPLE"] = "as-set: AS-EXAMPLE\ndescr: Existing\nmnt-by: MNT-EXAMPLE-1\nsource: ARIN\n"
	resource.Test(t, resource.TestCase{ProtoV6ProviderFactories: map[string]func() (tfprotov6.ProviderServer, error){"arin": providerserver.NewProtocol6WithError(New("test")())}, Steps: []resource.TestStep{
		{Config: rpslResourceConfig("as-set", "AS-EXAMPLE", "Existing"), ExpectError: regexp.MustCompile("already exists")},
	}})
	f.mu.Lock()
	defer f.mu.Unlock()
	if len(f.writes) != 0 || len(f.objects) != 1 {
		t.Fatal("existing object adopted or changed without import")
	}
}
