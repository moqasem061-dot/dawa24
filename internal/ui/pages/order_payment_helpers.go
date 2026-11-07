package pages

import (
	"fmt"

	"github.com/moqasem061-dot/dawa24/internal/modules/commerce"
	"github.com/moqasem061-dot/dawa24/internal/shared/i18n"
)

// FormatPaymentMethodName returns a localized, human-friendly name for any payment method.
func FormatPaymentMethodName(method, lang string) string {
	switch method {
	case "cod", "cash", "cash_on_delivery":
		return i18n.T(lang, "orders.payment_method.cod")
	case "wallet":
		return i18n.T(lang, "orders.payment_method.wallet")
	case "instapay":
		return i18n.T(lang, "orders.payment_method.instapay")
	case "card", "online_card", "credit_card", "debit_card":
		return i18n.T(lang, "orders.payment_method.card")
	case "bank_transfer", "bank":
		return i18n.T(lang, "orders.payment_method.bank_transfer")
	case "fawry":
		return i18n.T(lang, "orders.payment_method.fawry")
	case "vodafone_cash", "vodafone":
		return i18n.T(lang, "orders.payment_method.vodafone_cash")
	case "orange_cash":
		return i18n.T(lang, "orders.payment_method.orange_cash")
	case "etisalat_cash":
		return i18n.T(lang, "orders.payment_method.etisalat_cash")
	case "we_pay":
		return i18n.T(lang, "orders.payment_method.we_pay")
	case "credit":
		return i18n.T(lang, "orders.payment_method.credit")
	case "online":
		return i18n.T(lang, "orders.payment_method.online")
	default:
		if method == "" {
			return i18n.T(lang, "orders.payment_method.cod")
		}
		return method
	}
}

// IsPrepaidPaymentMethod returns true if the method is not cash-on-delivery or cash.
func IsPrepaidPaymentMethod(method string) bool {
	return method != "cod" && method != "" && method != "cash" && method != "cash_on_delivery"
}

// FormatPaymentStatusLabel returns a localized status label for the order payment status.
func FormatPaymentStatusLabel(status commerce.PaymentStatus, method, lang string, orderStatus ...commerce.OrderStatus) string {
	switch status {
	case commerce.PaymentPaid:
		return i18n.T(lang, "orders.payment_status.paid")
	case commerce.PaymentPartiallyPaid:
		return i18n.T(lang, "orders.payment_status.partially_paid")
	case commerce.PaymentRefunded:
		return i18n.T(lang, "orders.payment_status.refunded")
	case commerce.PaymentPartiallyRefunded:
		return i18n.T(lang, "orders.payment_status.partially_refunded")
	case commerce.PaymentFailed:
		return i18n.T(lang, "orders.payment_status.failed")
	case commerce.PaymentAuthorized:
		if commerce.IsWalletPayment(method) {
			return i18n.T(lang, "orders.payment_status.wallet_authorized")
		}
		return i18n.T(lang, "orders.payment_status.online_authorized")
	default:
		if commerce.IsWalletPayment(method) {
			return i18n.T(lang, "orders.payment_status.wallet_pending")
		}
		if !IsPrepaidPaymentMethod(method) {
			if len(orderStatus) > 0 {
				switch orderStatus[0] {
				case commerce.StatusConfirmed, commerce.StatusProcessing, commerce.StatusShipped, commerce.StatusInTransit, commerce.StatusOutForDelivery:
					return i18n.T(lang, "orders.payment_status.cod_confirmed")
				case commerce.StatusPending:
					return i18n.T(lang, "orders.payment_status.cod_pending")
				case commerce.StatusDelivered, commerce.StatusCompleted:
					return i18n.T(lang, "orders.payment_status.paid")
				}
			}
			return i18n.T(lang, "orders.payment_status.cod_due")
		}
		return i18n.T(lang, "orders.payment_status.online_pending")
	}
}

// PaymentStatusBadgeClass returns the CSS class for the payment status badge.
func PaymentStatusBadgeClass(status commerce.PaymentStatus, method string, orderStatus ...commerce.OrderStatus) string {
	switch status {
	case commerce.PaymentPaid:
		return "badge-emerald"
	case commerce.PaymentPartiallyPaid:
		return "badge-amber"
	case commerce.PaymentRefunded, commerce.PaymentPartiallyRefunded:
		return "badge-purple"
	case commerce.PaymentFailed:
		return "badge-rose"
	case commerce.PaymentAuthorized:
		if commerce.IsWalletPayment(method) {
			return "badge-amber"
		}
		return "badge-sky"
	default:
		if !IsPrepaidPaymentMethod(method) {
			if len(orderStatus) > 0 {
				switch orderStatus[0] {
				case commerce.StatusConfirmed, commerce.StatusProcessing, commerce.StatusShipped, commerce.StatusInTransit, commerce.StatusOutForDelivery:
					return "badge-sky"
				case commerce.StatusDelivered, commerce.StatusCompleted:
					return "badge-emerald"
				}
			}
			return "badge-amber"
		}
		return "badge-sky"
	}
}

// PaymentStatusTextClass returns the text color class for the sidebar status.
func PaymentStatusTextClass(status commerce.PaymentStatus, method string, orderStatus ...commerce.OrderStatus) string {
	switch status {
	case commerce.PaymentPaid:
		return "text-emerald"
	case commerce.PaymentPartiallyPaid:
		return "text-amber"
	case commerce.PaymentRefunded, commerce.PaymentPartiallyRefunded:
		return "text-purple"
	case commerce.PaymentFailed:
		return "text-danger"
	case commerce.PaymentAuthorized:
		if commerce.IsWalletPayment(method) {
			return "text-amber"
		}
		return "text-sky"
	default:
		if !IsPrepaidPaymentMethod(method) {
			if len(orderStatus) > 0 {
				switch orderStatus[0] {
				case commerce.StatusConfirmed, commerce.StatusProcessing, commerce.StatusShipped, commerce.StatusInTransit, commerce.StatusOutForDelivery:
					return "text-sky"
				case commerce.StatusDelivered, commerce.StatusCompleted:
					return "text-emerald"
				}
			}
			return "text-amber"
		}
		return "text-sky"
	}
}

// FormatParcelPINInstruction returns the appropriate PIN instruction based on payment method and courier collection.
func FormatParcelPINInstruction(vendorName, method string, kind commerce.CollectionKind, lang string) string {
	switch kind {
	case commerce.CollectShippingOnly:
		return fmt.Sprintf(i18n.T(lang, "orders.parcel_pin_instruction_shipping_only"), vendorName)
	case commerce.CollectNothing:
		return fmt.Sprintf(i18n.T(lang, "orders.parcel_pin_instruction_prepaid"), vendorName)
	default:
		if IsPrepaidPaymentMethod(method) {
			return fmt.Sprintf(i18n.T(lang, "orders.parcel_pin_instruction_prepaid"), vendorName)
		}
		return fmt.Sprintf(i18n.T(lang, "orders.parcel_pin_instruction"), vendorName)
	}
}
