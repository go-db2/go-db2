package auth

import "testing"

func FuzzParseKeytab(f *testing.F) {
	validKT, err := BuildKeytab(
		KeytabEntry{
			Principal: "db2/db2server.corp.local@CORP.LOCAL",
			KeyType:   18,
			KVNO:      1,
			Key:       []byte("0123456789abcdef0123456789abcdef"),
		},
	)
	if err != nil {
		f.Fatalf("BuildKeytab seed failed: %v", err)
	}
	f.Add(validKT)
	// Keytab v2 header + MinInt32 entry length
	f.Add([]byte{0x05, 0x02, 0x80, 0x00, 0x00, 0x00})
	// Excessive numComponents
	f.Add([]byte{0x05, 0x02, 0x00, 0x00, 0x00, 0x08, 0xFF, 0xFF, 0x00, 0x01, 'A', 0x00, 0x00, 0x00})
	// Deleted negative entry length (-4)
	f.Add([]byte{0x05, 0x02, 0xFF, 0xFF, 0xFF, 0xFC, 0x01, 0x02, 0x03, 0x04})
	// Wrong version / empty
	f.Add([]byte{0x05, 0x01})
	f.Add([]byte{})

	f.Fuzz(func(t *testing.T, data []byte) {
		_, _ = ParseKeytab(data)
	})
}
