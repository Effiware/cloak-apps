package services

import (
	"fmt"
	"net/url"

	"github.com/effiware/cloak-apps/internal/server/models"
	"github.com/microcosm-cc/bluemonday"
)

type OrganizationService models.Organization

type OsOptions struct {
	Name        string
	HomeUrl     string
	Description string
}

func NewOrganizationService(options OsOptions) (*OrganizationService, error) {
	os := &OrganizationService{}

	canonicalized, err := os.canonicalizeHtml(options.Description)
	if err != nil {
		return nil, err
	}

	os.Name = options.Name
	os.HomeUrl = options.HomeUrl
	os.CustomDescription = os.saniziteHtml(canonicalized)

	return os, nil
}

func (os *OrganizationService) GetOrganization() (models.Organization, error) {
	return models.Organization(*os), nil
}

// saniziteHtml ensures that the text is plain and free from any HTML or script tags
func (os *OrganizationService) saniziteHtml(in string) string {
	policy := bluemonday.StrictPolicy()
	return policy.Sanitize(in)
}

// canonicalizeHtml decodes HTML repeatedly until no encoding tokens remain
func (os *OrganizationService) canonicalizeHtml(in string) (string, error) {
	var err error
	var prev string
	var count int

	for in != prev {
		prev = in

		if count > 10 {
			return "", fmt.Errorf("could't canonicalize description - too many escape layers")
		}
		if in, err = url.QueryUnescape(in); err != nil {
			return "", err
		}
		count++
	}

	return in, nil
}
