package erp

const (
	ScopeProductsSync   = "erp.products.sync"
	ScopeOrdersRead     = "erp.orders.read"
	ScopeOrdersAck      = "erp.orders.ack"
	ScopeOrdersDecide   = "erp.orders.decide"
	ScopePurchaseCreate = "erp.po.create"
)

func AllScopes() []string {
	return []string{ScopeProductsSync, ScopeOrdersRead, ScopeOrdersAck, ScopeOrdersDecide, ScopePurchaseCreate}
}
func DefaultScopes(vendor bool) []string {
	if vendor {
		return []string{ScopeProductsSync, ScopeOrdersRead, ScopeOrdersAck, ScopeOrdersDecide}
	}
	return []string{ScopeOrdersRead, ScopeOrdersAck, ScopePurchaseCreate}
}
func ValidScope(s string) bool {
	for _, scope := range AllScopes() {
		if s == scope {
			return true
		}
	}
	return false
}
