package framework

import (
	"os"
	"testing"

	"github.com/sirupsen/logrus"
)

func MainEntry(t *testing.M) {
	stopCh := make(chan struct{})

	if err := setup(stopCh); err != nil {
		logrus.Errorf("fail to setup framework: %v", err)
		os.Exit(1)
	}

	defer close(stopCh)
	code := t.Run()

	if err := teardown(); err != nil {
		logrus.Errorf("fail to teardown framework: %v", err)
		os.Exit(1)
	}
	os.Exit(code)
}
