// Package linux realises ADM capabilities on Linux through the kernel and
// desktop's own interfaces: sysfs and udev for removable storage, dconf
// lockdown for the desktop session, D-Bus services (systemd, login1,
// NetworkManager, PackageKit, UDisks2) for everything else. Privileged
// operations go through a small Runner interface so that adapters stay pure
// and testable and the agent decides how to execute them.
package linux

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/fleetdm/fleet/v4/adm/intent"
	"github.com/fleetdm/fleet/v4/adm/native"
)

// Runner executes a privileged, non-shell command (argv, no interpreter).
type Runner interface {
	Run(ctx context.Context, argv ...string) (string, error)
}

// FS abstracts the files the adapters manage so they can be tested in a
// temporary root.
type FS struct {
	// Root is prefixed to every path; "" means the real filesystem.
	Root string
}

func (f FS) path(p string) string { return filepath.Join(f.Root, p) }

func (f FS) write(p string, data []byte) error {
	full := f.path(p)
	if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
		return err
	}
	return os.WriteFile(full, data, 0o644)
}

func (f FS) remove(p string) error {
	err := os.Remove(f.path(p))
	if os.IsNotExist(err) {
		return nil
	}
	return err
}

func (f FS) exists(p string) bool {
	_, err := os.Stat(f.path(p))
	return err == nil
}

// ---- removable storage -----------------------------------------------------

// BlockDevice is a removable block device observed through sysfs.
type BlockDevice struct {
	Name      string `json:"name"`
	Vendor    string `json:"vendor,omitempty"`
	Model     string `json:"model,omitempty"`
	SizeBytes int64  `json:"size_bytes"`
}

// RemovableDevices lists removable block devices by reading
// /sys/block/*/removable, the same source udev uses.
func RemovableDevices(fs FS) ([]BlockDevice, error) {
	entries, err := os.ReadDir(fs.path("/sys/block"))
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var out []BlockDevice
	for _, e := range entries {
		base := "/sys/block/" + e.Name()
		if strings.TrimSpace(readFile(fs, base+"/removable")) != "1" {
			continue
		}
		dev := BlockDevice{Name: e.Name(), Vendor: strings.TrimSpace(readFile(fs, base+"/device/vendor")), Model: strings.TrimSpace(readFile(fs, base+"/device/model"))}
		var sectors int64
		fmt.Sscanf(strings.TrimSpace(readFile(fs, base+"/size")), "%d", &sectors)
		dev.SizeBytes = sectors * 512
		out = append(out, dev)
	}
	return out, nil
}

func readFile(fs FS, p string) string {
	b, err := os.ReadFile(fs.path(p))
	if err != nil {
		return ""
	}
	return string(b)
}

const udevRulePath = "/etc/udev/rules.d/90-adm-removable-storage.rules"

// UdevRule renders the rule that blocks (or makes read-only) USB block
// devices. Blocking uses the kernel's authorization attribute so the device
// never binds; read-only flips the block device's ro flag at add time.
func UdevRule(mode string) (string, error) {
	switch mode {
	case "block":
		return "# Managed by ADM (storage.removable.block mode=block)\n" +
			"ACTION==\"add\", SUBSYSTEM==\"usb\", ATTR{bInterfaceClass}==\"08\", ATTR{authorized}=\"0\"\n" +
			"ACTION==\"add\", SUBSYSTEM==\"block\", ENV{ID_BUS}==\"usb\", ENV{UDISKS_IGNORE}=\"1\"\n", nil
	case "read_only":
		return "# Managed by ADM (storage.removable.block mode=read_only)\n" +
			"ACTION==\"add\", SUBSYSTEM==\"block\", ENV{ID_BUS}==\"usb\", RUN+=\"/sbin/blockdev --setro /dev/%k\"\n", nil
	}
	return "", fmt.Errorf("storage.removable.block: mode must be block or read_only, got %q", mode)
}

// RemovableStorageAdapter implements storage.removable.block on Linux.
type RemovableStorageAdapter struct {
	fs     FS
	runner Runner
}

// NewRemovableStorageAdapter builds the adapter.
func NewRemovableStorageAdapter(fs FS, r Runner) *RemovableStorageAdapter {
	return &RemovableStorageAdapter{fs: fs, runner: r}
}

