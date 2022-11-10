package proc

import (
	"strings"
	"testing"

	"github.com/anyinone/go-zero/core/logx"
	"github.com/stretchr/testify/assert"
)

func TestProfile(t *testing.T) {
	var buf strings.Builder
	w := logx.NewWriter(&buf)
	o := logx.Reset()
	logx.SetWriter(w)

	defer func() {
		logx.Reset()
		logx.SetWriter(o)
	}()

	profiler := StartProfile()
	// start again should not work
	assert.NotNil(t, StartProfile())
	profiler.Stop()
	// stop twice
	profiler.Stop()
	assert.True(t, strings.Contains(buf.String(), ".pprof"))
}
