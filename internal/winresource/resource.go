package winresource

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"sort"
)

const (
	machineAMD64                  = 0x8664
	imageRelAMD64Addr32NB         = 0x0003
	sectionInitializedRead        = 0x40000040
	resourceIcon           uint32 = 3
	resourceGroupIcon      uint32 = 14
	resourceManifest       uint32 = 24
	resourceSubdirectory          = 0x80000000
	resourceLanguageUS            = 0x0409
)

type iconEntry struct {
	width, height, colorCount, reserved byte
	planes, bitCount                    uint16
	size, offset                        uint32
	data                                []byte
}

type resource struct {
	kind uint32
	id   uint16
	data []byte

	idDirectoryOffset uint32
	dataEntryOffset   uint32
	dataOffset        uint32
}

// BuildAMD64Object creates an AMD64 COFF object containing an application icon
// and optional Windows manifest. The resulting bytes can be saved as a .syso
// file beside a Go package before a Windows build.
func BuildAMD64Object(icoData, manifest []byte) ([]byte, error) {
	icons, err := parseICO(icoData)
	if err != nil {
		return nil, err
	}
	resources := make([]*resource, 0, len(icons)+2)
	for index, icon := range icons {
		resources = append(resources, &resource{kind: resourceIcon, id: uint16(index + 1), data: icon.data})
	}
	resources = append(resources, &resource{kind: resourceGroupIcon, id: 1, data: buildIconGroup(icons)})
	if len(manifest) > 0 {
		resources = append(resources, &resource{kind: resourceManifest, id: 1, data: append([]byte(nil), manifest...)})
	}
	section, relocations, err := buildResourceSection(resources)
	if err != nil {
		return nil, err
	}
	return buildCOFFObject(section, relocations), nil
}

func parseICO(data []byte) ([]iconEntry, error) {
	if len(data) < 6 {
		return nil, fmt.Errorf("icon file is truncated")
	}
	reserved := binary.LittleEndian.Uint16(data[0:2])
	kind := binary.LittleEndian.Uint16(data[2:4])
	count := int(binary.LittleEndian.Uint16(data[4:6]))
	if reserved != 0 || kind != 1 || count < 1 {
		return nil, fmt.Errorf("invalid icon header")
	}
	if count > 256 || len(data) < 6+16*count {
		return nil, fmt.Errorf("invalid icon entry count")
	}
	result := make([]iconEntry, 0, count)
	for index := 0; index < count; index++ {
		offset := 6 + 16*index
		entry := iconEntry{
			width:      data[offset],
			height:     data[offset+1],
			colorCount: data[offset+2],
			reserved:   data[offset+3],
			planes:     binary.LittleEndian.Uint16(data[offset+4 : offset+6]),
			bitCount:   binary.LittleEndian.Uint16(data[offset+6 : offset+8]),
			size:       binary.LittleEndian.Uint32(data[offset+8 : offset+12]),
			offset:     binary.LittleEndian.Uint32(data[offset+12 : offset+16]),
		}
		end := uint64(entry.offset) + uint64(entry.size)
		if entry.size == 0 || end > uint64(len(data)) {
			return nil, fmt.Errorf("icon image %d is outside the file", index+1)
		}
		entry.data = append([]byte(nil), data[entry.offset:uint32(end)]...)
		result = append(result, entry)
	}
	return result, nil
}

func buildIconGroup(icons []iconEntry) []byte {
	var buffer bytes.Buffer
	_ = binary.Write(&buffer, binary.LittleEndian, uint16(0))
	_ = binary.Write(&buffer, binary.LittleEndian, uint16(1))
	_ = binary.Write(&buffer, binary.LittleEndian, uint16(len(icons)))
	for index, icon := range icons {
		buffer.WriteByte(icon.width)
		buffer.WriteByte(icon.height)
		buffer.WriteByte(icon.colorCount)
		buffer.WriteByte(icon.reserved)
		_ = binary.Write(&buffer, binary.LittleEndian, icon.planes)
		_ = binary.Write(&buffer, binary.LittleEndian, icon.bitCount)
		_ = binary.Write(&buffer, binary.LittleEndian, uint32(len(icon.data)))
		_ = binary.Write(&buffer, binary.LittleEndian, uint16(index+1))
	}
	return buffer.Bytes()
}

