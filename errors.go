package mongogit

import (
	"errors"
	"fmt"
)

var (
	ErrNotFound      = errors.New("not found")
	ErrAlreadyExists = errors.New("already exists")
	ErrInvalidParams = errors.New("invalid params")
	ErrInternal      = errors.New("internal")
)

func ErrInternalBy(err error) error {
	if err == nil {
		return nil
	}
	return fmt.Errorf("%w: %w", ErrInternal, err)
}
