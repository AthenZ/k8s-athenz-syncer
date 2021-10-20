module github.com/yahoo/k8s-athenz-syncer

go 1.14

require (
	github.com/ardielle/ardielle-go v1.5.2
	github.com/cenkalti/backoff v2.2.1+incompatible
	github.com/fsnotify/fsnotify v1.4.9
	github.com/google/go-cmp v0.4.0
	github.com/imdario/mergo v0.3.7 // indirect
	github.com/mash/go-accesslog v1.2.0
	github.com/mohae/deepcopy v0.0.0-20170929034955-c48cc78d4826
	github.com/pkg/errors v0.9.1
	github.com/sirupsen/logrus v1.4.2
	github.com/stretchr/testify v1.5.1
	github.com/tevino/abool v0.0.0-20170917061928-9b9efcf221b5
	github.com/yahoo/athenz v1.9.30
	gopkg.in/natefinch/lumberjack.v2 v2.0.0
	k8s.io/api v0.19.15
	k8s.io/apimachinery v0.19.15
	k8s.io/client-go v0.19.15
)
