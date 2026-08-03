package pathenv

import (
	"encoding/base64"
	"encoding/binary"
	"fmt"
	"strings"
	"unicode/utf16"
)

// MaxWindowsStringTransportBytes bounds captured Base64 UTF-16 output while
// covering the maximum practical Windows environment variable value.
const MaxWindowsStringTransportBytes = 128 << 10

// EncodeWindowsString transports a Windows UTF-16 string through an
// ASCII-only process boundary without depending on the active console code
// page. PowerShell's [Text.Encoding]::Unicode uses UTF-16 little endian.
func EncodeWindowsString(value string) string {
	units := utf16.Encode([]rune(value))
	data := make([]byte, len(units)*2)
	for index, unit := range units {
		binary.LittleEndian.PutUint16(data[index*2:], unit)
	}
	return base64.StdEncoding.EncodeToString(data)
}

// DecodeWindowsString reverses EncodeWindowsString and rejects malformed
// Base64, odd byte counts, and unpaired UTF-16 surrogates.
func DecodeWindowsString(value string) (string, error) {
	data, err := base64.StdEncoding.DecodeString(strings.TrimSpace(value))
	if err != nil {
		return "", fmt.Errorf("decode Windows string transport: %w", err)
	}
	if len(data)%2 != 0 {
		return "", fmt.Errorf("decode Windows string transport: odd UTF-16 byte count %d", len(data))
	}
	units := make([]uint16, len(data)/2)
	for index := range units {
		units[index] = binary.LittleEndian.Uint16(data[index*2:])
	}
	for index := 0; index < len(units); index++ {
		unit := units[index]
		switch {
		case unit >= 0xD800 && unit <= 0xDBFF:
			if index+1 >= len(units) || units[index+1] < 0xDC00 || units[index+1] > 0xDFFF {
				return "", fmt.Errorf("decode Windows string transport: unpaired high surrogate at unit %d", index)
			}
			index++
		case unit >= 0xDC00 && unit <= 0xDFFF:
			return "", fmt.Errorf("decode Windows string transport: unpaired low surrogate at unit %d", index)
		}
	}
	return string(utf16.Decode(units)), nil
}
