package domain

// Transaction represents a transfer between two accounts. Amount is in micros.
type Transaction struct {
	From   string
	To     string
	Amount int64
}
