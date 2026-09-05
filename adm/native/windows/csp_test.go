package windows

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/fleetdm/fleet/v4/adm/native"
)

// fakeMDM emulates the local management stack: it stores node values and
// answers Get/Replace/Add like the OS does (404 on Replace of a missing node).
type fakeMDM struct {
	nodes map[string]string
	calls []string
}

func (f *fakeMDM) Apply(_ context.Context, _ native.Target, body string) (string, error) {
	f.calls = append(f.calls, body)
	var out strings.Builder
	out.WriteString("<SyncML xmlns=\"SYNCML:SYNCML1.2\"><SyncBody>")
	// crude parse: iterate commands in order
	cmdID := 0
	for _, verb := range []string{"Get", "Replace", "Add", "Exec", "Delete"} {
		parts := strings.Split(body, "<"+verb+">")
		for _, part := range parts[1:] {
			cmdID++
			loc := between(part, "<LocURI>", "</LocURI>")
			data := between(part, "<Data>", "</Data>")
			status := "200"
			switch verb {
			case "Get":
				if v, ok := f.nodes[loc]; ok {
					fmt.Fprintf(&out, "<Results><CmdRef>%d</CmdRef><Item><Source><LocURI>%s</LocURI></Source><Data>%s</Data></Item></Results>", cmdID, loc, v)
				} else {
					status = "404"
				}
			case "Replace":
				if _, ok := f.nodes[loc]; ok {
					f.nodes[loc] = data
				} else {
					status = "404"
				}
			case "Add":
				f.nodes[loc] = data
			case "Exec":
				f.nodes[loc+"#exec"] = "1"
			}
			fmt.Fprintf(&out, "<Status><CmdRef>%d</CmdRef><Data>%s</Data></Status>", cmdID, status)
		}
	}
	out.WriteString("</SyncBody></SyncML>")
	return out.String(), nil
}

func between(s, a, b string) string {
	i := strings.Index(s, a)
	if i < 0 {
		return ""
	}
	s = s[i+len(a):]
	j := strings.Index(s, b)
	if j < 0 {
		return ""
	}
	return s[:j]
}

func TestSyncBodyRendering(t *testing.T) {
	body := SyncBody(Command{Verb: VerbReplace, Setting: Setting{LocURI: "./Device/Vendor/MSFT/Policy/Config/Storage/RemovableDiskDenyWriteAccess", Format: FormatInt, Value: "1"}})
	want := `<SyncBody><Replace><CmdID>1</CmdID><Item><Target><LocURI>./Device/Vendor/MSFT/Policy/Config/Storage/RemovableDiskDenyWriteAccess</LocURI></Target><Meta><Format xmlns="syncml:metinf">int</Format></Meta><Data>1</Data></Item></Replace></SyncBody>`
	if body != want {
		t.Fatalf("got %s", body)
	}
	get := SyncBody(Command{Verb: VerbGet, Setting: Setting{LocURI: "./DevDetail/SwV"}})
	if get != `<SyncBody><Get><CmdID>1</CmdID><Item><Target><LocURI>./DevDetail/SwV</LocURI></Target></Item></Get></SyncBody>` {
		t.Fatalf("got %s", get)
	}
	if !strings.Contains(SyncBody(Command{Verb: VerbReplace, Setting: Setting{LocURI: "x", Format: FormatChr, Value: "<enabled/>"}}), "&lt;enabled/&gt;") {
		t.Fatal("XML data must be escaped")
	}
}

func TestParseResponse(t *testing.T) {
	r, err := ParseResponse(`<SyncML xmlns="SYNCML:SYNCML1.2"><SyncBody><Status><CmdRef>1</CmdRef><Data>200</Data></Status><Results><CmdRef>1</CmdRef><Item><Source><LocURI>./DevDetail/SwV</LocURI></Source><Data>10.0.26100</Data></Item></Results></SyncBody></SyncML>`)
	if err != nil {
		t.Fatal(err)
	}
	if r.Results["./DevDetail/SwV"] != "10.0.26100" || !r.OK() {
		t.Fatalf("%+v", r)
	}
	if _, err := ParseResponse("not xml"); err == nil {
		t.Fatal("expected parse error")
	}
}

func TestCSPAdapterConvergesAndReverts(t *testing.T) {
	mdm := &fakeMDM{nodes: map[string]string{
		policyRoot + "Storage/RemovableDiskDenyWriteAccess": "0",
	}}
	a := NewCSPAdapter("windows.csp.removable", "storage.removable.block", mdm, RemovableStorageSettings)
	ctx := context.Background()
	tgt := native.Target{HostID: 1}
	params := map[string]any{"mode": "block"}

	prior, err := a.Check(ctx, tgt, params)
	if err != nil {
		t.Fatal(err)
	}
	if prior.IsSatisfied() {
		t.Fatal("should not be satisfied before apply")
	}
	res, err := a.Apply(ctx, tgt, params)
	if err != nil {
		t.Fatal(err)
	}
	if !res.Changed {
		t.Fatal("expected change")
	}
	// The ADMX node did not exist: Replace 404 must have been retried with Add.
	if mdm.nodes[policyRoot+"ADMX_RemovableStorage/RemovableDisks_DenyRead_Access_2"] == "" {
		t.Fatalf("missing node was not added: %v", mdm.nodes)
	}
	after, err := a.Check(ctx, tgt, params)
	if err != nil {
		t.Fatal(err)
	}
	if !after.IsSatisfied() {
		t.Fatalf("expected satisfied after apply: %v", after)
	}
	if _, err := a.Revert(ctx, tgt, params, prior); err != nil {
		t.Fatal(err)
	}
	if mdm.nodes[policyRoot+"Storage/RemovableDiskDenyWriteAccess"] != "0" {
		t.Fatalf("revert did not restore prior value: %v", mdm.nodes)
	}
}

func TestMappers(t *testing.T) {
	if _, err := RemovableStorageSettings(map[string]any{"mode": "sometimes"}); err == nil {
		t.Fatal("expected mode error")
	}
	s, err := ScreenLockSettings(map[string]any{"idle_minutes": float64(15)})
	if err != nil || len(s) != 2 || s[1].Value != "15" {
		t.Fatalf("%v %v", s, err)
	}
	if _, err := ScreenLockSettings(map[string]any{}); err == nil {
		t.Fatal("expected idle_minutes error")
	}
	u, err := URLBlocklistSettings(map[string]any{"domains": []any{"chat.openai.com", "gemini.google.com"}})
	if err != nil || !strings.Contains(u[0].Value, "chat.openai.com") || !strings.Contains(u[0].LocURI, "microsoft_edge~Policy~microsoft_edge/URLBlocklist") {
		t.Fatalf("%v %v", u, err)
	}
	fw, _ := FirewallSettings(nil)
	if len(fw) != 3 {
		t.Fatal("expected three firewall profiles")
	}
	reg := native.NewRegistry()
	reg.MustRegister(NewAdapters(&fakeMDM{nodes: map[string]string{}})...)
	if reg.Len() != 5 {
		t.Fatalf("registered %d", reg.Len())
	}
	lock, _ := reg.Lookup("host.lock", "windows")
	res, err := lock.Apply(context.Background(), native.Target{}, nil)
	if err != nil || !res.Changed || !strings.Contains(res.Native, "<Exec>") {
		t.Fatalf("exec adapter: %+v %v", res, err)
	}
}
