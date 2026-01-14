package middlewares

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestBuildUserInfoFromClaims_CompleteUserInfo(t *testing.T) {
	claims := map[string]interface{}{
		"sub":                "1234567890",
		"email":              "user@example.com",
		"name":               "John Doe",
		"preferred_username": "johndoe",
		"given_name":         "John",
		"family_name":        "Doe",
		"email_verified":     true,
		"realm_access": map[string]interface{}{
			"roles": []interface{}{"admin", "user"},
		},
		"resource_access": map[string]interface{}{
			"cloak-apps-portal": map[string]interface{}{
				"roles": []interface{}{"portal-admin", "portal-user"},
			},
			"another-client": map[string]interface{}{
				"roles": []interface{}{"viewer"},
			},
		},
	}

	userInfo := buildUserInfoFromClaims(claims)

	assert.NotNil(t, userInfo)
	assert.Equal(t, "1234567890", userInfo.Sub)
	assert.Equal(t, "user@example.com", userInfo.Email)
	assert.Equal(t, "John Doe", userInfo.Name)
	assert.Equal(t, "johndoe", userInfo.PreferredUsername)
	assert.Equal(t, "John", userInfo.GivenName)
	assert.Equal(t, "Doe", userInfo.FamilyName)
	assert.True(t, userInfo.EmailVerified)
	assert.Equal(t, []string{"admin", "user"}, userInfo.Roles)
	assert.Len(t, userInfo.ClientRoles, 2)
	assert.Equal(t, []string{"portal-admin", "portal-user"}, userInfo.ClientRoles["cloak-apps-portal"])
	assert.Equal(t, []string{"viewer"}, userInfo.ClientRoles["another-client"])
}

func TestBuildUserInfoFromClaims_MinimalClaims(t *testing.T) {
	claims := map[string]interface{}{
		"sub":                "1234567890",
		"preferred_username": "johndoe",
	}

	userInfo := buildUserInfoFromClaims(claims)

	assert.NotNil(t, userInfo)
	assert.Equal(t, "1234567890", userInfo.Sub)
	assert.Equal(t, "johndoe", userInfo.PreferredUsername)
	assert.Equal(t, "", userInfo.Email)
	assert.Equal(t, "", userInfo.Name)
	assert.Equal(t, "", userInfo.GivenName)
	assert.Equal(t, "", userInfo.FamilyName)
	assert.False(t, userInfo.EmailVerified)
	assert.Empty(t, userInfo.Roles)
	assert.NotNil(t, userInfo.ClientRoles)
	assert.Empty(t, userInfo.ClientRoles)
}

func TestBuildUserInfoFromClaims_EmptyClaims(t *testing.T) {
	claims := map[string]interface{}{}

	userInfo := buildUserInfoFromClaims(claims)

	assert.NotNil(t, userInfo)
	assert.Equal(t, "", userInfo.Sub)
	assert.Equal(t, "", userInfo.Email)
	assert.Equal(t, "", userInfo.PreferredUsername)
	assert.NotNil(t, userInfo.ClientRoles)
	assert.Empty(t, userInfo.ClientRoles)
}

func TestBuildUserInfoFromClaims_EmptyRealmRoles(t *testing.T) {
	claims := map[string]interface{}{
		"sub": "1234567890",
		"realm_access": map[string]interface{}{
			"roles": []interface{}{},
		},
	}

	userInfo := buildUserInfoFromClaims(claims)

	assert.NotNil(t, userInfo)
	assert.Equal(t, "1234567890", userInfo.Sub)
	assert.Empty(t, userInfo.Roles)
}

func TestBuildUserInfoFromClaims_MissingRealmAccess(t *testing.T) {
	claims := map[string]interface{}{
		"sub":   "1234567890",
		"email": "user@example.com",
	}

	userInfo := buildUserInfoFromClaims(claims)

	assert.NotNil(t, userInfo)
	assert.Equal(t, "1234567890", userInfo.Sub)
	assert.Equal(t, "user@example.com", userInfo.Email)
	assert.Empty(t, userInfo.Roles)
}

func TestBuildUserInfoFromClaims_MissingResourceAccess(t *testing.T) {
	claims := map[string]interface{}{
		"sub":   "1234567890",
		"email": "user@example.com",
		"realm_access": map[string]interface{}{
			"roles": []interface{}{"admin"},
		},
	}

	userInfo := buildUserInfoFromClaims(claims)

	assert.NotNil(t, userInfo)
	assert.Equal(t, "1234567890", userInfo.Sub)
	assert.Equal(t, []string{"admin"}, userInfo.Roles)
	assert.NotNil(t, userInfo.ClientRoles)
	assert.Empty(t, userInfo.ClientRoles)
}

