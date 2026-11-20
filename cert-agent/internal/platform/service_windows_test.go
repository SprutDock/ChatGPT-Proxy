package platform

import (
	"bytes"
	"strings"
	"testing"
	"unicode/utf16"
)

func TestTaskXMLBytesAreUTF16LEWithBOM(t *testing.T) {
	definition := `<Task><Path>C:\Users\Тест\Proxy</Path></Task>`
	encoded := taskXMLBytes(definition)
	if !bytes.HasPrefix(encoded, []byte{0xff, 0xfe}) {
		t.Fatal("task XML is missing the UTF-16LE byte-order mark")
	}
	units := make([]uint16, (len(encoded)-2)/2)
	for index := range units {
		units[index] = uint16(encoded[2+index*2]) | uint16(encoded[3+index*2])<<8
	}
	if decoded := string(utf16.Decode(units)); decoded != definition || !strings.Contains(decoded, "Тест") {
		t.Fatalf("decoded task XML = %q", decoded)
	}
}

func TestStunnelStorePathRejectsDifferentDrives(t *testing.T) {
	if _, err := StunnelStorePath(`C:\Program Files\ChatGPT Proxy`, `D:\Users\Test\state\client.pem`); err == nil {
		t.Fatal("expected a cross-drive certificate path to fail")
	}
}
