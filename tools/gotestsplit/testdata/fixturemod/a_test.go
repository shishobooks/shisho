package fixturemod

import (
	"testing"
	"time"
)

func TestSlow(t *testing.T)   { time.Sleep(150 * time.Millisecond) }
func TestMedium(t *testing.T) { time.Sleep(80 * time.Millisecond) }
func TestFast(t *testing.T)   { time.Sleep(20 * time.Millisecond) }
