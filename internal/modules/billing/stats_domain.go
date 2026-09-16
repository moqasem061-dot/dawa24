package billing

import "github.com/muhiya/dawa24-store/internal/shared/money"

// AdminFinanceStats holds platform-wide aggregated financial KPI metrics directly queried from the database.
type AdminFinanceStats struct {
	TotalPaid          money.Amount
	TotalPayments      int
	TotalRevenue       money.Amount
	TotalInvoices      int
	TotalHeld          money.Amount
	TotalWallets       int
	TotalTransactions  int
	TotalDeposits      int
	PendingDeposits    int
	TotalWithdrawals   int
	PendingWithdrawals int
}
