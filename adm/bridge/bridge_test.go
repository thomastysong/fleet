package bridge

import (
	"context"
	"strings"
	"testing"

	"github.com/fleetdm/fleet/v4/adm/device"
	"github.com/fleetdm/fleet/v4/adm/intent"
	"github.com/fleetdm/fleet/v4/adm/native"
	"github.com/fleetdm/fleet/v4/adm/native/darwin"
	"github.com/fleetdm/fleet/v4/adm/native/windows"
	"github.com/fleetdm/fleet/v4/adm/substrate"
)

func TestWindowsBridgeReadsViaMDMBridgeAndQueuesWrites(t *testing.T) {
	mem := substrate.NewMemory(substrate.DemoFleet()...)
	mem.Respond("mdm_bridge", func(h device.Host) []map[string]string {
		return []map[string]string{{"raw_mdm_command_output": `<SyncML xmlns="SYNCML:SYNCML1.2"><SyncBody><Status><CmdRef>1</CmdRef><Data>200</Data></Status><Results><CmdRef>1</CmdRef><Item><Source><LocURI>./Device/Vendor/MSFT/Policy/Config/Storage/RemovableDiskDenyWriteAccess</LocURI></Source><Data>0</Data></Item></Results></SyncBody></SyncML>`}}
	})
	b := NewWindowsMDM(mem)
	reg := native.NewRegistry()
	reg.MustRegister(windows.NewAdapters(b)...)
	a, _ := reg.Lookup("storage.removable.block", intent.PlatformWindows)
	ctx := context.Background()
	tgt := native.Target{HostID: 3, UUID: "u3", Platform: intent.PlatformWindows}
	st, err := a.Check(ctx, tgt, map[string]any{"mode": "read_only"})
	if err != nil || st.IsSatisfied() {
		t.Fatalf("%v %v", st, err)
	}
	res, err := a.Apply(ctx, tgt, map[string]any{"mode": "read_only"})
	if err != nil || !res.Changed {
		t.Fatalf("%+v %v", res, err)
	}
	if len(mem.Commands) != 1 || mem.Commands[0].HostUUIDs[0] != "u3" || !strings.Contains(string(mem.Commands[0].Raw), "<Replace>") {
		t.Fatalf("write not queued: %+v", mem.Commands)
	}
	// An offline host cannot be read.
	if _, err := a.Check(ctx, native.Target{HostID: 4, UUID: "u4"}, map[string]any{"mode": "read_only"}); err == nil {
		t.Fatal("expected offline error")
	}
	if _, err := b.Apply(ctx, native.Target{HostID: 3}, "<SyncBody><Exec><CmdID>1</CmdID><Item><Target><LocURI>./Vendor/MSFT/RemoteLock/Lock</LocURI></Target></Item></Exec></SyncBody>"); err == nil {
		t.Fatal("write without UUID should fail")
	}
}

func TestApplePusher(t *testing.T) {
	mem := substrate.NewMemory(substrate.DemoFleet()...)
	p := NewApplePusher(mem, "Engineering", []string{"laptops"})
	reg := native.NewRegistry()
	reg.MustRegister(darwin.NewAdapters(p)...)
	a, _ := reg.Lookup("screen.lock.enforce", intent.PlatformDarwin)
	ctx := context.Background()
	res, err := a.Apply(ctx, native.Target{HostID: 1, Platform: intent.PlatformDarwin}, map[string]any{"idle_minutes": 5})
	if err != nil || !res.Changed {
		t.Fatalf("%+v %v", res, err)
	}
	if len(mem.Profiles) != 1 || mem.Profiles[0].Fleet != "Engineering" || !strings.Contains(string(mem.Profiles[0].Contents), "com.apple.configuration.screensaver.settings") {
		t.Fatalf("%+v", mem.Profiles)
	}
	if _, err := a.Revert(ctx, native.Target{}, map[string]any{"idle_minutes": 5}, nil); err == nil {
		t.Fatal("remove should report unsupported")
	}
}
