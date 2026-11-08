package erp

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/moqasem061-dot/dawa24/internal/modules/erp/provider"
	"github.com/moqasem061-dot/dawa24/internal/shared/money"
)

// Item is one normalized ERP product. Every money field went through
// money.Parse (two decimals, no floats — trap 13); Stock is the whole-number
// quantity for the key's warehouse.
type Item struct {
	Code         string
	ArName       string
	Barcode      string
	Unit         string
	Manufacturer string
	Batch        string
	PublicPrice  money.Amount
	Discount     *money.Amount
	Tax          *money.Amount
	Stock        int
	Expiry       *time.Time
	IsQuota      *bool
	QuotaQty     *int
}

// ItemError carries one item-level refusal (the "Product sync (item level)"
// rows of contracts/error-codes.md). Code is the stable error code; the
// reader-facing text comes from the i18n catalog, here and in the handler.
type ItemError struct {
	Code    string
	Details map[string]any
}

func (e *ItemError) Error() string { return e.Code }

func itemError(code string) *ItemError { return &ItemError{Code: code} }

var expiryLayouts = []string{"2006-01-02", "2006-01-02T15:04:05", time.RFC3339}

// NormalizeItem validates and converts one raw wire item. Every refusal is one
// of the item-level codes; nothing is rounded and nothing defaults silently.
func NormalizeItem(raw provider.RawItem) (Item, *ItemError) {
	if raw.Code == "" {
		return Item{}, itemError("ERP_ITEM_CODE_REQUIRED")
	}
	if raw.ArName == "" {
		return Item{}, itemError("ERP_ITEM_NAME_REQUIRED")
	}

	item := Item{Code: raw.Code, ArName: raw.ArName, Barcode: raw.Barcode, Unit: raw.Unit, Manufacturer: raw.Manufacturer, Batch: raw.Batch}

	if raw.PublicPrice == nil {
		return Item{}, itemError("ERP_ITEM_PRICE_INVALID")
	}
	price, err := money.Parse(raw.PublicPrice.String())
	if err != nil {
		return Item{}, itemError(precisionCode(err, "ERP_ITEM_PRICE_INVALID", "ERP_ITEM_PRICE_PRECISION"))
	}
	if price.IsNegative() {
		return Item{}, itemError("ERP_ITEM_PRICE_INVALID")
	}
	item.PublicPrice = price

	if raw.Discount != nil {
		discount, err := percent(raw.Discount.String())
		if err != nil {
			if strings.Contains(err.Error(), "decimal places") {
				return Item{}, itemError("ERP_ITEM_DISCOUNT_INVALID")
			}
			return Item{}, itemError("ERP_ITEM_DISCOUNT_INVALID")
		}
		item.Discount = &discount
	}
	if raw.Tax != nil {
		tax, err := percent(raw.Tax.String())
		if err != nil {
			return Item{}, itemError("ERP_ITEM_TAX_INVALID")
		}
		item.Tax = &tax
	}

	if raw.Stock == nil {
		return Item{}, itemError("ERP_ITEM_STOCK_INVALID")
	}
	stock, err := strconv.ParseInt(raw.Stock.String(), 10, 64)
	if err != nil {
		return Item{}, itemError("ERP_ITEM_STOCK_INVALID")
	}
	if stock < 0 {
		return Item{}, itemError("ERP_ITEM_STOCK_NEGATIVE")
	}
	item.Stock = int(stock)

	switch {
	case raw.IsQuota != nil && *raw.IsQuota:
		if raw.QuotaQty == nil {
			return Item{}, itemError("ERP_ITEM_QUOTA_INVALID")
		}
		qty, err := strconv.ParseInt(raw.QuotaQty.String(), 10, 64)
		if err != nil || qty <= 0 {
			return Item{}, itemError("ERP_ITEM_QUOTA_INVALID")
		}
		yes := true
		quota := int(qty)
		item.IsQuota = &yes
		item.QuotaQty = &quota
	case raw.IsQuota != nil:
		no := false
		item.IsQuota = &no
		// quotaQuantity 0 with isQuota false is the common shape and means no
		// quota at all: the field is dropped rather than stored.
	}

	if raw.Barcode != "" {
		if len(raw.Barcode) > 32 {
			return Item{}, itemError("ERP_ITEM_BARCODE_INVALID")
		}
		for _, r := range raw.Barcode {
			if r < '0' || r > '9' {
				return Item{}, itemError("ERP_ITEM_BARCODE_INVALID")
			}
		}
	}

	if raw.Expiry != "" {
		for _, layout := range expiryLayouts {
			if parsed, err := time.Parse(layout, raw.Expiry); err == nil {
				date := time.Date(parsed.Year(), parsed.Month(), parsed.Day(), 0, 0, 0, 0, time.UTC)
				item.Expiry = &date
				break
			}
		}
		if item.Expiry == nil {
			return Item{}, itemError("ERP_ITEM_EXPIRY_INVALID")
		}
	}
	return item, nil
}

// percent parses a 0..100 percentage with at most two decimals. The percentages
// are stored (those of the sample), never applied.
func percent(text string) (money.Amount, error) {
	value, err := money.Parse(text)
	if err != nil {
		return money.Zero, err
	}
	if value.IsNegative() || value.Minor() > money.FromMajor(100).Minor() {
		return money.Zero, fmt.Errorf("percentage out of range")
	}
	return value, nil
}

// precisionCode maps a money.Parse failure onto the contract's split: too many
// decimals is its own code (the ERP must resend, we will not round); anything
// else is a plain invalid value.
func precisionCode(err error, invalid, precision string) string {
	if errors.Is(err, money.ErrInvalidFormat) && strings.Contains(err.Error(), "decimal places") {
		return precision
	}
	return invalid
}

// Hash fingerprints the item for the link's last_hash: identical item on the
// same warehouse and branch produces the same hash, any change in stock, price
// or location changes it. The text form is fixed-order and newline-free.
func (i Item) Hash(warehouseID, branchID int64) string {
	sum := sha256.Sum256([]byte(strings.Join([]string{
		"v1",
		i.Code,
		i.ArName,
		i.Barcode,
		i.Unit,
		i.Manufacturer,
		i.Batch,
		i.PublicPrice.String(),
		amountPtr(i.Discount),
		amountPtr(i.Tax),
		strconv.FormatInt(int64(i.Stock), 10),
		timePtr(i.Expiry),
		boolPtr(i.IsQuota),
		intPtr(i.QuotaQty),
		strconv.FormatInt(warehouseID, 10),
		strconv.FormatInt(branchID, 10),
	}, "\x1f")))
	return hex.EncodeToString(sum[:])
}

func amountPtr(a *money.Amount) string {
	if a == nil {
		return "-"
	}
	return a.String()
}

func timePtr(t *time.Time) string {
	if t == nil {
		return "-"
	}
	return t.Format("2006-01-02")
}

func boolPtr(b *bool) string {
	if b == nil {
		return "-"
	}
	if *b {
		return "1"
	}
	return "0"
}

func intPtr(i *int) string {
	if i == nil {
		return "-"
	}
	return strconv.Itoa(*i)
}
