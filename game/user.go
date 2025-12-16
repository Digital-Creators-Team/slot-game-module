package game

// User represents the current user in the game context
// This is always available in ModuleContext
type User struct {
	userID     string
	username   string
	currencyID string
}

// ID returns the user ID
func (u *User) ID() string {
	return u.userID
}

// Username returns the username
func (u *User) Username() string {
	return u.username
}

// CurrencyID returns the currency ID
func (u *User) CurrencyID() string {
	return u.currencyID
}

// NewUser creates a new User instance
func NewUser(userID, username, currencyID string) *User {
	return &User{
		userID:     userID,
		username:   username,
		currencyID: currencyID,
	}
}