func (a *RemovableStorageAdapter) ID() string                { return "linux.udev.removable" }
func (a *RemovableStorageAdapter) Capability() string        { return "storage.removable.block" }
func (a *RemovableStorageAdapter) Platform() intent.Platform { return intent.PlatformLinux }

// Check reports the rule state and the currently attached removable devices.
func (a *RemovableStorageAdapter) Check(ctx context.Context, t native.Target, params map[string]any) (native.State, error) {
	mode, _ := params["mode"].(string)
	want, err := UdevRule(mode)
	if err != nil {
		return nil, err
	}
	devices, err := RemovableDevices(a.fs)
	if err != nil {
		return nil, err
	}
	current := readFile(a.fs, udevRulePath)
	return native.State{
		"rule":              current,
		"removable_devices": devices,
		native.Satisfied:    current == want,
	}, nil
}

// Apply writes the rule, reloads udev and re-triggers the block subsystem so
// already-attached devices are re-evaluated immediately.
func (a *RemovableStorageAdapter) Apply(ctx context.Context, t native.Target, params map[string]any) (native.Result, error) {
	mode, _ := params["mode"].(string)
	rule, err := UdevRule(mode)
	if err != nil {
		return native.Result{}, err
	}
	start := time.Now()
	if err := a.fs.write(udevRulePath, []byte(rule)); err != nil {
		return native.Result{}, err
	}
	for _, argv := range [][]string{{"udevadm", "control", "--reload"}, {"udevadm", "trigger", "--subsystem-match=block", "--action=add"}} {
		if _, err := a.runner.Run(ctx, argv...); err != nil {
			return native.Result{}, fmt.Errorf("%s: %w", strings.Join(argv, " "), err)
		}
	}
	return native.Result{Changed: true, Native: "udev " + udevRulePath, Duration: time.Since(start)}, nil
}

// Revert restores the prior rule (or removes ours).
func (a *RemovableStorageAdapter) Revert(ctx context.Context, t native.Target, params map[string]any, prior native.State) (native.Result, error) {
	prev, _ := prior["rule"].(string)
	var err error
	if prev == "" {
		err = a.fs.remove(udevRulePath)
	} else {
		err = a.fs.write(udevRulePath, []byte(prev))
	}
	if err != nil {
		return native.Result{}, err
	}
	if _, err := a.runner.Run(ctx, "udevadm", "control", "--reload"); err != nil {
		return native.Result{}, err
	}
	return native.Result{Changed: true, Native: "udev restore " + udevRulePath}, nil
}

// ---- screen lock (dconf lockdown) -------------------------------------------

const (
	dconfProfile  = "/etc/dconf/profile/user"
	dconfSettings = "/etc/dconf/db/local.d/90-adm-screenlock"
	dconfLocks    = "/etc/dconf/db/local.d/locks/90-adm-screenlock"
)

// ScreenLockSettings renders the dconf keyfile for an idle timeout.
func ScreenLockSettings(idleMinutes int) (string, error) {
	if idleMinutes <= 0 {
		return "", fmt.Errorf("screen.lock.enforce: idle_minutes must be a positive integer")
	}
	return fmt.Sprintf("# Managed by ADM (screen.lock.enforce)\n[org/gnome/desktop/session]\nidle-delay=uint32 %d\n\n[org/gnome/desktop/screensaver]\nlock-enabled=true\nlock-delay=uint32 0\n", idleMinutes*60), nil
}

const screenLockLocks = "/org/gnome/desktop/session/idle-delay\n/org/gnome/desktop/screensaver/lock-enabled\n/org/gnome/desktop/screensaver/lock-delay\n"

// ScreenLockAdapter implements screen.lock.enforce for GNOME sessions.
type ScreenLockAdapter struct {
	fs     FS
	runner Runner
}

// NewScreenLockAdapter builds the adapter.
func NewScreenLockAdapter(fs FS, r Runner) *ScreenLockAdapter {
	return &ScreenLockAdapter{fs: fs, runner: r}
}

func (a *ScreenLockAdapter) ID() string                { return "linux.dconf.screenlock" }
func (a *ScreenLockAdapter) Capability() string        { return "screen.lock.enforce" }
func (a *ScreenLockAdapter) Platform() intent.Platform { return intent.PlatformLinux }

