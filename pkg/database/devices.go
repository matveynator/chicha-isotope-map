package database

import (
	"context"
	"fmt"
	"strings"
)

// Device represents a unique measurement instrument shared across tracks.
// We keep it minimal so imports can store hardware metadata without tying
// the schema to a specific vendor payload.
type Device struct {
	ID    string
	Model string
}

// EnsureDevice inserts a device record if it does not exist yet, keeping
// the lookup deterministic across database engines.
func (db *Database) EnsureDevice(ctx context.Context, device Device, dbType string) error {
	if db == nil || db.DB == nil {
		return fmt.Errorf("database unavailable")
	}
	device.ID = strings.TrimSpace(device.ID)
	device.Model = strings.TrimSpace(device.Model)
	if device.ID == "" {
		return fmt.Errorf("device id required")
	}
	if ctx == nil {
		ctx = context.Background()
	}

	ph := placeholder(dbType, 1)
	ph2 := placeholder(dbType, 2)
	ph3 := placeholder(dbType, 3)

	stmt := fmt.Sprintf(`INSERT INTO devices (device_id, model)
SELECT %s, %s
WHERE NOT EXISTS (SELECT 1 FROM devices WHERE device_id = %s);`, ph, ph2, ph3)
	if _, err := db.DB.ExecContext(ctx, stmt, device.ID, device.Model, device.ID); err != nil {
		return fmt.Errorf("insert device: %w", err)
	}

	if device.Model != "" {
		update := fmt.Sprintf("UPDATE devices SET model = %s WHERE device_id = %s AND (model IS NULL OR model = '')", ph2, ph)
		if _, err := db.DB.ExecContext(ctx, update, device.ID, device.Model); err != nil {
			return fmt.Errorf("update device model: %w", err)
		}
	}
	return nil
}

// EnsureTrackDeviceMapping records the relationship between a track and a device.
// We rely on a NOT EXISTS guard to keep inserts idempotent without DB-specific upserts.
func (db *Database) EnsureTrackDeviceMapping(ctx context.Context, trackID, deviceID, dbType string) error {
	if db == nil || db.DB == nil {
		return fmt.Errorf("database unavailable")
	}
	trackID = strings.TrimSpace(trackID)
	deviceID = strings.TrimSpace(deviceID)
	if trackID == "" || deviceID == "" {
		return nil
	}
	if ctx == nil {
		ctx = context.Background()
	}

	ph := placeholder(dbType, 1)
	ph2 := placeholder(dbType, 2)
	ph3 := placeholder(dbType, 3)
	ph4 := placeholder(dbType, 4)

	stmt := fmt.Sprintf(`INSERT INTO track_devices (trackID, device_id)
SELECT %s, %s
WHERE NOT EXISTS (SELECT 1 FROM track_devices WHERE trackID = %s AND device_id = %s);`, ph, ph2, ph3, ph4)
	if _, err := db.DB.ExecContext(ctx, stmt, trackID, deviceID, trackID, deviceID); err != nil {
		return fmt.Errorf("insert track device: %w", err)
	}
	return nil
}
