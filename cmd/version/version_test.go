package version

import (
	"bytes"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestDumpVersion(t *testing.T) {
	t.Run("errors if version not linked", func(t *testing.T) {
		assert.NotNil(t, DumpVersion())
	})

	t.Run("dumps the version out", func(t *testing.T) {
		buf := bytes.NewBuffer([]byte{})

		CommitHash = "thing"
		altFunc := OutputFunc
		OutputFunc = func(s string, args ...interface{}) (int, error) {
			return fmt.Fprintf(buf, s, args...)
		}
		defer func() {
			OutputFunc = altFunc
			CommitHash = ""
		}()

		assert.Nil(t, DumpVersion())
		assert.Equal(t, "\nVersion: \nCommit: thing\nDate: \n", buf.String())
	})
}
