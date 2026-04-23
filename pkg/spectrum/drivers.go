package spectrum

import (
	"bytes"
	"fmt"
)

// Driver parses one concrete spectrum format into the shared model.
type Driver interface {
	Name() string
	CanParse(raw []byte) bool
	Parse(raw []byte) (SpectrumMeasurement, error)
}

// ParseWithKnownDrivers tries built-in drivers in order.
func ParseWithKnownDrivers(raw []byte) (SpectrumMeasurement, error) {
	for _, driver := range builtInDrivers() {
		if !driver.CanParse(raw) {
			continue
		}
		measurement, err := driver.Parse(raw)
		if err != nil {
			return SpectrumMeasurement{}, fmt.Errorf("parse with %s: %w", driver.Name(), err)
		}
		return measurement, nil
	}
	return SpectrumMeasurement{}, fmt.Errorf("unsupported spectrum format")
}

func builtInDrivers() []Driver {
	return []Driver{RadiacodeXMLDriver{}}
}

func byteContainsFold(haystack []byte, needle string) bool {
	return bytes.Contains(bytes.ToLower(haystack), bytes.ToLower([]byte(needle)))
}
