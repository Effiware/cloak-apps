package models

type Organization struct {
	Name              string `json:"name"`
	HomeUrl           string `json:"home_url"`
	CustomDescription string `json:"custom_description"`
}
