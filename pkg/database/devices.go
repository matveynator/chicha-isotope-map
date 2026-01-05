package database

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
)

// ---------- Device persistence helpers ----------

// UpsertDevice stores device metadata without relying on database-specific UPSERT syntax.
// We read first and then insert/update so the logic remains portable across engines.
func (db *Database) UpsertDevice(ctx context.Context, device Device, dbType string) error {
	device.ID = strings.TrimSpace(device.ID)
	device.Model = strings.TrimSpace(device.Model)
	if device.ID == "" && device.Model == "" {
		return nil
	}
	if device.ID == "" {
		device.ID = device.Model
	}

	if ctx == nil {
		ctx = context.Background()
	}

	return db.withSerializedConnectionFor(ctx, WorkloadArchive, func(ctx context.Context, conn *sql.DB) error {
		placeholder := newPlaceholderGenerator(dbType)
		idPlaceholder := placeholder()
		query := fmt.Sprintf("SELECT device_id, model FROM devices WHERE device_id = %s", idPlaceholder)
		var existingID string
		var existingModel sql.NullString
		err := conn.QueryRowContext(ctx, query, device.ID).Scan(&existingID, &existingModel)
		if err != nil && err != sql.ErrNoRows {
			return fmt.Errorf("read device: %w", err)
		}

		if err == sql.ErrNoRows {
			insert := fmt.Sprintf("INSERT INTO devices (device_id, model) VALUES (%s, %s)", placeholder(), placeholder())
			if _, err := conn.ExecContext(ctx, insert, device.ID, nullableDeviceModel(device)); err != nil {
				return fmt.Errorf("insert device: %w", err)
			}
			return nil
		}

		if device.Model == "" || (existingModel.Valid && existingModel.String == device.Model) {
			return nil
		}
		update := fmt.Sprintf("UPDATE devices SET model = %s WHERE device_id = %s", placeholder(), placeholder())
		if _, err := conn.ExecContext(ctx, update, device.Model, device.ID); err != nil {
			return fmt.Errorf("update device: %w", err)
		}
		return nil
	})
}

// AssignTrackDevice links a track to its device record.
// We avoid mutexes by keeping the check/insert/update inside a serialized lane.
func (db *Database) AssignTrackDevice(ctx context.Context, trackID string, device Device, dbType string) error {
	trackID = strings.TrimSpace(trackID)
	if trackID == "" {
		return nil
	}
	if err := db.UpsertDevice(ctx, device, dbType); err != nil {
		return err
	}
	deviceID := strings.TrimSpace(device.ID)
	if deviceID == "" {
		return nil
	}

	if ctx == nil {
		ctx = context.Background()
	}

	return db.withSerializedConnectionFor(ctx, WorkloadArchive, func(ctx context.Context, conn *sql.DB) error {
		placeholder := newPlaceholderGenerator(dbType)
		idPlaceholder := placeholder()
		query := fmt.Sprintf("SELECT device_id FROM tracks WHERE trackID = %s", idPlaceholder)
		var existing sql.NullString
		err := conn.QueryRowContext(ctx, query, trackID).Scan(&existing)
		if err != nil && err != sql.ErrNoRows {
			return fmt.Errorf("read track device: %w", err)
		}

		if err == sql.ErrNoRows {
			insert := fmt.Sprintf("INSERT INTO tracks (trackID, device_id) VALUES (%s, %s)", placeholder(), placeholder())
			if _, err := conn.ExecContext(ctx, insert, trackID, deviceID); err != nil {
				return fmt.Errorf("insert track device: %w", err)
			}
			return nil
		}

		if existing.Valid && existing.String == deviceID {
			return nil
		}
		update := fmt.Sprintf("UPDATE tracks SET device_id = %s WHERE trackID = %s", placeholder(), placeholder())
		if _, err := conn.ExecContext(ctx, update, deviceID, trackID); err != nil {
			return fmt.Errorf("update track device: %w", err)
		}
		return nil
	})
}

// TrackDeviceID returns the device ID assigned to a track, if any.
// Returning an empty string is safe for callers that only need a best effort check.
func (db *Database) TrackDeviceID(ctx context.Context, trackID string, dbType string) (string, error) {
	trackID = strings.TrimSpace(trackID)
	if trackID == "" {
		return "", nil
	}
	if ctx == nil {
		ctx = context.Background()
	}

	var deviceID sql.NullString
	err := db.withSerializedConnectionFor(ctx, WorkloadWebRead, func(ctx context.Context, conn *sql.DB) error {
		placeholder := newPlaceholderGenerator(dbType)
		query := fmt.Sprintf("SELECT device_id FROM tracks WHERE trackID = %s", placeholder())
		return conn.QueryRowContext(ctx, query, trackID).Scan(&deviceID)
	})
	if err != nil && err != sql.ErrNoRows {
		return "", err
	}
	if !deviceID.Valid {
		return "", nil
	}
	return deviceID.String, nil
}

func nullableDeviceModel(device Device) any {
	if strings.TrimSpace(device.Model) == "" {
		return nil
	}
	return device.Model
}
