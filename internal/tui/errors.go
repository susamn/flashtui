package tui

import "errors"

var (
	errNothingMounted = errors.New("nothing could be mounted; the drive may be unformatted")
	errTargetGone     = errors.New("the target device disappeared after the write")
)

var errNoPublicKey = errors.New("no public key found in ~/.ssh/*.pub")
