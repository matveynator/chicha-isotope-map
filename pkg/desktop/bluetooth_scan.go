package desktop

import (
	"bufio"
	"context"
	"fmt"
	"os/exec"
	"runtime"
	"strings"
	"time"
)

// BluetoothDevice keeps the minimal discovery payload exposed to the UI.
type BluetoothDevice struct {
	Name    string `json:"name"`
	Address string `json:"address"`
}

// ScanRadiacodeDevices performs an active discovery where possible and returns
// only Radiacode-like devices.
func ScanRadiacodeDevices(ctx context.Context) ([]BluetoothDevice, error) {
	if ctx == nil {
		ctx = context.Background()
	}

	switch runtime.GOOS {
	case "linux":
		return scanRadiacodeLinuxActive(ctx)
	case "darwin":
		return scanRadiacodeDarwin(ctx)
	case "windows":
		return scanRadiacodeWindows(ctx)
	default:
		return nil, fmt.Errorf("bluetooth scan unsupported on %s", runtime.GOOS)
	}
}

// ConnectRadiacodeDevice asks the host Bluetooth stack to connect to the device.
func ConnectRadiacodeDevice(ctx context.Context, address string) error {
	address = strings.TrimSpace(address)
	if address == "" {
		return fmt.Errorf("empty bluetooth address")
	}
	if ctx == nil {
		ctx = context.Background()
	}

	switch runtime.GOOS {
	case "linux":
		output, err := runCommand(ctx, "bluetoothctl", "connect", address)
		if err != nil {
			return err
		}
		if !strings.Contains(strings.ToLower(output), "connection successful") {
			return fmt.Errorf("connect failed: %s", strings.TrimSpace(output))
		}
		return nil
	case "darwin", "windows":
		return fmt.Errorf("native bluetooth connect is not implemented on %s yet; use Chrome/Edge Web Bluetooth", runtime.GOOS)
	default:
		return fmt.Errorf("bluetooth connect unsupported on %s", runtime.GOOS)
	}
}

func scanRadiacodeLinuxActive(ctx context.Context) ([]BluetoothDevice, error) {
	scanCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	output, err := runCommand(scanCtx, "bluetoothctl", "--timeout", "8", "scan", "on")
	if err != nil {
		return nil, err
	}
	_, _ = runCommand(context.Background(), "bluetoothctl", "scan", "off")

	radiacodeDevices := parseLinuxScanOutput(output)
	if len(radiacodeDevices) > 0 {
		return radiacodeDevices, nil
	}

	fallbackOutput, fallbackErr := runCommand(ctx, "bluetoothctl", "devices")
	if fallbackErr != nil {
		return nil, fallbackErr
	}
	return parseLinuxKnownDevices(fallbackOutput), nil
}

func parseLinuxScanOutput(raw string) []BluetoothDevice {
	deviceByAddress := make(map[string]BluetoothDevice)
	scanner := bufio.NewScanner(strings.NewReader(raw))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if !(strings.Contains(line, "[NEW] Device") || strings.Contains(line, "[CHG] Device") || strings.HasPrefix(line, "Device ")) {
			continue
		}
		address, name := splitAddressAndName(line)
		if address == "" || !isRadiacodeDeviceName(name) {
			continue
		}
		deviceByAddress[address] = BluetoothDevice{Name: name, Address: address}
	}
	return mapValues(deviceByAddress)
}

func parseLinuxKnownDevices(raw string) []BluetoothDevice {
	deviceByAddress := make(map[string]BluetoothDevice)
	scanner := bufio.NewScanner(strings.NewReader(raw))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if !strings.HasPrefix(line, "Device ") {
			continue
		}
		address, name := splitAddressAndName(line)
		if address == "" || !isRadiacodeDeviceName(name) {
			continue
		}
		deviceByAddress[address] = BluetoothDevice{Name: name, Address: address}
	}
	return mapValues(deviceByAddress)
}

func scanRadiacodeDarwin(ctx context.Context) ([]BluetoothDevice, error) {
	output, err := runCommand(ctx, "system_profiler", "SPBluetoothDataType")
	if err != nil {
		return nil, err
	}

	devices := make([]BluetoothDevice, 0)
	scanner := bufio.NewScanner(strings.NewReader(output))
	currentName := ""
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if strings.HasSuffix(line, ":") {
			candidateName := strings.TrimSpace(strings.TrimSuffix(line, ":"))
			if isRadiacodeDeviceName(candidateName) {
				currentName = candidateName
				devices = append(devices, BluetoothDevice{Name: candidateName})
			}
			continue
		}
		if currentName == "" || !strings.HasPrefix(strings.ToLower(line), "address:") {
			continue
		}
		address := strings.TrimSpace(line[len("Address:"):])
		if len(devices) > 0 {
			devices[len(devices)-1].Address = address
		}
		currentName = ""
	}
	return devices, nil
}

func scanRadiacodeWindows(ctx context.Context) ([]BluetoothDevice, error) {
	output, err := runCommand(ctx, "powershell", "-NoProfile", "-Command", "Get-PnpDevice -Class Bluetooth | Select-Object FriendlyName | Format-Table -HideTableHeaders")
	if err != nil {
		return nil, err
	}

	devices := make([]BluetoothDevice, 0)
	scanner := bufio.NewScanner(strings.NewReader(output))
	for scanner.Scan() {
		name := strings.TrimSpace(scanner.Text())
		if !isRadiacodeDeviceName(name) {
			continue
		}
		devices = append(devices, BluetoothDevice{Name: name})
	}
	return devices, nil
}

func runCommand(ctx context.Context, name string, args ...string) (string, error) {
	command := exec.CommandContext(ctx, name, args...)
	output, err := command.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("%s failed: %w (%s)", name, err, strings.TrimSpace(string(output)))
	}
	return string(output), nil
}

func splitAddressAndName(line string) (string, string) {
	parts := strings.Fields(line)
	if len(parts) < 3 {
		return "", ""
	}
	addressIndex := -1
	for index := 0; index < len(parts); index++ {
		if strings.Count(parts[index], ":") == 5 {
			addressIndex = index
			break
		}
	}
	if addressIndex == -1 || addressIndex+1 >= len(parts) {
		return "", ""
	}
	address := strings.TrimSpace(parts[addressIndex])
	name := strings.TrimSpace(strings.Join(parts[addressIndex+1:], " "))
	return address, name
}

func mapValues(deviceByAddress map[string]BluetoothDevice) []BluetoothDevice {
	devices := make([]BluetoothDevice, 0, len(deviceByAddress))
	for _, device := range deviceByAddress {
		devices = append(devices, device)
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
