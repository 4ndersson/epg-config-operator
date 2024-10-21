package aci

import (
	"fmt"

	"github.com/4ndersson/epg-config-operator/pkg/utils"
)

type endpointGroup struct {
	name      string
	tenant    string
	app       string
	Bd        string
	Vmm       string
	VmmType   string
	contracts map[string][]string
}

type ApicClientMocks struct {
	endpointGroups map[string]endpointGroup
}

func init() {
	ApicMockClient.endpointGroups = map[string]endpointGroup{}
}

var (
	ApicMockClient ApicClientMocks
)

func (ac *ApicClientMocks) CreateEpg(name, app, tenant, bd, vmm, vmmType string) error {
	dn := fmt.Sprintf("uni/tn-%s/ap-%s/epg-%s", tenant, app, name)
	fmt.Printf("Creating EPG %s \n", dn)
	ac.endpointGroups[dn] = endpointGroup{name: name, app: app, tenant: tenant, Bd: bd, VmmType: vmmType, Vmm: vmm, contracts: map[string][]string{}}
	return nil
}

func (ac *ApicClientMocks) DeleteEpg(name, app, tenant string) error {
	dn := fmt.Sprintf("uni/tn-%s/ap-%s/epg-%s", tenant, app, name)
	fmt.Printf("Deleting EPG %s \n", dn)
	delete(ac.endpointGroups, dn)
	return nil
}

func (ac *ApicClientMocks) EpgExists(name, app, tenant string) (bool, error) {
	dn := fmt.Sprintf("uni/tn-%s/ap-%s/epg-%s", tenant, app, name)
	fmt.Printf("Checking if EPG %s exists\n", dn)
	_, exists := ac.endpointGroups[dn]
	fmt.Println(ac.endpointGroups[dn])
	return exists, nil
}

func (ac *ApicClientMocks) ConsumeContract(epg, app, tenant, contract string) error {
	dn := fmt.Sprintf("uni/tn-%s/ap-%s/epg-%s", tenant, app, epg)
	fmt.Printf("EPG %s consuming contract %s\n", dn, contract)
	if !utils.Contains(ac.endpointGroups[dn].contracts["consumed"], contract) {
		ac.endpointGroups[dn].contracts["consumed"] = append(ac.endpointGroups[dn].contracts["consumed"], contract)
	}
	return nil
}

func (ac *ApicClientMocks) ProvideContract(epg, app, tenant, contract string) error {
	dn := fmt.Sprintf("uni/tn-%s/ap-%s/epg-%s", tenant, app, epg)
	fmt.Printf("EPG %s providing contract %s\n", dn, contract)
	if !utils.Contains(ac.endpointGroups[dn].contracts["provided"], contract) {
		ac.endpointGroups[dn].contracts["provided"] = append(ac.endpointGroups[dn].contracts["provided"], contract)
	}
	return nil
}

func (ac *ApicClientMocks) GetEpg(name, app, tenant string) endpointGroup {
	dn := fmt.Sprintf("uni/tn-%s/ap-%s/epg-%s", tenant, app, name)
	fmt.Printf("Getting EPG %s \n", dn)
	return ac.endpointGroups[dn]
}

func (ac *ApicClientMocks) GetConsumedContracts(name, app, tenant string) ([]string, error) {
	dn := fmt.Sprintf("uni/tn-%s/ap-%s/epg-%s", tenant, app, name)
	fmt.Printf("Getting EPG %s \n", dn)
	return ac.endpointGroups[dn].contracts["consumed"], nil
}

func (ac *ApicClientMocks) GetProvidedContracts(name, app, tenant string) ([]string, error) {
	dn := fmt.Sprintf("uni/tn-%s/ap-%s/epg-%s", tenant, app, name)
	fmt.Printf("Getting EPG %s \n", dn)
	return ac.endpointGroups[dn].contracts["provided"], nil
}
