package protocol

import (
	"github.com/neoforth/xray-core/common/errors"
	"github.com/neoforth/xray-core/common/serial"
)

func (u *User) GetTypedAccount() (Account, error) {
	if u.GetAccount() == nil {
		return nil, errors.New("Account is missing").AtWarning()
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
		// Reserved for global device limit
		ID:	u.ID,

		Account:	account,
		Email:		u.Email,
		Level:		u.Level,

		// Device limit and speed limit
		DeviceLimit:	u.DeviceLimit,
		SpeedLimit:	u.SpeedLimit,
	}, nil
}

func ToProtoUser(mu *MemoryUser) *User {
	if mu == nil {
		return nil
	}
	return &User{
		// Reserved for global device limit
		ID:	mu.ID,

		Account:	serial.ToTypedMessage(mu.Account.ToProto()),
		Email:		mu.Email,
		Level:		mu.Level,

		// Device limit and speed limit
		DeviceLimit:	mu.DeviceLimit,
		SpeedLimit:	mu.SpeedLimit,
	}
}

// MemoryUser is a parsed form of User, to reduce number of parsing of Account proto.
type MemoryUser struct {
	// Reserved for global device limit
	ID	uint32

	// Account is the parsed account of the protocol.
	Account	Account
	Email	string
	Level	uint32

	// Device limit and speed limit
	DeviceLimit	uint32
	SpeedLimit	uint64
}
