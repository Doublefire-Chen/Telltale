package main

import "testing"

func TestServiceSID(t *testing.T) {
	// TrustedInstaller's SID is well known; Windows derives it from the name.
	if got, want := serviceSID("TrustedInstaller"), "S-1-5-80-956008885-3418522649-1831038044-1853292631-2271478464"; got != want {
		t.Fatalf("serviceSID(TrustedInstaller) = %s, want %s", got, want)
	}
	if serviceSID("telltale-agent") != serviceSID("TELLTALE-AGENT") {
		t.Fatal("serviceSID depends on case; Windows ignores it")
	}
}
