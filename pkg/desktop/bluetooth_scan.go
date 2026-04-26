package desktop

import (
	"bufio"
	"context"
	"fmt"
	"os/exec"
	"runtime"
	"strings"
)

// BluetoothDevice keeps the minimal discovery payload exposed to the UI.
type BluetoothDevice struct {
	Name    string `json:"name"`
	Address string `json:"address"`
}

// ScanRadiacodeDevices discovers nearby or known Bluetooth devices and returns
// only Radiacode-like entries. The implementation uses small OS-native commands
// so server builds avoid heavy BLE dependencies.
func ScanRadiacodeDevices(ctx context.Context) ([]BluetoothDevice, error) {
	if ctx == nil {
		ctx = context.Background()
	}

	var (
		rawOutput string
		err       error
	)

	switch runtime.GOOS {
	case "linux":
		rawOutput, err = runCommand(ctx, "bluetoothctl", "devices")
	case "darwin":
		rawOutput, err = runCommand(ctx, "system_profiler", "SPBluetoothDataType")
	case "windows":
		rawOutput, err = runCommand(ctx, "powershell", "-NoProfile", "-Command", "Get-PnpDevice -Class Bluetooth | Select-Object FriendlyName,InstanceId | Format-Table -HideTableHeaders")
	default:
		return nil, fmt.Errorf("bluetooth scan unsupported on %s", runtime.GOOS)
	}
	if err != nil {
		return nil, err
	}

	switch runtime.GOOS {
	case "linux":
		return parseLinuxBluetoothctlDevices(rawOutput), nil
	case "darwin":
		return parseDarwinSystemProfiler(rawOutput), nil
	case "windows":
		return parseWindowsPnp(rawOutput), nil
	default:
		return nil, nil
	}
}

func runCommand(ctx context.Context, name string, args ...string) (string, error) {
	command := exec.CommandContext(ctx, name, args...)
	output, err := command.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("%s failed: %w (%s)", name, err, strings.TrimSpace(string(output)))
	}
	return string(output), nil
}

func parseLinuxBluetoothctlDevices(raw string) []BluetoothDevice {
	devices := make([]BluetoothDevice, 0)
	scanner := bufio.NewScanner(strings.NewReader(raw))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if !strings.HasPrefix(line, "Device ") {
			continue
		}
		parts := strings.Fields(line)
		if len(parts) < 3 {
			continue
		}
		address := strings.TrimSpace(parts[1])
		name := strings.TrimSpace(strings.Join(parts[2:], " "))
		if !isRadiacodeDeviceName(name) {
			continue
		}
		devices = append(devices, BluetoothDevice{Name: name, Address: address})
	}
	return devices
}

func parseDarwinSystemProfiler(raw string) []BluetoothDevice {
	devices := make([]BluetoothDevice, 0)
	scanner := bufio.NewScanner(strings.NewReader(raw))
	currentName := ""
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if strings.HasSuffix(line, ":") && isRadiacodeDeviceName(strings.TrimSuffix(line, ":")) {
			currentName = strings.TrimSpace(strings.TrimSuffix(line, ":"))
			devices = append(devices, BluetoothDevice{Name: currentName})
			continue
		}
		if currentName == "" || !strings.HasPrefix(strings.ToLower(line), "address:") {
			continue
		}
		address := strings.TrimSpace(strings.TrimPrefix(line, "Address:"))
		address = strings.TrimSpace(strings.TrimPrefix(address, "address:"))
		if len(devices) > 0 {
			devices[len(devices)-1].Address = address
		}
		currentName = ""
	}
	return devices
}

func parseWindowsPnp(raw string) []BluetoothDevice {
	devices := make([]BluetoothDevice, 0)
	scanner := bufio.NewScanner(strings.NewReader(raw))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || !isRadiacodeDeviceName(line) {
			continue
		}
		devices = append(devices, BluetoothDevice{Name: line})
	}
	return devices
}

func isRadiacodeDeviceName(name string) bool {
	nameLower := strings.ToLower(strings.TrimSpace(name))
	if nameLower == "" {
		return false
	}
	return strings.Contains(nameLower, "radiacode") || strings.HasPrefix(nameLower, "rc-") || strings.Contains(nameLower, "radiacode 101") || strings.Contains(nameLower, "radiacode 102") || strings.Contains(nameLower, "radiacode 103") || strings.Contains(nameLower, "radiacode 110") || strings.Contains(nameLower, "radiacode zero")
}
