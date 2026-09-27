package postgres

import (
	"testing"

	"github.com/jackc/pgx/v5/pgconn"
)

func TestDuplicateVariantSKURecognizesOnlyVendorSKUConstraint(t *testing.T) {
	tests := []struct {
		name string
		err  *pgconn.PgError
		want bool
	}{
		{
			name: "vendor sku collision",
			err:  &pgconn.PgError{Code: "23505", ConstraintName: "product_variants_org_sku_key"},
			want: true,
		},
		{
			name: "different unique constraint",
			err:  &pgconn.PgError{Code: "23505", ConstraintName: "product_variants_public_id_key"},
		},
		{
			name: "other database error",
			err:  &pgconn.PgError{Code: "23503", ConstraintName: "product_variants_product_id_fkey"},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := duplicateVariantSKU(test.err); got != test.want {
				t.Errorf("duplicateVariantSKU() = %t, want %t", got, test.want)
			}
		})
	}
}
