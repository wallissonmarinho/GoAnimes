package domain

// RssMainFeedBuildFingerprint is the body ETag/Last-Modified + SHA-256 of a top-level RSS URL used for the last catalog build.
type RssMainFeedBuildFingerprint struct {
	ETag         string `json:"etag,omitempty"`
	LastModified string `json:"last_modified,omitempty"`
	SHA256Hex    string `json:"sha256,omitempty"`
}
