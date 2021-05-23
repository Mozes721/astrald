package net

import "errors"

// ErrAlreadyRegistered - a different dial is already registered for the network
var ErrAlreadyRegistered = errors.New("network already registered")

var ErrInvalidNetworkName = errors.New("invalid network name")

// ErrUnsupportedNetwork - address's network is not supported.
var ErrUnsupportedNetwork = errors.New("unsupported network")

var ErrHostUnreachable = errors.New("host unreachable")
