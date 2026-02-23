package domain

// Account represents a financial account. Balance is stored in micros (1 unit = 0.000001 of currency).
type Account struct {
	ID      string
	Balance int64
}
