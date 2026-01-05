package devices

import "strings"

// Device keeps display metadata for a supported detector so the UI can link
// markers to consistent descriptions without hitting the database.
type Device struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Summary     string `json:"summary"`
	Description string `json:"description"`
	SmallImage  string `json:"smallImage"`
	LargeImage  string `json:"largeImage"`
	PurchaseURL string `json:"purchaseURL,omitempty"`
}

// Known device IDs stay stable so marker metadata keeps a short, portable form.
const (
	DeviceAtomFast  = "atomfast"
	DeviceRadiacode = "radiacode"
	DeviceSafecast  = "safecast"
	DeviceBGeigie   = "bgeigie"
	DeviceGeneric   = "generic"
	DeviceUnknown   = "unknown"
)

// catalog holds the canonical metadata so callers never mutate shared state.
var catalog = []Device{
	{
		ID:          DeviceAtomFast,
		Name:        "Atom Fast",
		Summary:     "Compact dosimeter uploads shared measurements to AtomFast maps.",
		Description: "Atom Fast devices capture radiation readings while you travel, synchronising markers into the AtomFast community map for crowdsourced cartography.",
		SmallImage:  "/static/images/devices/atomfast-small.svg",
		LargeImage:  "/static/images/devices/atomfast.svg",
	},
	{
		ID:          DeviceRadiacode,
		Name:        "Radiacode",
		Summary:     "Spectrometer-grade mobile dosimeter with detailed gamma data.",
		Description: "Radiacode devices combine spectroscopy with GPS logging, giving rich dose-rate tracks for environmental surveys and safety work.",
		SmallImage:  "/static/images/devices/radiacode-small.svg",
		LargeImage:  "/static/images/devices/radiacode.svg",
	},
	{
		ID:          DeviceSafecast,
		Name:        "Safecast",
		Summary:     "Open hardware network of radiation sensors for realtime maps.",
		Description: "Safecast sensors broadcast realtime dose-rate measurements, helping communities monitor background radiation and share data openly.",
		SmallImage:  "/static/images/devices/safecast-small.svg",
		LargeImage:  "/static/images/devices/safecast.svg",
	},
	{
		ID:          DeviceBGeigie,
		Name:        "bGeigie",
		Summary:     "Drive-by mapper from the Safecast open sensor family.",
		Description: "bGeigie loggers are designed for wide-area surveys, recording geotagged dose rates during vehicle or walking surveys.",
		SmallImage:  "/static/images/devices/bgeigie-small.svg",
		LargeImage:  "/static/images/devices/bgeigie.svg",
	},
	{
		ID:          DeviceGeneric,
		Name:        "Generic GPS logger",
		Summary:     "Generic GPS or CSV uploads without device telemetry.",
		Description: "Tracks uploaded via GPX, KML, or CSV formats without detector metadata are tagged as generic devices for transparency.",
		SmallImage:  "/static/images/devices/generic-small.svg",
		LargeImage:  "/static/images/devices/generic.svg",
	},
	{
		ID:          DeviceUnknown,
		Name:        "Unknown device",
		Summary:     "Device metadata was not supplied in the track.",
		Description: "When a track does not contain detector information we tag it as unknown, keeping the data transparent while awaiting details.",
		SmallImage:  "/static/images/devices/unknown-small.svg",
		LargeImage:  "/static/images/devices/unknown.svg",
	},
}

// catalogIndex maps IDs to devices for quick lookups without extra allocations.
var catalogIndex = func() map[string]Device {
	index := make(map[string]Device, len(catalog))
	for _, device := range catalog {
		index[device.ID] = device
	}
	return index
}()

// Catalog returns a copy so callers can reorder without mutating the shared slice.
func Catalog() []Device {
	out := make([]Device, len(catalog))
	copy(out, catalog)
	return out
}

// ByID looks up a device by its canonical ID.
func ByID(id string) (Device, bool) {
	deviceID := NormalizeDeviceID(id)
	if deviceID == "" {
		return Device{}, false
	}
	device, ok := catalogIndex[deviceID]
	return device, ok
}

// ResolveDeviceID maps known aliases to stable IDs so UI and exports stay aligned.
func ResolveDeviceID(raw string) (string, bool) {
	trimmed := strings.ToLower(strings.TrimSpace(raw))
	if trimmed == "" {
		return "", false
	}
	switch {
	case trimmed == DeviceAtomFast || strings.Contains(trimmed, "atomfast"):
		return DeviceAtomFast, true
	case trimmed == DeviceRadiacode || strings.Contains(trimmed, "radiacode"):
		return DeviceRadiacode, true
	case trimmed == DeviceSafecast || strings.Contains(trimmed, "safecast"):
		return DeviceSafecast, true
	case trimmed == DeviceBGeigie || strings.Contains(trimmed, "bgeigie"):
		return DeviceBGeigie, true
	case trimmed == DeviceGeneric || trimmed == "gpx" || trimmed == "kml" || trimmed == "kmz":
		return DeviceGeneric, true
	case trimmed == DeviceUnknown:
		return DeviceUnknown, true
	default:
		return "", false
	}
}

// NormalizeDeviceID keeps known devices consistent while preserving unknown labels.
func NormalizeDeviceID(raw string) string {
	if resolved, ok := ResolveDeviceID(raw); ok {
		return resolved
	}
	return strings.TrimSpace(raw)
}
