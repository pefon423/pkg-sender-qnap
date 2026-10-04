package pkgmeta

import (
	"encoding/binary"
	"strconv"
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

// compareVersions compares dotted numeric versions such as "01.05" and
// "01.009.001"; it returns <0, 0 or >0. Non-numeric parts compare as text.
func compareVersions(a, b string) int {
	pa, pb := strings.Split(a, "."), strings.Split(b, ".")
	for i := 0; i < len(pa) || i < len(pb); i++ {
		var x, y string
		if i < len(pa) {
			x = pa[i]
		}
		if i < len(pb) {
			y = pb[i]
		}
		nx, errX := strconv.Atoi(x)
		ny, errY := strconv.Atoi(y)
		switch {
		case errX == nil && errY == nil && nx != ny:
			if nx < ny {
				return -1
			}
			return 1
		case (errX != nil || errY != nil) && x != y:
			return strings.Compare(x, y)
		}
	}
	return 0
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
	// A base game carries its real version in VERSION (the V0122 in file names)
	// with APP_VER left at 01.00, while a patch is the other way round
	// (APP_VER 01.05, VERSION 01.00). Whichever is higher is the real version.
	meta.Version, meta.MasterVersion = sfo["VERSION"], sfo["APP_VER"]
	if compareVersions(meta.Version, meta.MasterVersion) < 0 {
		meta.Version, meta.MasterVersion = meta.MasterVersion, meta.Version
	}
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
