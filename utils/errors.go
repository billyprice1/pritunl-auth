package utils

import (
	"github.com/dropbox/godropbox/errors"
)

type ParseError struct {
	errors.DropboxError
}

type InvalidLicenseError struct {
	errors.DropboxError
}
