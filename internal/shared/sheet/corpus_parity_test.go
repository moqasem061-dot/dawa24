package sheet_test

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/moqasem061-dot/dawa24/internal/shared/sheet"
)

// corpusParityBaseline records, for every real distributor sample in
// test/corpus/files, how many rows and what content ReadRows produced before
// the excelize upgrade (v2.11.0 to v2.11.1). A changed hash means the parser
// now reads the same file differently, which must be reviewed before shipping.
var corpusParityBaseline = map[string]corpusParityEntry{
	"admin-198.xlsx":     {Rows: 19997, Sum: "1f9341145f2da86509a2c8fde4519c0776f890a06eedd41c4f0b2c01411222f4"},
	"admin-203.xlsx":     {Rows: 9021, Sum: "8c87ee1ab62b825a2d7121b87b293db3d3c043e3f11444430368bdca7237d6b1"},
	"admin-208.csv":      {Rows: 4, Sum: "607b4a04dc3a386104b166907ce024e4554ccf6617889edb2b2954074f19a90e"},
	"admin-209.csv":      {Rows: 4, Sum: "3e82f2a59b129e8b3202192e31e06edec122e7fbab881d20cffdb7fc03c83645"},
	"admin-211.csv":      {Rows: 2, Sum: "375e228da5b2acde9ca9baf52130e74e94ffc0d2bf89982ba7dc071d44b1d00a"},
	"admin-212.csv":      {Rows: 2, Sum: "6c22d8073a5733369ab7e406bceeee2981dad1c0e96078c5bc6b5710a8193851"},
	"admin-213.csv":      {Rows: 3, Sum: "b4a96136553069e8123afca6633caf880a71cd62d48a3e6b9a1650e8bce973b9"},
	"admin-214.csv":      {Rows: 4, Sum: "9d58c2efc89ca2eeca7f876e9af8cd9c64cfe2961688722f97688ea8c4aac2d8"},
	"admin-215.csv":      {Rows: 4, Sum: "c3df366be173eafd848b47897e7ad252f0840e6f3112f8cdbc31880811bb3f19"},
	"admin-216.csv":      {Rows: 4, Sum: "add4db290c34d2d4b1a31754cd4e5742c9fe3cec9a44af3661ad352ba82a2ffb"},
	"admin-217.csv":      {Rows: 2, Sum: "0d5bc2c2e61cf263eddb9c272252cde60e44e2861fcea797cbe5fb60d20e1a22"},
	"admin-218.csv":      {Rows: 36, Sum: "5d16bd319c39de0f5fa3edac891a59daeafb24e46ac6cb127ff64d00a2108367"},
	"admin-252.csv":      {Rows: 4, Sum: "6801fe256480f8570b16a1e16a9d6b2c3299760777776745412aba5c243e14d0"},
	"admin-253.csv":      {Rows: 4, Sum: "4c66796489b56f0b4415524ac2197596dac7fcd4cd74a68fb6939e759ecd880b"},
	"admin-254.csv":      {Rows: 4, Sum: "293f5fe9b0f1e08fb630ef6ce85d4ded78c0d8a724010eb4403a4dc6d796e071"},
	"admin-255.csv":      {Rows: 4, Sum: "b97d33e9ea5e45675155961dcf46d3236389f3901deba11ed6e386d32f61cc40"},
	"admin-274.csv":      {Rows: 7, Sum: "620d2ba0a13493f040e32dda386b350b81305f0f26750370ee0e5ec068b84ce5"},
	"smartorder-14.xlsx": {Rows: 1644, Sum: "e5018d6ba8235c906eba177f6bf8cb0b5e2d24032da3fa81dd1ad4b7fa4d6f00"},
	"vendor-30.xlsx":     {Rows: 474, Sum: "dfc742aef55b6d0e9aea316212a8a676e96219607751e7c2401366522cadb4b4"},
	"vendor-31.xlsx":     {Rows: 1196, Sum: "663404faabde36c6a844286a04c2db51c6576df4633334c4fc55c88bf36d7631"},
	"vendor-35.xlsx":     {Rows: 364, Sum: "62c71fb672abf0a0ee5febbddf34fa71d52888b11c028af89ad7f49c60ef2fc9"},
	"vendor-37.xls":      {Rows: 1012, Sum: "f0d748907ff8bf6c358b4d1e027fc73daf518a5a597fd882685d7996874aa2bd"},
	"vendor-38.xlsx":     {Rows: 387, Sum: "6309b313077aa17760c9ae384f7d2fe595dc943c7aa9fd14215f219a10d1837b"},
	"vendor-40.xls":      {Rows: 1135, Sum: "d5f66b668e94d35d274eee75a318d962d789939f440450dccf89a73bdd1b63d2"},
}

type corpusParityEntry struct {
	Rows int
	Sum  string
}

func TestCorpusFilesParseAsBefore(t *testing.T) {
	dir := filepath.Join("..", "..", "..", "test", "corpus", "files")
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Skipf("corpus files not present: %v", err)
	}
	got := map[string]corpusParityEntry{}
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := e.Name()
		ext := strings.ToLower(filepath.Ext(name))
		if ext != ".xlsx" && ext != ".xls" && ext != ".csv" {
			continue
		}
		content, err := os.ReadFile(filepath.Join(dir, name))
		if err != nil {
			t.Fatalf("read %s: %v", name, err)
		}
		rows, err := sheet.ReadRows(content, name)
		if err != nil {
			t.Fatalf("ReadRows(%s): %v", name, err)
		}
		payload, err := json.Marshal(rows)
		if err != nil {
			t.Fatalf("marshal %s: %v", name, err)
		}
		sum := sha256.Sum256(payload)
		got[name] = corpusParityEntry{Rows: len(rows), Sum: hex.EncodeToString(sum[:])}
	}
	if out := os.Getenv("CORPUS_PARITY_PRINT"); out != "" {
		data, _ := json.MarshalIndent(got, "", "  ")
		if err := os.WriteFile(out, data, 0o600); err != nil {
			t.Fatalf("write %s: %v", out, err)
		}
		return
	}
	if len(corpusParityBaseline) == 0 {
		t.Fatal("corpusParityBaseline is empty; record it from the pre-upgrade tree")
	}
	for name, want := range corpusParityBaseline {
		have, ok := got[name]
		if !ok {
			t.Errorf("%s missing from corpus", name)
			continue
		}
		if have != want {
			t.Errorf("%s: parsed rows=%d sum=%s, want rows=%d sum=%s", name, have.Rows, have.Sum, want.Rows, want.Sum)
		}
	}
	if len(got) != len(corpusParityBaseline) {
		t.Errorf("corpus has %d files, baseline has %d", len(got), len(corpusParityBaseline))
	}
}
