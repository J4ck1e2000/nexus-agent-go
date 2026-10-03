package sshcollector

import "testing"

func TestParseNetDev_SumsInterfacesExcludingLo(t *testing.T) {
	snapshot := parseNetDev(loadFixtureLines(t, "netdev.txt"))

	// eth0 RX 53687091200 + eth1 RX 1048576 = 53688139776。
	if snapshot.RXBytes != 53688139776 {
		t.Fatalf("rx bytes = %d, want 53688139776", snapshot.RXBytes)
	}
	// eth0 TX 32212254720 + eth1 TX 2097152 = 32214351872。
	if snapshot.TXBytes != 32214351872 {
		t.Fatalf("tx bytes = %d, want 32214351872", snapshot.TXBytes)
	}
}

func TestParseNetDev_SkipsLoopback(t *testing.T) {
	lines := []string{
		"Inter-|   Receive  |  Transmit",
		"    lo: 999999999 1000 0 0 0 0 0 0 999999999 1000 0 0 0 0 0 0",
		"  eth0: 1000 10 0 0 0 0 0 0 2000 20 0 0 0 0 0 0",
	}
	snapshot := parseNetDev(lines)
	if snapshot.RXBytes != 1000 || snapshot.TXBytes != 2000 {
		t.Fatalf("loopback should be excluded: %+v", snapshot)
	}
}

func TestParseNetDev_EmptyAndMalformed(t *testing.T) {
	snapshot := parseNetDev(nil)
	if snapshot.RXBytes != 0 || snapshot.TXBytes != 0 {
		t.Fatalf("empty input should yield zero: %+v", snapshot)
	}

	snapshot = parseNetDev([]string{"garbage-without-colon", "eth9: not numbers"})
	if snapshot.RXBytes != 0 || snapshot.TXBytes != 0 {
		t.Fatalf("malformed input should yield zero: %+v", snapshot)
	}
}

func TestParseNetDev_HandlesIPv6AliasColon(t *testing.T) {
	lines := []string{
		"  eth0:0: 1000 10 0 0 0 0 0 0 2000 20 0 0 0 0 0 0",
	}
	snapshot := parseNetDev(lines)
	if snapshot.RXBytes != 1000 || snapshot.TXBytes != 2000 {
		t.Fatalf("alias interface should parse: %+v", snapshot)
	}
}
