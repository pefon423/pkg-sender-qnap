package pkgmeta

import (
	"encoding/binary"
	"strings"
)

const (
	sfoMagic       = 0x46535000 // "\0PSF" read as little-endian
	sfoHeaderSize  = 20
	sfoIndexSize   = 16
	sfoMaxEntries  = 256
	sfoEntryParam  = 0x1000 // PARAM.SFO in the CNT entry table (PS4 packages)
	sfoFmtUTF8     = 0x0004
	sfoFmtUTF8Null = 0x0204
)

// parseSFO reads the string values of a PARAM.SFO. Integer fields are ignored.
func parseSFO(data []byte) (map[string]string, bool) {
	if len(data) < sfoHeaderSize || binary.LittleEndian.Uint32(data[0:4]) != sfoMagic {
		return nil, false
	}
	keyTable := int(binary.LittleEndian.Uint32(data[8:12]))
	dataTable := int(binary.LittleEndian.Uint32(data[12:16]))
	count := int(binary.LittleEndian.Uint32(data[16:20]))
	if count <= 0 || count > sfoMaxEntries || keyTable > len(data) || dataTable > len(data) ||
		sfoHeaderSize+count*sfoIndexSize > len(data) {
		return nil, false
	}

	values := make(map[string]string, count)
	for i := 0; i < count; i++ {
		idx := data[sfoHeaderSize+i*sfoIndexSize:]
		keyOff := int(binary.LittleEndian.Uint16(idx[0:2]))
		format := binary.LittleEndian.Uint16(idx[2:4])
		length := int(binary.LittleEndian.Uint32(idx[4:8]))
		dataOff := int(binary.LittleEndian.Uint32(idx[12:16]))
		if format != sfoFmtUTF8 && format != sfoFmtUTF8Null {
			continue
		}
		start := keyTable + keyOff
		if start >= len(data) {
			continue
		}
		end := start
		for end < len(data) && data[end] != 0 {
			end++
		}
		valStart := dataTable + dataOff
		valEnd := valStart + length
		if valStart < 0 || length < 0 || valEnd > len(data) {
			continue
		}
		values[string(data[start:end])] = strings.TrimSpace(strings.TrimRight(string(data[valStart:valEnd]), "\x00"))
	}
	return values, len(values) > 0
}

// applySFO fills metadata from a PS4 PARAM.SFO and returns the package type its
// CATEGORY implies ("" when it implies none).
func applySFO(meta *Metadata, sfo map[string]string) (packageType string) {
	if v := sfo["TITLE_ID"]; v != "" {
		meta.TitleID = v
	}
	if v := sfo["CONTENT_ID"]; v != "" {
		meta.ContentID = v
	}
	if v := sfo["TITLE"]; v != "" {
		meta.Title = v
	}
	// VERSION is the package's content version (the V0122 in file names).
	meta.Version = sfo["VERSION"]
	meta.MasterVersion = sfo["APP_VER"]
	if meta.TitleID == "" {
		meta.TitleID = titleIDFromContentID(meta.ContentID)
	}
	if strings.HasPrefix(strings.ToUpper(meta.TitleID), "CUSA") {
		meta.Platform = "PS4"
	}

	switch category := strings.ToLower(sfo["CATEGORY"]); {
	case strings.HasPrefix(category, "gd"):
		return "game"
	case category == "gp":
		return "patch"
	case category == "ac" || category == "ad":
		return "dlc"
	}
	return ""
}
