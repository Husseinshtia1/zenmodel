module github.com/zenmodel/zenmodel/example

go 1.21.0

require github.com/zenmodel/zenmodel v0.1.0

require (
	github.com/go-logr/logr v1.3.0 // indirect
	github.com/google/uuid v1.6.0 // indirect
	github.com/pkg/errors v0.9.1 // indirect
	golang.org/x/time v0.3.0 // indirect
	k8s.io/apimachinery v0.29.2 // indirect
	k8s.io/client-go v0.29.2 // indirect
	k8s.io/klog/v2 v2.110.1 // indirect
	k8s.io/utils v0.0.0-20230726121419-3b25d923346b // indirect
)

replace github.com/zenmodel/zenmodel v0.1.0 => ./..
