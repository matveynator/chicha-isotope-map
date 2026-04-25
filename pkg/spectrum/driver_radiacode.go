package spectrum

import (
	"encoding/xml"
	"fmt"
	"strings"
	"time"
)

// RadiacodeXMLDriver parses ResultDataFile XML exports.
type RadiacodeXMLDriver struct{}

func (RadiacodeXMLDriver) Name() string { return "radiacode-xml" }

func (RadiacodeXMLDriver) CanParse(raw []byte) bool {
	return byteContainsFold(raw, "<ResultDataFile") && byteContainsFold(raw, "<EnergySpectrum")
}

func (RadiacodeXMLDriver) Parse(raw []byte) (SpectrumMeasurement, error) {
	var document radiacodeResultDataFile
	if err := xml.Unmarshal(raw, &document); err != nil {
		return SpectrumMeasurement{}, fmt.Errorf("xml unmarshal: %w", err)
	}
	if len(document.ResultDataList.Items) == 0 {
		return SpectrumMeasurement{}, fmt.Errorf("empty ResultData list")
	}

	entry := document.ResultDataList.Items[0]
	if len(entry.EnergySpectrum.Spectrum.Data) == 0 {
		return SpectrumMeasurement{}, fmt.Errorf("empty spectrum data")
	}
	if len(entry.EnergySpectrum.EnergyCalibration.Coefficients.Values) < 2 {
		return SpectrumMeasurement{}, fmt.Errorf("insufficient calibration coefficients")
	}

	startTime, err := parseXMLTime(entry.StartTime)
	if err != nil {
		return SpectrumMeasurement{}, err
	}
	endTime, err := parseXMLTime(entry.EndTime)
	if err != nil {
		return SpectrumMeasurement{}, err
	}

	return SpectrumMeasurement{
		Format:         "radiacode-xml",
		DeviceName:     strings.TrimSpace(entry.DeviceConfigReference.Name),
		DeviceSerial:   strings.TrimSpace(entry.EnergySpectrum.SerialNumber),
		StartTime:      startTime,
		EndTime:        endTime,
		MeasurementSec: entry.EnergySpectrum.MeasurementTime,
		Channels:       append([]float64(nil), entry.EnergySpectrum.Spectrum.Data...),
		Coefficients:   append([]float64(nil), entry.EnergySpectrum.EnergyCalibration.Coefficients.Values...),
	}, nil
}

type radiacodeResultDataFile struct {
	XMLName        xml.Name            `xml:"ResultDataFile"`
	ResultDataList radiacodeResultList `xml:"ResultDataList"`
}

type radiacodeResultList struct {
	Items []radiacodeResultItem `xml:"ResultData"`
}

type radiacodeResultItem struct {
	DeviceConfigReference struct {
		Name string `xml:"Name"`
	} `xml:"DeviceConfigReference"`
	StartTime      string `xml:"StartTime"`
	EndTime        string `xml:"EndTime"`
	EnergySpectrum struct {
		SerialNumber      string `xml:"SerialNumber"`
		MeasurementTime   int64  `xml:"MeasurementTime"`
		EnergyCalibration struct {
			Coefficients struct {
				Values []float64 `xml:"Coefficient"`
			} `xml:"Coefficients"`
		} `xml:"EnergyCalibration"`
		Spectrum struct {
			Data []float64 `xml:"DataPoint"`
		} `xml:"Spectrum"`
	} `xml:"EnergySpectrum"`
}

func parseXMLTime(raw string) (time.Time, error) {
	trimmed := strings.TrimSpace(raw)
	layouts := []string{time.RFC3339, "2006-01-02T15:04:05"}
	for _, layout := range layouts {
		parsedTime, err := time.Parse(layout, trimmed)
		if err == nil {
			return parsedTime.UTC(), nil
		}
	}
	return time.Time{}, fmt.Errorf("parse time %q", raw)
}
