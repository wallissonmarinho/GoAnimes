package domain

import "errors"

var (
	ErrDuplicateRSSSourceURL = errors.New("duplicate rss source url")
	ErrInvalidSourceURL      = errors.New("invalid source url")
)
