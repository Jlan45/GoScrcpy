package adb

import (
	"fmt"
	"log"
	"os/exec"
	"strings"
)

const (
	DefaultServerDevicePath = "/data/local/tmp/scrcpy-server.jar"
	DefaultPortRangeStart   = 27183
	DefaultPortRangeEnd     = 27199
)

// DeviceInfo represents a connected ADB device.
type DeviceInfo struct {
	Serial string
	State  string // "device", "offline", "unauthorized", etc.
}

// ListDevices returns all connected ADB devices.
func ListDevices() ([]DeviceInfo, error) {
	out, err := exec.Command("adb", "devices").CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("adb devices: %w: %s", err, string(out))
	}
	var devices []DeviceInfo
	for _, line := range strings.Split(string(out), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "List of") || strings.HasPrefix(line, "*") {
			continue
		}
		parts := strings.Fields(line)
		if len(parts) >= 2 {
			devices = append(devices, DeviceInfo{Serial: parts[0], State: parts[1]})
		}
	}
	return devices, nil
}

// Push pushes a local file to the device.
func Push(serial, localPath, remotePath string) error {
	return run(serial, "push", localPath, remotePath)
}

// Forward sets up an adb forward tunnel.
func Forward(serial, local, remote string) error {
	return run(serial, "forward", local, remote)
}

// Reverse sets up an adb reverse tunnel.
func Reverse(serial, remote, local string) error {
	return run(serial, "reverse", remote, local)
}

// RemoveForward removes an adb forward tunnel.
func RemoveForward(serial, local string) error {
	return run(serial, "forward", "--remove", local)
}

// RemoveReverse removes an adb reverse tunnel.
func RemoveReverse(serial, remote string) error {
	return run(serial, "reverse", "--remove", remote)
}

// StartServer launches the scrcpy server on the device and returns the running command.
// The caller is responsible for killing the process when done.
func StartServer(serial, version string, params map[string]string) (*exec.Cmd, error) {
	args := serialArgs(serial)
	args = append(args, "shell",
		fmt.Sprintf("CLASSPATH=%s", DefaultServerDevicePath),
		"app_process", "/", "com.genymobile.scrcpy.Server", version,
	)
	for k, v := range params {
		args = append(args, fmt.Sprintf("%s=%s", k, v))
	}

	log.Printf("[adb] launching: adb %s", strings.Join(args, " "))

	cmd := exec.Command("adb", args...)
	// Pipe server output to our logger for debugging
	cmd.Stdout = &logWriter{prefix: "[server]"}
	cmd.Stderr = &logWriter{prefix: "[server]"}
	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("start server: %w", err)
	}
	return cmd, nil
}

// logWriter writes each line to log.Printf.
type logWriter struct {
	prefix string
}

func (w *logWriter) Write(p []byte) (int, error) {
	s := strings.TrimRight(string(p), "\n\r")
	if s != "" {
		log.Printf("%s %s", w.prefix, s)
	}
	return len(p), nil
}

func serialArgs(serial string) []string {
	if serial == "" {
		return nil
	}
	return []string{"-s", serial}
}

func run(serial string, adbArgs ...string) error {
	args := append(serialArgs(serial), adbArgs...)
	out, err := exec.Command("adb", args...).CombinedOutput()
	if err != nil {
		return fmt.Errorf("adb %s: %w: %s", strings.Join(adbArgs, " "), err, string(out))
	}
	return nil
}
