package translate

import (
	"errors"
	"strconv"
	"strings"

	"github.com/wallissonmarinho/GoAnimes/internal/core/ports"
)

// Chain tries each SynopsisTranslator in order until one returns non-empty text without error.
type Chain []ports.SynopsisTranslator

var _ ports.SynopsisTranslator = Chain(nil)

// Name summarizes the chain length.
func (c Chain) Name() string {
	return "chain(" + strconv.Itoa(len(c)) + ")"
}

// Translate delegates to adapters in order.
func (c Chain) Translate(text, source, target string) (string, error) {
	if len(c) == 0 {
		return "", errors.New("translate: empty chain")
	}
	var lastErr error
	for _, t := range c {
		if t == nil {
			continue
		}
		out, err := t.Translate(text, source, target)
		if err == nil && strings.TrimSpace(out) != "" {
			return out, nil
		}
		if err != nil {
			lastErr = err
		}
	}
	if lastErr != nil {
		return "", lastErr
	}
	return "", errors.New("translate: chain exhausted")
}

// Merge flattens non-nil translators and nested Chains into one Chain (order preserved).
// Use in main to combine translate.FromEnv(getter) with custom adapters (e.g. Google, DeepL).
func Merge(ts ...ports.SynopsisTranslator) ports.SynopsisTranslator {
	var nodes []ports.SynopsisTranslator
	for _, t := range ts {
		if t == nil {
			continue
		}
		if ch, ok := t.(Chain); ok {
			for _, x := range ch {
				if x != nil {
					nodes = append(nodes, x)
				}
			}
			continue
		}
		nodes = append(nodes, t)
	}
	if len(nodes) == 0 {
		return nil
	}
	if len(nodes) == 1 {
		return nodes[0]
	}
	return Chain(nodes)
}
