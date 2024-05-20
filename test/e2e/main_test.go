//go:build e2e
// +build e2e

package e2e

import (
	"testing"

	"github.com/AthenZ/k8s-athenz-syncer/test/e2e/framework"
)

func TestMain(m *testing.M) {
	framework.MainEntry(m)
}
