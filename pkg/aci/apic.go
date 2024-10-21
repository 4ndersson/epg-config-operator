package aci

import (
	"fmt"
	"strings"

	aciclient "github.com/ciscoecosystem/aci-go-client/client"
	"github.com/ciscoecosystem/aci-go-client/models"
)

type ApicClient struct {
	host     string
	user     string
	password string
	client   *aciclient.Client
}

type ApicInterface interface {
	CreateEpg(name, app, tenant, bd, vmm, vmmType string) error
	DeleteEpg(name, app, tenant string) error
	EpgExists(name, app, tenant string) (bool, error)
	ConsumeContract(epgName, app, tenant, conName string) error
	ProvideContract(epgName, app, tenant, conName string) error
	GetConsumedContracts(epgName, app, tenant string) ([]string, error)
	GetProvidedContracts(epgName, app, tenant string) ([]string, error)
}

func NewClient(host, user, password, key string) (*ApicClient, error) {
	ac := &ApicClient{
		host:     host,
		user:     user,
		password: password,
	}
	if key == "" {
		ac.client = aciclient.GetClient(fmt.Sprintf("https://%s/", host), user, aciclient.Password(password), aciclient.Insecure(true), aciclient.SkipLoggingPayload(true))
	} else {
		ac.client = aciclient.GetClient(fmt.Sprintf("https://%s/", host), user, aciclient.PrivateKey(key), aciclient.AdminCert(fmt.Sprintf("%s.crt", user)), aciclient.Insecure(true), aciclient.SkipLoggingPayload(true))
	}

	_, err := ac.client.ListSystem()

	return ac, err
}

func (ac *ApicClient) CreateEpg(name, app, tenant, bd, vmm, vmmType string) error {
	fvAEpgAttr := models.ApplicationEPGAttributes{}
	fvAEpgAttr.Annotation = fmt.Sprintf("orchestrator:%s", strings.ToLower(vmmType))
	fvAEpg := models.NewApplicationEPG(fmt.Sprintf("epg-%s", name), fmt.Sprintf("uni/tn-%s/ap-%s", tenant, app), "created by kubernetes operator", fvAEpgAttr)

	err := ac.client.Save(fvAEpg)
	if err != nil {
		return err
	}

	err = ac.client.CreateRelationfvRsBdFromApplicationEPG(fvAEpg.DistinguishedName, bd)
	if err != nil {
		return err
	}

	// Does not return error if not bound correctly
	err = ac.client.CreateRelationfvRsDomAttFromApplicationEPG(fvAEpg.DistinguishedName, fmt.Sprintf("uni/vmmp-%s/dom-%s", vmmType, vmm))
	if err != nil {
		return err
	}

	_, err = ac.client.ReadRelationfvRsDomAttFromApplicationEPG(fmt.Sprintf("uni/tn-%s/ap-%s/epg-%s", tenant, app, name))
	if err != nil {
		return err
	}
	return nil
}

func (ac *ApicClient) DeleteEpg(name, app, tenant string) error {
	err := ac.client.DeleteApplicationEPG(name, app, tenant)
	if err != nil {
		return err
	}
	return nil
}

func (ac *ApicClient) EpgExists(name, app, tenant string) (bool, error) {

	fvAEPgCont, err := ac.client.Get(fmt.Sprintf("uni/tn-%s/ap-%s/epg-%s", tenant, app, name))
	if err != nil {
		return false, err
	}
	fvAEPg := models.ApplicationEPGFromContainer(fvAEPgCont)

	if fvAEPg.DistinguishedName == "" {
		return false, nil
	}
	return true, nil
}

func (ac *ApicClient) ConsumeContract(epg, app, tenant, contract string) error {

	fvRsConsAtt := models.ContractConsumerAttributes{}
	fvRsConsAtt.TnVzBrCPName = contract
	fvRsCons := models.NewContractConsumer(fmt.Sprintf("rscons-%s", contract), fmt.Sprintf("uni/tn-%s/ap-%s/epg-%s", tenant, app, epg), fvRsConsAtt)

	err := ac.client.Save(fvRsCons)
	if err != nil {
		return err
	}
	return nil
}

func (ac *ApicClient) ProvideContract(epg, app, tenant, contract string) error {

	fvRsProvAtt := models.ContractProviderAttributes{}
	fvRsProvAtt.TnVzBrCPName = contract
	fvRsCons := models.NewContractProvider(fmt.Sprintf("rsprov-%s", contract), fmt.Sprintf("uni/tn-%s/ap-%s/epg-%s", tenant, app, epg), fvRsProvAtt)

	err := ac.client.Save(fvRsCons)
	if err != nil {
		return err
	}
	return nil
}

func (ac *ApicClient) GetConsumedContracts(epg, app, tenant string) ([]string, error) {
	baseurlStr := "/api/node/class"
	cont, err := ac.client.GetViaURL(fmt.Sprintf("%s/uni/tn-%s/ap-%s/epg-%s/fvRsCons.json", baseurlStr, tenant, app, epg))

	if err != nil {
		if strings.Contains(err.Error(), "may not exists") {
			return []string{}, nil
		} else {
			return []string{}, err
		}
	}

	contracts := models.ContractConsumerListFromContainer(cont)
	contractsParsed := make([]string, len(models.ContractConsumerListFromContainer(cont)))

	for i, contract := range contracts {
		contractsParsed[i] = contract.TnVzBrCPName
	}
	return contractsParsed, nil
}

func (ac *ApicClient) GetProvidedContracts(epg, app, tenant string) ([]string, error) {
	baseurlStr := "/api/node/class"
	cont, err := ac.client.GetViaURL(fmt.Sprintf("%s/uni/tn-%s/ap-%s/epg-%s/fvRsProv.json", baseurlStr, tenant, app, epg))

	if err != nil {
		if strings.Contains(err.Error(), "may not exists") {
			return []string{}, nil
		} else {
			return []string{}, err
		}
	}

	contracts := models.ContractProviderListFromContainer(cont)
	contractsParsed := make([]string, len(contracts))

	for i, contract := range contracts {
		contractsParsed[i] = contract.TnVzBrCPName
	}
	return contractsParsed, nil
}
