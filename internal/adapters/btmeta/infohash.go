package btmeta

import (
	"bytes"
	"strings"

	"github.com/anacrolix/torrent/metainfo"
)

// InfoHashHexFromTorrentBody parses a .torrent file and returns the 40-char lowercase info hash hex.
func InfoHashHexFromTorrentBody(body []byte) (string, error) {
	mi, err := metainfo.Load(bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	return strings.ToLower(mi.HashInfoBytes().HexString()), nil
}
