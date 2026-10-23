package productmatch

import "testing"

func TestSalePriceConvertedBeforeImport(t *testing.T) {
	mapping := &Mapping{ByField: map[Field]int{FieldName: 0, FieldPublicPrice: 1, FieldNetPrice: 2}}
	reader := NewReader(mapping, DefaultParseOptions())
	for _, tc := range []struct {
		public, sale string
		want         int64
		valid        bool
	}{
		{"100", "80", 2000, true}, {"100", "100", 0, true}, {"100", "0", 10000, true},
		{"3", "2", 3333, true}, {"١٠٠", "٨٠", 2000, true},
		{"100", "101", 0, false}, {"100", "-1", 0, false}, {"0", "0", 0, false},
		{"100", "", 0, false}, {"100", "invalid", 0, false}, {"100", "20%", 0, false},
		{"invalid", "80", 0, false}, {"0", "80", 0, false}, {"100%", "80", 0, false},
	} {
		t.Run(tc.public+"/"+tc.sale, func(t *testing.T) {
			row, valid := reader.Read(2, []string{"cream", tc.public, tc.sale})
			if valid != tc.valid || valid && row.DiscountBps != tc.want {
				t.Fatalf("valid=%v discount=%d issues=%+v", valid, row.DiscountBps, row.Issues)
			}
		})
	}
}
