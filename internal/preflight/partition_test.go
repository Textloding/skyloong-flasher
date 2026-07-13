package preflight

import (
	"encoding/binary"
	"testing"
)

type partitionFixture struct {
	Magic   uint16
	Type    uint8
	Subtype uint8
	Offset  uint32
	Size    uint32
	Label   string
	Flags   uint32
}

func partitionTable(fixtures ...partitionFixture) []byte {
	raw := make([]byte, 0, len(fixtures)*32)
	for _, fixture := range fixtures {
		entry := make([]byte, 32)
		magic := fixture.Magic
		if magic == 0 {
			magic = 0x50AA
		}
		binary.LittleEndian.PutUint16(entry[0:2], magic)
		entry[2] = fixture.Type
		entry[3] = fixture.Subtype
		binary.LittleEndian.PutUint32(entry[4:8], fixture.Offset)
		binary.LittleEndian.PutUint32(entry[8:12], fixture.Size)
		copy(entry[12:28], fixture.Label)
		binary.LittleEndian.PutUint32(entry[28:32], fixture.Flags)
		raw = append(raw, entry...)
	}
	return raw
}

func validPartitionFixtures() []partitionFixture {
	return []partitionFixture{
		{Type: 0x01, Subtype: 0x02, Offset: 0x10000, Size: 0x2000, Label: "otadata"},
		{Type: 0x00, Subtype: 0x10, Offset: 0x20000, Size: 0x500000, Label: "ota_0"},
		{Type: 0x00, Subtype: 0x11, Offset: 0x520000, Size: 0x500000, Label: "ota_1"},
		{Type: 0x01, Subtype: 0x82, Offset: 0xA20000, Size: 0x5D0000, Label: "spiffs"},
	}
}

func TestParsePartitionTableAndMinimumFlashSize(t *testing.T) {
	parts, err := ParsePartitionTable(partitionTable(validPartitionFixtures()...))
	if err != nil {
		t.Fatalf("ParsePartitionTable() error = %v", err)
	}
	if len(parts) != 4 {
		t.Fatalf("partition count = %d, want 4", len(parts))
	}
	if parts[0].Label != "otadata" || parts[1].Label != "ota_0" || parts[3].Label != "spiffs" {
		t.Fatalf("unexpected parsed partitions: %#v", parts)
	}
	if got := uint64(parts[3].Offset) + uint64(parts[3].Size); got != 0xFF0000 {
		t.Fatalf("highest partition end = %#x, want 0xFF0000", got)
	}
	if checks := ValidatePartitions(parts); len(checks) != 0 {
		t.Fatalf("ValidatePartitions() = %#v, want no checks", checks)
	}
	if got := MinimumFlashSize(parts); got != 16*1024*1024 {
		t.Fatalf("MinimumFlashSize() = %#x, want 16MB", got)
	}
}

func TestParsePartitionTableRejectsInvalidMagic(t *testing.T) {
	_, err := ParsePartitionTable(partitionTable(partitionFixture{
		Magic: 0x1234, Type: 0, Subtype: 0x10, Offset: 0x20000, Size: 0x100000, Label: "factory",
	}))
	if err == nil {
		t.Fatal("ParsePartitionTable() error = nil, want invalid magic error")
	}
}

func TestValidatePartitionsReportsOverlap(t *testing.T) {
	parts, err := ParsePartitionTable(partitionTable(
		partitionFixture{Type: 0, Subtype: 0x10, Offset: 0x20000, Size: 0x500000, Label: "ota_0"},
		partitionFixture{Type: 0, Subtype: 0x11, Offset: 0x500000, Size: 0x500000, Label: "ota_1"},
	))
	if err != nil {
		t.Fatalf("ParsePartitionTable() error = %v", err)
	}
	checks := ValidatePartitions(parts)
	if len(checks) != 1 || checks[0].Code != "partition_overlap" || checks[0].Status != StatusWarning {
		t.Fatalf("ValidatePartitions() = %#v, want partition_overlap warning", checks)
	}
}

func TestParsePartitionTableRejectsTruncatedEntry(t *testing.T) {
	raw := partitionTable(partitionFixture{Type: 0, Subtype: 0x10, Offset: 0x20000, Size: 0x100000, Label: "ota_0"})
	_, err := ParsePartitionTable(raw[:len(raw)-1])
	if err == nil {
		t.Fatal("ParsePartitionTable() error = nil, want truncated entry error")
	}
}

func TestParsePartitionTableStopsAtErasedOrMD5Entry(t *testing.T) {
	first := partitionTable(partitionFixture{Type: 0, Subtype: 0x10, Offset: 0x20000, Size: 0x100000})
	for _, marker := range []uint16{0xFFFF, 0xEBEB} {
		markerEntry := make([]byte, 32)
		binary.LittleEndian.PutUint16(markerEntry[:2], marker)
		raw := append(append([]byte{}, first...), markerEntry...)
		raw = append(raw, partitionTable(partitionFixture{Type: 1, Subtype: 0x82, Offset: 0x200000, Size: 0x100000, Label: "ignored"})...)

		parts, err := ParsePartitionTable(raw)
		if err != nil {
			t.Fatalf("marker %#x: ParsePartitionTable() error = %v", marker, err)
		}
		if len(parts) != 1 || parts[0].Label != "" {
			t.Fatalf("marker %#x: parsed partitions = %#v, want one empty-label partition", marker, parts)
		}
	}
}

func TestMinimumFlashSizeUsesSupportedCapacityTiers(t *testing.T) {
	tests := []struct {
		end  uint32
		want uint64
	}{
		{end: 1, want: 1 * 1024 * 1024},
		{end: 1*1024*1024 + 1, want: 2 * 1024 * 1024},
		{end: 2*1024*1024 + 1, want: 4 * 1024 * 1024},
		{end: 4*1024*1024 + 1, want: 8 * 1024 * 1024},
		{end: 8*1024*1024 + 1, want: 16 * 1024 * 1024},
		{end: 16*1024*1024 + 1, want: 32 * 1024 * 1024},
		{end: 32*1024*1024 + 1, want: 64 * 1024 * 1024},
		{end: 64*1024*1024 + 1, want: 128 * 1024 * 1024},
	}
	for _, test := range tests {
		parts := []Partition{{Offset: 0, Size: test.end}}
		if got := MinimumFlashSize(parts); got != test.want {
			t.Errorf("MinimumFlashSize(end=%#x) = %#x, want %#x", test.end, got, test.want)
		}
	}
}
