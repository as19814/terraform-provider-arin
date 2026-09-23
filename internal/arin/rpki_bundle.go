package arin

import (
	"errors"
	"fmt"
	"maps"
	"slices"
	"strings"
)

// RPKIBundleROA describes one desired authorization and its local removal policy.
type RPKIBundleROA struct {
	Request            ROARequest
	DeleteLinkedRoutes bool
}
type RPKIBundleDesired struct {
	ROAs  map[string]RPKIBundleROA
	ASPAs map[int64]ASPA
}
type RPKIBundleOwnedROA struct {
	Handle             string
	DeleteLinkedRoutes bool
}
type RPKIBundleOwnership struct {
	ROAs  map[string]RPKIBundleOwnedROA
	ASPAs []int64
}
type RPKIBundleInventory struct {
	ROAs  []ROA
	ASPAs []ASPA
}

// RPKIBundlePlan is a serializable pre-write receipt. It contains no API key and
// no unrelated object contents. Keep it until the complete postcondition holds.
type RPKIBundlePlan struct {
	Desired            RPKIBundleDesired
	Prior              RPKIBundleOwnership
	Transaction        RPKITransaction
	BaselineROAHandles []string
	RetainedROAs       map[string]string
	AddedROALabels     []string
}
type RPKIBundleState struct {
	ROAs  map[string]ROA
	ASPAs map[int64]ASPA
}

func (d RPKIBundleDesired) Validate() error {
	// Empty desired state is intentional for destroy. Resource configuration must
	// separately require at least one member.
	origins := map[string]bool{}
	for label, entry := range d.ROAs {
		if strings.TrimSpace(label) == "" || strings.ContainsAny(label, "\r\n") {
			return errors.New("ROA labels must be nonempty single lines")
		}
		if err := entry.Request.Validate(); err != nil {
			return err
		}
		for _, resource := range entry.Request.Resources {
			key := fmt.Sprintf("%d:%s", entry.Request.ASN, resource.Prefix)
			if origins[key] {
				return errors.New("bundle ROAs must not repeat an origin ASN and prefix")
			}
			origins[key] = true
		}
	}
	for customer, entry := range d.ASPAs {
		if customer != entry.CustomerASN {
			return errors.New("ASPA map key differs from customer identity")
		}
		if err := entry.Validate(); err != nil {
			return err
		}
	}
	return nil
}
func (o RPKIBundleOwnership) validate() error {
	handles := map[string]bool{}
	for label, entry := range o.ROAs {
		if strings.TrimSpace(label) == "" || !handlePattern.MatchString(entry.Handle) || handles[entry.Handle] {
			return errors.New("invalid or repeated bundle ROA ownership")
		}
		handles[entry.Handle] = true
	}
	customers := map[int64]bool{}
	for _, customer := range o.ASPAs {
		if validateRPKIASN(customer, false) != nil || customers[customer] {
			return errors.New("invalid or repeated bundle ASPA ownership")
		}
		customers[customer] = true
	}
	return nil
}
func bundleInventory(i RPKIBundleInventory) (map[string]ROA, map[int64]ASPA, error) {
	roas, aspas := map[string]ROA{}, map[int64]ASPA{}
	for _, r := range i.ROAs {
		if _, ok := roas[r.Handle]; ok || !handlePattern.MatchString(r.Handle) {
			return nil, nil, errors.New("invalid or duplicate ROA inventory identity")
		}
		if err := (ROARequest{Name: r.Name, ASN: r.ASN, Resources: r.Resources}).Validate(); err != nil {
			return nil, nil, err
		}
		roas[r.Handle] = r
	}
	for _, a := range i.ASPAs {
		if _, ok := aspas[a.CustomerASN]; ok {
			return nil, nil, errors.New("duplicate ASPA inventory identity")
		}
		if err := a.Validate(); err != nil {
			return nil, nil, err
		}
		aspas[a.CustomerASN] = a
	}
	return roas, aspas, nil
}
func cloneBundleDesired(d RPKIBundleDesired) RPKIBundleDesired {
	out := RPKIBundleDesired{ROAs: map[string]RPKIBundleROA{}, ASPAs: map[int64]ASPA{}}
	for label, entry := range d.ROAs {
		entry.Request.Resources = slices.Clone(entry.Request.Resources)
		for j, r := range entry.Request.Resources {
			if r.MaxLength != nil {
				value := *r.MaxLength
				entry.Request.Resources[j].MaxLength = &value
			}
		}
		out.ROAs[label] = entry
	}
	for customer, entry := range d.ASPAs {
		entry.ProviderASNs = slices.Clone(entry.ProviderASNs)
		out.ASPAs[customer] = entry
	}
	return out
}