func buildResourceSection(resources []*resource) ([]byte, []uint32, error) {
	byType := map[uint32][]*resource{}
	for _, item := range resources {
		byType[item.kind] = append(byType[item.kind], item)
	}
	types := make([]uint32, 0, len(byType))
	for kind := range byType {
		types = append(types, kind)
		sort.Slice(byType[kind], func(i, j int) bool { return byType[kind][i].id < byType[kind][j].id })
	}
	sort.Slice(types, func(i, j int) bool { return types[i] < types[j] })

	rootSize := uint32(16 + 8*len(types))
	cursor := rootSize
	typeOffsets := map[uint32]uint32{}
	for _, kind := range types {
		typeOffsets[kind] = cursor
		cursor += uint32(16 + 8*len(byType[kind]))
	}
	for _, kind := range types {
		for _, item := range byType[kind] {
			item.idDirectoryOffset = cursor
			cursor += 24 // directory header plus one language entry
		}
	}
	cursor = align(cursor, 4)
	for _, kind := range types {
		for _, item := range byType[kind] {
			item.dataEntryOffset = cursor
			cursor += 16
		}
	}
	cursor = align(cursor, 8)
	for _, kind := range types {
		for _, item := range byType[kind] {
			item.dataOffset = cursor
			cursor += uint32(len(item.data))
			cursor = align(cursor, 8)
		}
	}
	section := make([]byte, cursor)
	writeDirectoryHeader(section, 0, uint16(len(types)))
	for index, kind := range types {
		writeDirectoryEntry(section, rootSize-uint32(8*len(types))+uint32(index*8), kind, resourceSubdirectory|typeOffsets[kind])
	}
	for _, kind := range types {
		offset := typeOffsets[kind]
		items := byType[kind]
		writeDirectoryHeader(section, offset, uint16(len(items)))
		for index, item := range items {
			writeDirectoryEntry(section, offset+16+uint32(index*8), uint32(item.id), resourceSubdirectory|item.idDirectoryOffset)
		}
	}
	relocations := make([]uint32, 0, len(resources))
	for _, kind := range types {
		for _, item := range byType[kind] {
			writeDirectoryHeader(section, item.idDirectoryOffset, 1)
			writeDirectoryEntry(section, item.idDirectoryOffset+16, resourceLanguageUS, item.dataEntryOffset)
			binary.LittleEndian.PutUint32(section[item.dataEntryOffset:item.dataEntryOffset+4], item.dataOffset)
			binary.LittleEndian.PutUint32(section[item.dataEntryOffset+4:item.dataEntryOffset+8], uint32(len(item.data)))
			relocations = append(relocations, item.dataEntryOffset)
			copy(section[item.dataOffset:], item.data)
		}
	}
	return section, relocations, nil
}

func writeDirectoryHeader(section []byte, offset uint32, idEntries uint16) {
	binary.LittleEndian.PutUint16(section[offset+12:offset+14], 0)
	binary.LittleEndian.PutUint16(section[offset+14:offset+16], idEntries)
}

func writeDirectoryEntry(section []byte, offset, id, target uint32) {
	binary.LittleEndian.PutUint32(section[offset:offset+4], id)
	binary.LittleEndian.PutUint32(section[offset+4:offset+8], target)
}

func buildCOFFObject(section []byte, relocationOffsets []uint32) []byte {
	const headerSize = uint32(20 + 40)
	relocationPointer := headerSize + uint32(len(section))
	symbolPointer := relocationPointer + uint32(len(relocationOffsets))*10
	total := symbolPointer + 18 + 4
	object := make([]byte, total)

	// IMAGE_FILE_HEADER.
	binary.LittleEndian.PutUint16(object[0:2], machineAMD64)
	binary.LittleEndian.PutUint16(object[2:4], 1)
	binary.LittleEndian.PutUint32(object[8:12], symbolPointer)
	binary.LittleEndian.PutUint32(object[12:16], 1)
	binary.LittleEndian.PutUint16(object[18:20], 0x0004) // line numbers stripped

	// IMAGE_SECTION_HEADER.
	copy(object[20:28], []byte(".rsrc"))
	binary.LittleEndian.PutUint32(object[36:40], uint32(len(section)))
	binary.LittleEndian.PutUint32(object[40:44], headerSize)
	binary.LittleEndian.PutUint32(object[44:48], relocationPointer)
	binary.LittleEndian.PutUint16(object[52:54], uint16(len(relocationOffsets)))
	binary.LittleEndian.PutUint32(object[56:60], sectionInitializedRead)
	copy(object[headerSize:headerSize+uint32(len(section))], section)

	cursor := relocationPointer
	for _, offset := range relocationOffsets {
		binary.LittleEndian.PutUint32(object[cursor:cursor+4], offset)
		binary.LittleEndian.PutUint32(object[cursor+4:cursor+8], 0)
		binary.LittleEndian.PutUint16(object[cursor+8:cursor+10], imageRelAMD64Addr32NB)
		cursor += 10
	}
	copy(object[symbolPointer:symbolPointer+8], []byte(".rsrc"))
	binary.LittleEndian.PutUint16(object[symbolPointer+12:symbolPointer+14], 1)
	object[symbolPointer+16] = 3 // static symbol
	binary.LittleEndian.PutUint32(object[symbolPointer+18:symbolPointer+22], 4)
	return object
}

func align(value, alignment uint32) uint32 {
	return (value + alignment - 1) &^ (alignment - 1)
}
