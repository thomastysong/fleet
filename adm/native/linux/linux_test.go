package linux

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/fleetdm/fleet/v4/adm/native"
)

type fakeRunner struct{ calls [][]string }

func (f *fakeRunner) Run(_ context.Context, argv ...string) (string, error) {
	f.calls = append(f.calls, argv)
	return "", nil
}

type fakeBus struct{ calls []string }

func (f *fakeBus) Call(_ context.Context, dest, path, iface, method string, _ ...any) ([]any, error) {
	f.calls = append(f.calls, iface+"."+method)
	return nil, nil
}

func sysfs(t *testing.T, root string) {
	t.Helper()
	for name, files := range map[string]map[string]string{
		"sda": {"removable": "0", "size": "1000000"},
		"sdb": {"removable": "1", "size": "61440000", "device/vendor": "SanDisk ", "device/model": "Cruzer"},
	} {
		for f, v := range files {
			p := filepath.Join(root, "sys/block", name, f)
			if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(p, []byte(v+"\n"), 0o644); err != nil {
				t.Fatal(err)
			}
		}
	}
}

func TestRemovableDevices(t *testing.T) {
	root := t.TempDir()
	sysfs(t, root)
	devs, err := RemovableDevices(FS{Root: root})
	if err != nil {
		t.Fatal(err)
	}
	if len(devs) != 1 || devs[0].Name != "sdb" || devs[0].Vendor != "SanDisk" || devs[0].SizeBytes != 61440000*512 {
		t.Fatalf("%+v", devs)
	}
	if devs, _ := RemovableDevices(FS{Root: t.TempDir()}); len(devs) != 0 {
		t.Fatal("missing sysfs should yield no devices")
	}
}

func TestRemovableStorageAdapter(t *testing.T) {
	root := t.TempDir()
	sysfs(t, root)
	fs := FS{Root: root}
	r := &fakeRunner{}
	a := NewRemovableStorageAdapter(fs, r)
	ctx := context.Background()
	params := map[string]any{"mode": "block"}
	prior, err := a.Check(ctx, native.Target{}, params)
	if err != nil || prior.IsSatisfied() {
		t.Fatalf("%v %v", prior, err)
	}
	if devs := prior["removable_devices"].([]BlockDevice); len(devs) != 1 {
		t.Fatalf("expected the attached stick to be reported: %+v", devs)
	}
	res, err := a.Apply(ctx, native.Target{}, params)
	if err != nil || !res.Changed {
		t.Fatalf("%+v %v", res, err)
	}
	if b, _ := os.ReadFile(filepath.Join(root, udevRulePath)); !strings.Contains(string(b), "ATTR{authorized}=\"0\"") {
		t.Fatalf("rule not written: %s", b)
	}
	if len(r.calls) != 2 || r.calls[0][0] != "udevadm" {
		t.Fatalf("udev not reloaded: %v", r.calls)
	}
	after, _ := a.Check(ctx, native.Target{}, params)
	if !after.IsSatisfied() {
		t.Fatal("expected satisfied")
	}
	if _, err := a.Revert(ctx, native.Target{}, params, prior); err != nil {
		t.Fatal(err)
	}
	if fs.exists(udevRulePath) {
		t.Fatal("revert should remove the rule when there was none before")
	}
	if _, err := a.Apply(ctx, native.Target{}, map[string]any{"mode": "nope"}); err == nil {
		t.Fatal("expected mode error")
	}
}

func TestScreenLockAdapter(t *testing.T) {
	fs := FS{Root: t.TempDir()}
	r := &fakeRunner{}
	a := NewScreenLockAdapter(fs, r)
	ctx := context.Background()
	params := map[string]any{"idle_minutes": 10}
	st, _ := a.Check(ctx, native.Target{}, params)
	if st.IsSatisfied() {
		t.Fatal("should not be satisfied")
	}
	if _, err := a.Apply(ctx, native.Target{}, params); err != nil {
		t.Fatal(err)
	}
	b, _ := os.ReadFile(fs.path(dconfSettings))
	if !strings.Contains(string(b), "idle-delay=uint32 600") || !fs.exists(dconfLocks) || !fs.exists(dconfProfile) {
		t.Fatalf("dconf files wrong: %s", b)
	}
	st, _ = a.Check(ctx, native.Target{}, params)
	if !st.IsSatisfied() {
		t.Fatal("expected satisfied")
	}
	if len(r.calls) != 1 || r.calls[0][0] != "dconf" {
		t.Fatalf("dconf update not run: %v", r.calls)
	}
	if _, err := a.Revert(ctx, native.Target{}, params, native.State{"settings": ""}); err != nil {
		t.Fatal(err)
	}
	if fs.exists(dconfSettings) {
		t.Fatal("revert should remove settings")
	}
}

func TestLockAdapterAndRegistry(t *testing.T) {
	bus := &fakeBus{}
	reg := native.NewRegistry()
	reg.MustRegister(NewAdapters(FS{Root: t.TempDir()}, &fakeRunner{}, bus)...)
	if reg.Len() != 3 {
		t.Fatalf("registered %d", reg.Len())
	}
	lock, _ := reg.Lookup("host.lock", "linux")
	if _, err := lock.Apply(context.Background(), native.Target{}, nil); err != nil {
		t.Fatal(err)
	}
	if len(bus.calls) != 1 || bus.calls[0] != "org.freedesktop.login1.Manager.LockSessions" {
		t.Fatalf("%v", bus.calls)
	}
}