// PlanRPKIBundle does not write. Only identities explicitly owned by prior can
// be deleted or replaced. Callers must fetch complete inventories, not use an
// empty inventory as a fallback for a failed read.
func PlanRPKIBundle(prior RPKIBundleOwnership, desired RPKIBundleDesired, inventory RPKIBundleInventory) (*RPKIBundlePlan, error) {
	if err := prior.validate(); err != nil {
		return nil, err
	}
	if err := desired.Validate(); err != nil {
		return nil, err
	}
	roas, aspas, err := bundleInventory(inventory)
	if err != nil {
		return nil, err
	}
	desired = cloneBundleDesired(desired)
	prior = RPKIBundleOwnership{ROAs: maps.Clone(prior.ROAs), ASPAs: slices.Clone(prior.ASPAs)}
	slices.Sort(prior.ASPAs)
	plan := &RPKIBundlePlan{Desired: desired, Prior: prior, BaselineROAHandles: slices.Sorted(maps.Keys(roas)), RetainedROAs: map[string]string{}}
	deleting := map[string]bool{}
	for _, label := range slices.Sorted(maps.Keys(prior.ROAs)) {
		owned := prior.ROAs[label]
		current, exists := roas[owned.Handle]
		want, keep := desired.ROAs[label]
		if !exists {
			continue
		}
		if keep && ROAMatchesRequest(current, want.Request) {
			plan.RetainedROAs[label] = owned.Handle
			continue
		}
		// Replacement unlinks/preserves old routes; removal uses saved policy.
		plan.Transaction.DeleteROAs = append(plan.Transaction.DeleteROAs, ROADelete{Handle: owned.Handle, AutoLink: !keep && owned.DeleteLinkedRoutes})
		deleting[owned.Handle] = true
	}
	for _, label := range slices.Sorted(maps.Keys(desired.ROAs)) {
		if _, ok := plan.RetainedROAs[label]; ok {
			continue
		}
		want := desired.ROAs[label].Request
		for handle, current := range roas {
			if deleting[handle] || current.ASN != want.ASN {
				continue
			}
			for _, a := range current.Resources {
				for _, b := range want.Resources {
					if a.Prefix == b.Prefix {
						return nil, errors.New("ROA origin/prefix already exists outside the planned replacements; import its ownership first")
					}
				}
			}
		}
		plan.AddedROALabels = append(plan.AddedROALabels, label)
		plan.Transaction.AddROAs = append(plan.Transaction.AddROAs, want)
	}
	ownedASPAs := map[int64]bool{}
	for _, customer := range prior.ASPAs {
		ownedASPAs[customer] = true
	}
	for _, customer := range slices.Sorted(maps.Keys(ownedASPAs)) {
		actual, exists := aspas[customer]
		want, keep := desired.ASPAs[customer]
		if exists && (!keep || !ASPAEqual(actual, want)) {
			plan.Transaction.DeleteASPAs = append(plan.Transaction.DeleteASPAs, customer)
		}
	}
	for _, customer := range slices.Sorted(maps.Keys(desired.ASPAs)) {
		want := desired.ASPAs[customer]
		actual, exists := aspas[customer]
		if exists && !ownedASPAs[customer] {
			return nil, errors.New("ASPA customer already exists; import its ownership first")
		}
		if !exists || !ASPAEqual(actual, want) {
			plan.Transaction.AddASPAs = append(plan.Transaction.AddASPAs, want)
		}
	}
	if !plan.Empty() {
		if err := plan.Transaction.Validate(); err != nil {
			return nil, err
		}
	}
	return plan, nil
}
func (p *RPKIBundlePlan) Empty() bool {
	t := p.Transaction
	return len(t.AddROAs)+len(t.DeleteROAs)+len(t.AddASPAs)+len(t.DeleteASPAs) == 0
}

