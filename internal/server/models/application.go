package models

// Application represents an application available in the portal
type Application struct {
	ID           string   `json:"id"`
	ClientID     string   `json:"clientId"`
	Name         string   `json:"name"`                // From Name field
	Description  string   `json:"description"`         // From Description JSON "text" field
	Tooltip      string   `json:"tooltip,omitempty"`   // From Description JSON
	IconEmoji    string   `json:"iconEmoji,omitempty"` // From Description JSON
	ThumbnailURL string   `json:"thumbnailUrl"`        // From Attributes["logoUrl"]
	URL          string   `json:"url"`                 // From BaseURL (Home URL)
	Space        string   `json:"space"`               // From client scopes "space-*"
	Environment  string   `json:"environment"`         // From client scopes "env-*"
	Tags         []string `json:"tags,omitempty"`      // From Description JSON
	Order        int      `json:"order"`               // From Description JSON
	SSOEnabled   bool     `json:"ssoEnabled"`          // From Description JSON
	HasAccess    bool     `json:"hasAccess"`           // Computed based on user roles
}
