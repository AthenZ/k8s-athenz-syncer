package framework

import (
	"os"
	"testing"

	"github.com/sirupsen/logrus"
)

func MainEntry(t *testing.M) {
	if err := setup(); err != nil {
		logrus.Errorf("fail to setup framework: %v", err)
		os.Exit(1)
	}

	code := t.Run()

	if err := teardown(); err != nil {
		logrus.Errorf("fail to teardown framework: %v", err)
		os.Exit(1)
	}
	os.Exit(code)
}
