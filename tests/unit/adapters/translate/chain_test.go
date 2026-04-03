package translate_test

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/wallissonmarinho/GoAnimes/internal/adapters/translate"
)

type stubTr struct {
	name string
	out  string
	err  error
}

func (s stubTr) Name() string { return s.name }

func (s stubTr) Translate(_, _, _ string) (string, error) {
	return s.out, s.err
}

func TestChain_fallback(t *testing.T) {
	c := translate.Chain{
		stubTr{name: "bad", err: errors.New("fail")},
		stubTr{name: "ok", out: "olá"},
	}
	out, err := c.Translate("hi", "en", "pt")
	require.NoError(t, err)
	require.Equal(t, "olá", out)
}

func TestChain_allFail(t *testing.T) {
	c := translate.Chain{
		stubTr{name: "a", err: errors.New("e1")},
		stubTr{name: "b", err: errors.New("e2")},
	}
	_, err := c.Translate("hi", "en", "pt")
	require.Error(t, err)
}

func TestMerge_flattensChain(t *testing.T) {
	inner := translate.Chain{stubTr{name: "x", out: "ok"}}
	m := translate.Merge(inner, stubTr{name: "y", out: "no"})
	require.NotNil(t, m)
	out, err := m.Translate("t", "en", "pt")
	require.NoError(t, err)
	require.Equal(t, "ok", out)
}
