package winresource

import (
	"bytes"
	"debug/pe"
	"encoding/binary"
	"strings"
	"testing"
)

func TestBuildObjectContainsIconGroupManifestAndRelocations(t *testing.T) {
	image := []byte("fake-png-image")
	var ico bytes.Buffer
	_ = binary.Write(&ico, binary.LittleEndian, uint16(0))
	_ = binary.Write(&ico, binary.LittleEndian, uint16(1))
	_ = binary.Write(&ico, binary.LittleEndian, uint16(1))
	ico.Write([]byte{16, 16, 0, 0})
	_ = binary.Write(&ico, binary.LittleEndian, uint16(1))
	_ = binary.Write(&ico, binary.LittleEndian, uint16(32))
	_ = binary.Write(&ico, binary.LittleEndian, uint32(len(image)))
	_ = binary.Write(&ico, binary.LittleEndian, uint32(22))
	ico.Write(image)
	manifest := []byte(`<assembly><requestedExecutionLevel level="asInvoker"/></assembly>`)

	object, err := BuildAMD64Object(ico.Bytes(), manifest)
	if err != nil {
		t.Fatal(err)
	}
	file, err := pe.NewFile(bytes.NewReader(object))
	if err != nil {
		t.Fatalf("parse object: %v", err)
	}
	defer file.Close()
	section := file.Section(".rsrc")
	if section == nil {
		t.Fatalf("resource section missing")
	}
	data, err := section.Data()
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(data, image) || !strings.Contains(string(data), "requestedExecutionLevel") {
		t.Fatalf("resource data missing icon or manifest")
	}
	if len(section.Relocs) != 3 {
		t.Fatalf("relocations=%d want 3", len(section.Relocs))
	}
}
