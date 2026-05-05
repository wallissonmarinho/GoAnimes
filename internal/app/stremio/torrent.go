package stremio

import (
	"context"
	"crypto/sha1"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
)

type bencodeValue struct {
	rawStart int
	rawEnd   int
	value    any
}

func resolvePlaybackURL(ctx context.Context, raw string) (string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", nil
	}
	lower := strings.ToLower(raw)
	if strings.HasPrefix(lower, "magnet:") {
		return raw, nil
	}
	if !looksLikeTorrentURL(raw) {
		return raw, nil
	}
	return torrentURLToMagnet(ctx, raw)
}

func looksLikeTorrentURL(raw string) bool {
	parsed, err := url.Parse(strings.TrimSpace(raw))
	if err != nil {
		return strings.HasSuffix(strings.ToLower(raw), ".torrent")
	}
	return strings.HasSuffix(strings.ToLower(parsed.Path), ".torrent")
}

func torrentURLToMagnet(ctx context.Context, torrentURL string) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, torrentURL, nil)
	if err != nil {
		return "", err
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= http.StatusBadRequest {
		return "", fmt.Errorf("torrent download failed: %s", resp.Status)
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, 10<<20))
	if err != nil {
		return "", err
	}
	return torrentBytesToMagnet(data)
}

func torrentBytesToMagnet(data []byte) (string, error) {
	parsed, end, err := parseBencode(data, 0)
	if err != nil {
		return "", err
	}
	if end != len(data) {
		return "", fmt.Errorf("unexpected torrent payload tail")
	}
	top, ok := parsed.value.(map[string]bencodeValue)
	if !ok {
		return "", fmt.Errorf("invalid torrent root")
	}
	info, ok := top["info"]
	if !ok {
		return "", fmt.Errorf("torrent missing info dict")
	}
	infoRaw := data[info.rawStart:info.rawEnd]
	hash := sha1.Sum(infoRaw)
	magnet := "magnet:?xt=urn:btih:" + hex.EncodeToString(hash[:])

	params := make([]string, 0, 8)
	params = append(params, "xt=urn:btih:"+hex.EncodeToString(hash[:]))
	if announce, ok := top["announce"]; ok {
		if s, ok := announce.value.(string); ok && strings.TrimSpace(s) != "" {
			params = append(params, "tr="+url.QueryEscape(strings.TrimSpace(s)))
		}
	}
	if announceList, ok := top["announce-list"]; ok {
		params = append(params, collectTrackers(announceList.value)...)
	}
	if infoDict, ok := info.value.(map[string]bencodeValue); ok {
		if name, ok := infoDict["name"]; ok {
			if s, ok := name.value.(string); ok && strings.TrimSpace(s) != "" {
				params = append(params, "dn="+url.QueryEscape(strings.TrimSpace(s)))
			}
		}
	}
	if len(params) == 1 {
		return magnet, nil
	}
	return "magnet:?" + strings.Join(params, "&"), nil
}

func collectTrackers(v any) []string {
	out := make([]string, 0, 4)
	var walk func(any)
	walk = func(value any) {
		switch typed := value.(type) {
		case string:
			if trimmed := strings.TrimSpace(typed); trimmed != "" {
				out = append(out, "tr="+url.QueryEscape(trimmed))
			}
		case []bencodeValue:
			for _, item := range typed {
				walk(item.value)
			}
		case []any:
			for _, item := range typed {
				walk(item)
			}
		case map[string]bencodeValue:
			for _, item := range typed {
				walk(item.value)
			}
		}
	}
	walk(v)
	return out
}

func parseBencode(data []byte, pos int) (bencodeValue, int, error) {
	if pos >= len(data) {
		return bencodeValue{}, pos, fmt.Errorf("unexpected end of bencode")
	}
	start := pos
	switch data[pos] {
	case 'i':
		pos++
		neg := false
		if pos < len(data) && data[pos] == '-' {
			neg = true
			pos++
		}
		if pos >= len(data) {
			return bencodeValue{}, pos, fmt.Errorf("invalid integer")
		}
		n := 0
		for pos < len(data) && data[pos] != 'e' {
			if data[pos] < '0' || data[pos] > '9' {
				return bencodeValue{}, pos, fmt.Errorf("invalid integer digit")
			}
			n = n*10 + int(data[pos]-'0')
			pos++
		}
		if pos >= len(data) || data[pos] != 'e' {
			return bencodeValue{}, pos, fmt.Errorf("unterminated integer")
		}
		pos++
		if neg {
			n = -n
		}
		return bencodeValue{rawStart: start, rawEnd: pos, value: n}, pos, nil
	case 'l':
		pos++
		items := make([]bencodeValue, 0, 4)
		for pos < len(data) && data[pos] != 'e' {
			item, next, err := parseBencode(data, pos)
			if err != nil {
				return bencodeValue{}, pos, err
			}
			items = append(items, item)
			pos = next
		}
		if pos >= len(data) || data[pos] != 'e' {
			return bencodeValue{}, pos, fmt.Errorf("unterminated list")
		}
		pos++
		return bencodeValue{rawStart: start, rawEnd: pos, value: items}, pos, nil
	case 'd':
		pos++
		items := make(map[string]bencodeValue)
		for pos < len(data) && data[pos] != 'e' {
			keyVal, next, err := parseBencodeString(data, pos)
			if err != nil {
				return bencodeValue{}, pos, err
			}
			key, _ := keyVal.value.(string)
			pos = next
			val, next, err := parseBencode(data, pos)
			if err != nil {
				return bencodeValue{}, pos, err
			}
			items[key] = val
			pos = next
		}
		if pos >= len(data) || data[pos] != 'e' {
			return bencodeValue{}, pos, fmt.Errorf("unterminated dict")
		}
		pos++
		return bencodeValue{rawStart: start, rawEnd: pos, value: items}, pos, nil
	default:
		return parseBencodeString(data, pos)
	}
}

func parseBencodeString(data []byte, pos int) (bencodeValue, int, error) {
	start := pos
	length := 0
	for pos < len(data) && data[pos] != ':' {
		if data[pos] < '0' || data[pos] > '9' {
			return bencodeValue{}, pos, fmt.Errorf("invalid string length")
		}
		length = length*10 + int(data[pos]-'0')
		pos++
	}
	if pos >= len(data) || data[pos] != ':' {
		return bencodeValue{}, pos, fmt.Errorf("unterminated string length")
	}
	pos++
	end := pos + length
	if end > len(data) {
		return bencodeValue{}, pos, fmt.Errorf("string overruns payload")
	}
	return bencodeValue{rawStart: start, rawEnd: end, value: string(data[pos:end])}, end, nil
}
