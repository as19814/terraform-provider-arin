.PHONY: build fmt vet test testacc generate check

build:
	go build -o bin/terraform-provider-arin .

fmt:
	gofmt -w main.go internal tools
	terraform fmt -recursive examples

vet:
	go vet ./...

test:
	go test -race ./...

testacc:
	TF_ACC=1 go test -race ./internal/provider -run TestAcc -v -timeout 5m

generate:
	go generate ./...

check: vet test testacc build

# Read-only live tests. Requires ARIN_API_KEY and ARIN_TEST_ORG_HANDLE.
.PHONY: testlive
testlive:
	ARIN_LIVE_TESTS=1 TF_ACC=1 go test ./internal/provider -run '^TestLive' -v -count=1 -timeout 10m

# Explicit opt-in writes, pinned to OT&E. DNS, org and RPKI tests snapshot and restore.
# Serialize packages because client and provider tests share sandbox objects.
.PHONY: testote
testote:
	ARIN_OTE_WRITE_TESTS=1 TF_ACC=1 go test -p 1 ./internal/arin ./internal/provider -run '^TestOTE(ASSet|IRRRoute|IRRRouteMetadata|RouteSet|Autnum|Customer|CustomerNetGraph|Net|NetMultiBlock|NetMetadata|NetRemove|NetAssignmentClient|DelegationClient|Delegation|DelegationNameserver|POC|POCClient|POCContacts|OrgPOC|OrgPOCClient|RPKIClient|RPKIBundle|RPSLClient|RPSLRead|RPSLResource|ASPA|ROA|ReportClient|ReportResource|TicketStatusClient|TicketStatus)Lifecycle$$|^TestOTEOrganizationClientNoChange$$|^TestOTEOrgResourceRecovery$$' -v -count=1 -timeout 10m
