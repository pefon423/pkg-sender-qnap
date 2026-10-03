package pkgmeta

import (
	"encoding/binary"
	"encoding/json"
	"testing"
)

// buildSFO assembles a minimal PARAM.SFO holding the given string values.
func buildSFO(pairs [][2]string) []byte {
	var keys, vals []byte
	index := make([]byte, 0, len(pairs)*sfoIndexSize)
	for _, p := range pairs {
		entry := make([]byte, sfoIndexSize)
		binary.LittleEndian.PutUint16(entry[0:2], uint16(len(keys)))
		binary.LittleEndian.PutUint16(entry[2:4], sfoFmtUTF8Null)
		binary.LittleEndian.PutUint32(entry[4:8], uint32(len(p[1])+1))
		binary.LittleEndian.PutUint32(entry[8:12], uint32(len(p[1])+1))
		binary.LittleEndian.PutUint32(entry[12:16], uint32(len(vals)))
		index = append(index, entry...)
		keys = append(append(keys, p[0]...), 0)
		vals = append(append(vals, p[1]...), 0)
	}
	keyTable := sfoHeaderSize + len(index)
	header := make([]byte, sfoHeaderSize)
	binary.LittleEndian.PutUint32(header[0:4], sfoMagic)
	binary.LittleEndian.PutUint32(header[4:8], 0x101)
	binary.LittleEndian.PutUint32(header[8:12], uint32(keyTable))
	binary.LittleEndian.PutUint32(header[12:16], uint32(keyTable+len(keys)))
	binary.LittleEndian.PutUint32(header[16:20], uint32(len(pairs)))
	return append(append(append(header, index...), keys...), vals...)
}

func TestApplySFO(t *testing.T) {
	sfo, ok := parseSFO(buildSFO([][2]string{
		{"CATEGORY", "gd"}, {"TITLE", "Like a Dragon Gaiden"}, {"TITLE_ID", "CUSA43228"},
		{"CONTENT_ID", "JP0177-CUSA43228_00-LIKEADRGNGAIDEN0"}, {"APP_VER", "01.22"}, {"VERSION", "01.00"},
	}))
	if !ok {
		t.Fatal("parseSFO failed")
	}
	meta := Metadata{Platform: "PS5"}
	if got := applySFO(&meta, sfo); got != "game" {
		t.Fatalf("package type=%q, want game", got)
	}
	if meta.Title != "Like a Dragon Gaiden" || meta.TitleID != "CUSA43228" || meta.Version != "01.00" ||
		meta.MasterVersion != "01.22" || meta.Platform != "PS4" {
		t.Fatalf("unexpected metadata: %+v", meta)
	}

	for category, want := range map[string]string{"gp": "patch", "ac": "dlc", "gdd": "game", "xx": ""} {
		sfo, _ := parseSFO(buildSFO([][2]string{{"CATEGORY", category}, {"TITLE_ID", "CUSA00001"}}))
		if got := applySFO(&Metadata{}, sfo); got != want {
			t.Fatalf("category %q -> %q, want %q", category, got, want)
		}
	}
	if _, ok := parseSFO([]byte("not an sfo at all, just text")); ok {
		t.Fatal("garbage must not parse")
	}
}

func TestReadPrefersSFOWhenJSONHasNoTitle(t *testing.T) {
	// An entry table where 0x2000 holds a JSON without any title and 0x1000 holds the real PARAM.SFO.
	sfo := buildSFO([][2]string{{"CATEGORY", "gp"}, {"TITLE", "Some Patch"}, {"TITLE_ID", "CUSA44357"}, {"VERSION", "01.05"}})
	meta := Metadata{Platform: "PS5"}
	var param map[string]json.RawMessage
	if err := json.Unmarshal([]byte(`{"pieceHashes":[1,2,3]}`), &param); err != nil {
		t.Fatal(err)
	}
	applyParamJSON(&meta, param)
	if meta.Title != "" {
		t.Fatalf("a JSON without localizedParameters must not give a title, got %q", meta.Title)
	}
	parsed, ok := parseSFO(sfo)
	if !ok {
		t.Fatal("parseSFO failed")
	}
	if got := applySFO(&meta, parsed); got != "patch" || meta.Title != "Some Patch" || meta.Version != "01.05" {
		t.Fatalf("type=%q meta=%+v", got, meta)
	}
}