func TestBuildUserInfoFromClaims_SingleClientRole(t *testing.T) {
	claims := map[string]interface{}{
		"sub": "1234567890",
		"resource_access": map[string]interface{}{
			"my-app": map[string]interface{}{
				"roles": []interface{}{"developer"},
			},
		},
	}

	userInfo := buildUserInfoFromClaims(claims)

	assert.NotNil(t, userInfo)
	assert.Len(t, userInfo.ClientRoles, 1)
	assert.Equal(t, []string{"developer"}, userInfo.ClientRoles["my-app"])
}

func TestBuildUserInfoFromClaims_EmailVerifiedFalse(t *testing.T) {
	claims := map[string]interface{}{
		"sub":            "1234567890",
		"email":          "user@example.com",
		"email_verified": false,
	}

	userInfo := buildUserInfoFromClaims(claims)

	assert.NotNil(t, userInfo)
	assert.Equal(t, "user@example.com", userInfo.Email)
	assert.False(t, userInfo.EmailVerified)
}

func TestBuildUserInfoFromClaims_TypeCoercion(t *testing.T) {
	// mapstructure with WeaklyTypedInput should handle different types
	claims := map[string]interface{}{
		"sub":            "1234567890",
		"email_verified": "true", // String instead of bool
	}

	userInfo := buildUserInfoFromClaims(claims)

	assert.NotNil(t, userInfo)
	assert.Equal(t, "1234567890", userInfo.Sub)
	// WeaklyTypedInput should convert "true" string to true bool
	assert.True(t, userInfo.EmailVerified)
}

func TestBuildUserInfoFromClaims_NilRealmAccessRoles(t *testing.T) {
	claims := map[string]interface{}{
		"sub": "1234567890",
		"realm_access": map[string]interface{}{
			"roles": nil,
		},
	}

	userInfo := buildUserInfoFromClaims(claims)

	assert.NotNil(t, userInfo)
	assert.Equal(t, "1234567890", userInfo.Sub)
	assert.Empty(t, userInfo.Roles)
}

func TestBuildUserInfoFromClaims_EmptyResourceAccess(t *testing.T) {
	claims := map[string]interface{}{
		"sub":             "1234567890",
		"resource_access": map[string]interface{}{},
	}

	userInfo := buildUserInfoFromClaims(claims)

	assert.NotNil(t, userInfo)
	assert.NotNil(t, userInfo.ClientRoles)
	assert.Empty(t, userInfo.ClientRoles)
}

func TestBuildUserInfoFromClaims_MultipleRealmsAndClients(t *testing.T) {
	claims := map[string]interface{}{
		"sub":                "1234567890",
		"preferred_username": "johndoe",
		"realm_access": map[string]interface{}{
			"roles": []interface{}{"offline_access", "uma_authorization", "default-roles-realm"},
		},
		"resource_access": map[string]interface{}{
			"account": map[string]interface{}{
				"roles": []interface{}{"manage-account", "view-profile"},
			},
			"cloak-apps-portal": map[string]interface{}{
				"roles": []interface{}{"portal-admin"},
			},
			"broker": map[string]interface{}{
				"roles": []interface{}{"read-token"},
			},
		},
	}

	userInfo := buildUserInfoFromClaims(claims)

	assert.NotNil(t, userInfo)
	assert.Equal(t, "1234567890", userInfo.Sub)
	assert.Len(t, userInfo.Roles, 3)
	assert.Contains(t, userInfo.Roles, "offline_access")
	assert.Contains(t, userInfo.Roles, "uma_authorization")
	assert.Contains(t, userInfo.Roles, "default-roles-realm")

	assert.Len(t, userInfo.ClientRoles, 3)
	assert.Equal(t, []string{"manage-account", "view-profile"}, userInfo.ClientRoles["account"])
	assert.Equal(t, []string{"portal-admin"}, userInfo.ClientRoles["cloak-apps-portal"])
	assert.Equal(t, []string{"read-token"}, userInfo.ClientRoles["broker"])
}

func TestBuildUserInfoFromClaims_SpecialCharactersInFields(t *testing.T) {
	claims := map[string]interface{}{
		"sub":                "user-123-abc",
		"email":              "user+test@example.com",
		"name":               "John O'Doe",
		"preferred_username": "john.doe@example",
	}

	userInfo := buildUserInfoFromClaims(claims)

	assert.NotNil(t, userInfo)
	assert.Equal(t, "user-123-abc", userInfo.Sub)
	assert.Equal(t, "user+test@example.com", userInfo.Email)
	assert.Equal(t, "John O'Doe", userInfo.Name)
	assert.Equal(t, "john.doe@example", userInfo.PreferredUsername)
}

func TestBuildUserInfoFromClaims_UnicodeCharacters(t *testing.T) {
	claims := map[string]interface{}{
		"sub":         "1234567890",
		"given_name":  "José",
		"family_name": "Müller",
		"name":        "José Müller",
	}

	userInfo := buildUserInfoFromClaims(claims)

	assert.NotNil(t, userInfo)
	assert.Equal(t, "José", userInfo.GivenName)
	assert.Equal(t, "Müller", userInfo.FamilyName)
	assert.Equal(t, "José Müller", userInfo.Name)
}
