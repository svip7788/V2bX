package sing

import (
	"testing"

	"github.com/InazumaV/V2bX/conf"
)

func TestNewWithoutOriginalConfig(t *testing.T) {
	c, err := New(&conf.CoreConfig{Type: "sing", SingConfig: conf.NewSingConfig()})
	if err != nil {
		t.Fatal(err)
	}
	_ = c.Close()
}
