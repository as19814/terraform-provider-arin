package arin

import (
	"fmt"
	"net/http"
	"os"
	"regexp"
	"strings"
	"sync"
	"testing"
)

// Read the upstream snapshot rather than deriving expectations from client code
// or fake handlers. The query-auth variant also supplies the documented /phone
// route omitted from the header-auth example; see docs/reference/pocs.md.
type regRWSContract struct {
	method string
	path   *regexp.Regexp
}

func loadRegRWSContracts(t *testing.T) []regRWSContract {
	t.Helper()
	raw, err := os.ReadFile("../../docs/reference/arin-api/reg-rws/methods.md")
	if err != nil {
		t.Fatal(err)
	}
	entries := regexp.MustCompile(`(?s)\*\*Method:\*\* `+"`"+`(GET|POST|PUT|DELETE)`+"`"+`.*?\*\*URL:\*\* ([^\r\n]+)`).FindAllStringSubmatch(string(raw), -1)
	if len(entries) < 90 {
		t.Fatal("upstream method snapshot could not be parsed completely")
	}
	tokens := regexp.MustCompile(`[A-Z][A-Z0-9]+`)
	var contracts []regRWSContract
	for _, entry := range entries {
		path := strings.Split(strings.TrimSpace(entry[2]), "?")[0]
		expression := tokens.ReplaceAllString(regexp.QuoteMeta(path), `[^/;?]+`)
		// The guide explicitly permits omission of NUMBER or TYPE for phone deletion.
		if strings.Contains(path, "/phone/NUMBER;type=TYPE") {
			expression = strings.Replace(expression, `/phone/[^/;?]+;type=[^/;?]+`, `/phone/[^/;?]*(;type=[^/;?]+)?`, 1)
		}
		contracts = append(contracts, regRWSContract{entry[1], regexp.MustCompile("^" + expression + "$")})
	}
	return contracts
}
func contractAllows(contracts []regRWSContract, method, path string) bool {
	for _, contract := range contracts {
		if method == contract.method && contract.path.MatchString(path) {
			return true
		}
	}
	return false
}

type contractTransport struct {
	base      http.RoundTripper
	contracts []regRWSContract
	mu        sync.Mutex
	observed  map[string]bool
}

func (c *contractTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	if !contractAllows(c.contracts, req.Method, req.URL.EscapedPath()) {
		return nil, fmt.Errorf("request violates upstream Reg-RWS method/path contract: %s %s", req.Method, req.URL.EscapedPath())
	}
	c.mu.Lock()
	c.observed[req.Method+" "+req.URL.Path] = true
	c.mu.Unlock()
	return c.base.RoundTrip(req)
}
func TestRegRWSUpstreamContracts(t *testing.T) {
	contracts := loadRegRWSContracts(t)
	for _, bad := range [][2]string{{"POST", "/rest/net/NET-192-0-2-0-1/remove"}, {"POST", "/rest/poc/TEST-ARIN/phone"}, {"POST", "/rest/org/FT-684/poc/TEST-ARIN;pocFunction=T"}} {
		if contractAllows(contracts, bad[0], bad[1]) {
			t.Fatalf("contract accepted wrong verb: %v", bad)
		}
	}
	original := http.DefaultTransport
	audit := &contractTransport{base: original, contracts: contracts, observed: map[string]bool{}}
	http.DefaultTransport = audit
	t.Cleanup(func() { http.DefaultTransport = original })
	// These tests run sequentially. Neither the lifecycle mocks nor client code
	// supplies this transport's expectations.
	for _, suite := range []struct {
		name string
		run  func(*testing.T)
	}{
		{"organization", TestRegisteredOrganizationLifecycle},
		{"organization_pocs", TestOrganizationPOCClientLifecycle},
		{"poc", TestPOCClientLifecycle},
		{"poc_contacts", TestPOCContactOperations},
		{"delegations", TestDelegationFullAndSuboperationRequests},
		{"networks", TestNetAssignmentClientLifecycle},
		{"reports", TestReportRequests},
		{"ticket_status", TestCloseTicket},
		{"ticket_payload", TestCloseTicketWithPayload},
	} {
		t.Run(suite.name, suite.run)
	}
	if len(audit.observed) < 35 {
		t.Fatalf("insufficient independent wire coverage: %d routes", len(audit.observed))
	}
	t.Logf("checked %d distinct method/path combinations against upstream documentation", len(audit.observed))
}

func TestRegRWSContractBlocksPermissiveMock(t *testing.T) {
	// Even a fake accepting every request cannot hide a wrong write verb.
	var dispatched bool
	audit := &contractTransport{
		base: rrdpTestTransport(func(*http.Request) (*http.Response, error) {
			dispatched = true
			return nil, fmt.Errorf("permissive fake was reached")
		}),
		contracts: loadRegRWSContracts(t), observed: map[string]bool{},
	}
	req, err := http.NewRequest(http.MethodPost, "http://127.0.0.1/rest/net/NET-192-0-2-0-1/remove", nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = audit.RoundTrip(req); err == nil || dispatched {
		t.Fatal("wrong write verb reached the permissive fake")
	}
}
