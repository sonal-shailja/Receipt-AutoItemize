package health

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	healthv1 "github.com/sonalshailja/receipt-autoitemize/rpc/common/health/v1"
)

type pinger struct{ err error }

func (p pinger) Ping(context.Context) error { return p.err }

func TestCheck(t *testing.T) {
	got, err := NewHandler(pinger{}).Check(context.Background(), &healthv1.CheckRequest{})
	require.NoError(t, err)
	assert.Equal(t, "SERVING", got.GetStatus())

	got, err = NewHandler(pinger{err: errors.New("disk gone")}).Check(context.Background(), &healthv1.CheckRequest{})
	require.NoError(t, err)
	assert.Equal(t, "NOT_SERVING", got.GetStatus())
	assert.Equal(t, "disk gone", got.GetDependencies()[0].GetError())
}
