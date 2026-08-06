package models

import "time"

// OfficeLocation is a registered physical office employees must be within
// radius_meters of to check in/out — geofence math happens at write time
// against every active location, not via a stored reference.
type OfficeLocation struct {
	ID           string    `json:"id"`
	ShortID      string    `json:"short_id"`
	Name         string    `json:"name"`
	Address      string    `json:"address,omitempty"`
	Latitude     float64   `json:"latitude"`
	Longitude    float64   `json:"longitude"`
	RadiusMeters int       `json:"radius_meters"`
	IsActive     bool      `json:"is_active"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}

type CreateOfficeLocationInput struct {
	Name         string  `json:"name"          binding:"required" example:"HQ"`
	Address      string  `json:"address" example:"123 Main St"`
	Latitude     float64 `json:"latitude"       binding:"min=-90,max=90"`
	Longitude    float64 `json:"longitude"      binding:"min=-180,max=180"`
	RadiusMeters int     `json:"radius_meters"  binding:"required,min=10" example:"200"`
}

type UpdateOfficeLocationInput struct {
	Name         *string  `json:"name"`
	Address      *string  `json:"address"`
	Latitude     *float64 `json:"latitude"  binding:"omitempty,min=-90,max=90"`
	Longitude    *float64 `json:"longitude" binding:"omitempty,min=-180,max=180"`
	RadiusMeters *int     `json:"radius_meters" binding:"omitempty,min=10"`
	IsActive     *bool    `json:"is_active"`
}

// ResolveMapsLinkInput carries a pasted Google Maps URL (short or full) to
// resolve to coordinates.
type ResolveMapsLinkInput struct {
	URL string `json:"url" binding:"required" example:"https://maps.app.goo.gl/vCPqUuyE1VdT2CSx6"`
}

// ResolvedMapsLocation is the lat/lng (and, when derivable, a place name)
// extracted from a resolved Google Maps link.
type ResolvedMapsLocation struct {
	Latitude  float64 `json:"latitude"`
	Longitude float64 `json:"longitude"`
	Name      string  `json:"name,omitempty"`
}
