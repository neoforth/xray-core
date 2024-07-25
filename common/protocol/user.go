package protocol

import "github.com/xtls/xray-core/common/errors"

func (u *User) GetTypedAccount() (Account, error) {
	if u.GetAccount() == nil {
		return nil, errors.New("Account missing").AtWarning()
	}

	rawAccount, err := u.Account.GetInstance()
	if err != nil {
		return nil, err
	}
	if asAccount, ok := rawAccount.(AsAccount); ok {
		return asAccount.AsAccount()
	}
	if account, ok := rawAccount.(Account); ok {
		return account, nil
	}
	return nil, errors.New("Unknown account type: ", u.Account.Type)
}

func (u *User) ToMemoryUser() (*MemoryUser, error) {
	account, err := u.GetTypedAccount()
	if err != nil {
		return nil, err
	}
	return &MemoryUser{
		// For global device limit
		ID: u.ID,

		Account: account,
		Email:   u.Email,
		Level:   u.Level,

		// Device limit
		DeviceLimit: u.DeviceLimit,
		// Speed limit
		SpeedLimit:  u.SpeedLimit,
	}, nil
}

// MemoryUser is a parsed form of User, to reduce number of parsing of Account proto.
type MemoryUser struct {
	// For global device limit
	ID uint32

	// Account is the parsed account of the protocol.
	Account Account
	Email   string
	Level   uint32

	// Device limit
	DeviceLimit uint32
	// Speed limit
	SpeedLimit  uint64
}