func (a *ScreenLockAdapter) Check(ctx context.Context, t native.Target, params map[string]any) (native.State, error) {
	n, _ := asInt(params["idle_minutes"])
	want, err := ScreenLockSettings(n)
	if err != nil {
		return nil, err
	}
	cur := readFile(a.fs, dconfSettings)
	return native.State{"settings": cur, "locks": readFile(a.fs, dconfLocks), native.Satisfied: cur == want && a.fs.exists(dconfLocks)}, nil
}

func (a *ScreenLockAdapter) Apply(ctx context.Context, t native.Target, params map[string]any) (native.Result, error) {
	n, _ := asInt(params["idle_minutes"])
	settings, err := ScreenLockSettings(n)
	if err != nil {
		return native.Result{}, err
	}
	start := time.Now()
	if !a.fs.exists(dconfProfile) {
		if err := a.fs.write(dconfProfile, []byte("user-db:user\nsystem-db:local\n")); err != nil {
			return native.Result{}, err
		}
	}
	if err := a.fs.write(dconfSettings, []byte(settings)); err != nil {
		return native.Result{}, err
	}
	if err := a.fs.write(dconfLocks, []byte(screenLockLocks)); err != nil {
		return native.Result{}, err
	}
	if _, err := a.runner.Run(ctx, "dconf", "update"); err != nil {
		return native.Result{}, err
	}
	return native.Result{Changed: true, Native: "dconf lockdown " + dconfSettings, Duration: time.Since(start)}, nil
}

func (a *ScreenLockAdapter) Revert(ctx context.Context, t native.Target, params map[string]any, prior native.State) (native.Result, error) {
	prev, _ := prior["settings"].(string)
	var err error
	if prev == "" {
		if err = a.fs.remove(dconfSettings); err == nil {
			err = a.fs.remove(dconfLocks)
		}
	} else {
		err = a.fs.write(dconfSettings, []byte(prev))
	}
	if err != nil {
		return native.Result{}, err
	}
	if _, err := a.runner.Run(ctx, "dconf", "update"); err != nil {
		return native.Result{}, err
	}
	return native.Result{Changed: true, Native: "dconf restore"}, nil
}

// ---- systemd / login1 via D-Bus ---------------------------------------------

// DBus is the narrow D-Bus surface the adapters need. The agent provides an
// implementation over the system bus; tests provide a fake.
type DBus interface {
	Call(ctx context.Context, dest, path, iface, method string, args ...any) ([]any, error)
}

// LockAdapter implements host.lock through org.freedesktop.login1.
type LockAdapter struct{ bus DBus }

// NewLockAdapter builds the adapter.
func NewLockAdapter(bus DBus) *LockAdapter { return &LockAdapter{bus: bus} }

func (a *LockAdapter) ID() string                { return "linux.login1.lock" }
func (a *LockAdapter) Capability() string        { return "host.lock" }
func (a *LockAdapter) Platform() intent.Platform { return intent.PlatformLinux }

func (a *LockAdapter) Check(context.Context, native.Target, map[string]any) (native.State, error) {
	return native.State{native.Satisfied: false}, nil
}

func (a *LockAdapter) Apply(ctx context.Context, t native.Target, params map[string]any) (native.Result, error) {
	start := time.Now()
	if _, err := a.bus.Call(ctx, "org.freedesktop.login1", "/org/freedesktop/login1", "org.freedesktop.login1.Manager", "LockSessions"); err != nil {
		return native.Result{}, err
	}
	return native.Result{Changed: true, Native: "org.freedesktop.login1.Manager.LockSessions", Duration: time.Since(start)}, nil
}

func (a *LockAdapter) Revert(ctx context.Context, t native.Target, params map[string]any, prior native.State) (native.Result, error) {
	if _, err := a.bus.Call(ctx, "org.freedesktop.login1", "/org/freedesktop/login1", "org.freedesktop.login1.Manager", "UnlockSessions"); err != nil {
		return native.Result{}, err
	}
	return native.Result{Changed: true, Native: "org.freedesktop.login1.Manager.UnlockSessions"}, nil
}

// NewAdapters returns the Linux adapters.
func NewAdapters(fs FS, r Runner, bus DBus) []native.Adapter {
	return []native.Adapter{
		NewRemovableStorageAdapter(fs, r),
		NewScreenLockAdapter(fs, r),
		NewLockAdapter(bus),
	}
}

func asInt(v any) (int, bool) {
	switch n := v.(type) {
	case int:
		return n, true
	case int64:
		return int(n), true
	case float64:
		return int(n), true
	}
	return 0, false
}