// ReconcileRPKIBundle is read-only and requires every intended postcondition.
// Newly matching objects that existed outside this bundle before the write are
// never adopted. A planned replacement may reuse its previously owned handle.
func ReconcileRPKIBundle(plan *RPKIBundlePlan, inventory RPKIBundleInventory) (*RPKIBundleState, error) {
	if plan == nil {
		return nil, errors.New("missing RPKI bundle recovery receipt")
	}
	if err := plan.Desired.Validate(); err != nil {
		return nil, err
	}
	if err := plan.Prior.validate(); err != nil {
		return nil, err
	}
	if len(plan.AddedROALabels) != len(plan.Transaction.AddROAs) {
		return nil, errors.New("invalid bundle addition receipt")
	}
	if err := validateBundleReceipt(plan); err != nil {
		return nil, err
	}
	roas, aspas, err := bundleInventory(inventory)
	if err != nil {
		return nil, err
	}
	baseline := map[string]bool{}
	for _, handle := range plan.BaselineROAHandles {
		baseline[handle] = true
	}
	deleting := map[string]bool{}
	for _, entry := range plan.Transaction.DeleteROAs {
		deleting[entry.Handle] = true
	}
	out := &RPKIBundleState{ROAs: map[string]ROA{}, ASPAs: map[int64]ASPA{}}
	used := map[string]bool{}
	for label, handle := range plan.RetainedROAs {
		want, ok := plan.Desired.ROAs[label]
		prior, owned := plan.Prior.ROAs[label]
		actual, exists := roas[handle]
		if !ok || !owned || prior.Handle != handle || !exists || !ROAMatchesRequest(actual, want.Request) || used[handle] {
			return nil, errors.New("retained ROA no longer matches the bundle")
		}
		out.ROAs[label] = actual
		used[handle] = true
	}
	for index, label := range plan.AddedROALabels {
		want, ok := plan.Desired.ROAs[label]
		if !ok {
			return nil, errors.New("unknown ROA label in recovery receipt")
		}
		request := plan.Transaction.AddROAs[index]
		if !ROAMatchesRequest(ROA{Name: request.Name, ASN: request.ASN, Resources: bundleRequestedResources(request)}, want.Request) {
			return nil, errors.New("ROA request does not match desired recovery state")
		}
		if _, ok := out.ROAs[label]; ok {
			return nil, errors.New("duplicate ROA label in recovery receipt")
		}
		found := ""
		for handle, actual := range roas {
			if (baseline[handle] && !deleting[handle]) || !ROAMatchesRequest(actual, want.Request) {
				continue
			}
			if found != "" || used[handle] {
				return nil, errors.New("ambiguous bundle ROA recovery")
			}
			found = handle
		}
		if found == "" {
			return nil, errors.New("bundle ROA addition is not yet confirmed")
		}
		out.ROAs[label] = roas[found]
		used[found] = true
	}
	if len(out.ROAs) != len(plan.Desired.ROAs) {
		return nil, errors.New("incomplete bundle ROA recovery receipt")
	}
	for _, entry := range plan.Prior.ROAs {
		if _, exists := roas[entry.Handle]; exists && !used[entry.Handle] {
			return nil, errors.New("bundle ROA deletion is not yet confirmed")
		}
	}
	for customer, want := range plan.Desired.ASPAs {
		actual, exists := aspas[customer]
		if !exists || !ASPAEqual(actual, want) {
			return nil, errors.New("bundle ASPA is not yet confirmed")
		}
		out.ASPAs[customer] = actual
	}
	for _, customer := range plan.Prior.ASPAs {
		if _, keep := plan.Desired.ASPAs[customer]; keep {
			continue
		}
		if _, exists := aspas[customer]; exists {
			return nil, errors.New("bundle ASPA deletion is not yet confirmed")
		}
	}
	return out, nil
}
func bundleRequestedResources(request ROARequest) []ROAResource {
	out := slices.Clone(request.Resources)
	for i := range out {
		out[i].AutoLinked = request.AutoLink
	}
	return out
}

func validateBundleReceipt(plan *RPKIBundlePlan) error {
	if !plan.Empty() {
		if err := plan.Transaction.Validate(); err != nil {
			return err
		}
	}
	owned := map[string]RPKIBundleOwnedROA{}
	labels := map[string]string{}
	for label, entry := range plan.Prior.ROAs {
		owned[entry.Handle] = entry
		labels[entry.Handle] = label
	}
	for _, entry := range plan.Transaction.DeleteROAs {
		prior, ok := owned[entry.Handle]
		_, keep := plan.Desired.ROAs[labels[entry.Handle]]
		if !ok || entry.AutoLink != (!keep && prior.DeleteLinkedRoutes) {
			return errors.New("bundle receipt contains an unowned deletion or changed deletion policy")
		}
		for _, handle := range plan.RetainedROAs {
			if handle == entry.Handle {
				return errors.New("bundle receipt both retains and deletes a ROA")
			}
		}
	}
	ownedCustomers := map[int64]bool{}
	for _, customer := range plan.Prior.ASPAs {
		ownedCustomers[customer] = true
	}
	addedCustomers := map[int64]bool{}
	for _, entry := range plan.Transaction.AddASPAs {
		desired, ok := plan.Desired.ASPAs[entry.CustomerASN]
		if !ok || !ASPAEqual(entry, desired) {
			return errors.New("ASPA request does not match desired recovery state")
		}
		addedCustomers[entry.CustomerASN] = true
	}
	for _, customer := range plan.Transaction.DeleteASPAs {
		if !ownedCustomers[customer] {
			return errors.New("bundle receipt deletes an unowned ASPA")
		}
		if _, keep := plan.Desired.ASPAs[customer]; keep && !addedCustomers[customer] {
			return errors.New("bundle receipt omits a replacement ASPA addition")
		}
	}
	for customer := range plan.Desired.ASPAs {
		if !ownedCustomers[customer] && !addedCustomers[customer] {
			return errors.New("bundle receipt omits a required ASPA addition")
		}
	}
	return nil
}
