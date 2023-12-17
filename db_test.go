package gameserver_test

import (
	"os"
	"testing"

	"github.com/vkryukov/gameserver"
)

func TestMain(m *testing.M) {
	gameserver.InitDB(":memory:")
	os.Exit(m.Run())
}
