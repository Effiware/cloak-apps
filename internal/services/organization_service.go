package services

import "github.com/effiware/cloak-apps/internal/server/models"

type OrganizationService models.Organization

func NewOrganizationService(params ...string) (*OrganizationService, error) {
	os := &OrganizationService{}

	switch len(params) {
	case 3:
		os.CustomDescription = params[2]
		fallthrough
	case 2:
		os.HomeUrl = params[1]
		fallthrough
	case 1:
		os.Name = params[0]
	default:
	}

	if err := os.validateDescriptionHtml(); err != nil {
		return nil, err
	}
	return os, nil
}

func (os *OrganizationService) validateDescriptionHtml() error {
	// TODO: add method to see if the description HTML is not corrupted / insecure
	return nil
}

func (os *OrganizationService) GetOrganization() (models.Organization, error) {
	return models.Organization(*os), nil
}
