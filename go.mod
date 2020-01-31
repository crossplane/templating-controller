module github.com/crossplaneio/templating-controller

go 1.13

replace github.com/crossplaneio/crossplane => github.com/suskin/crossplane v0.2.1-0.20200130230013-f513309cdebb

require (
	github.com/crossplaneio/crossplane v0.7.0
	github.com/crossplaneio/crossplane-runtime v0.4.0
	github.com/google/go-cmp v0.3.1
	github.com/pkg/errors v0.8.1
	golang.org/x/net v0.0.0-20191004110552-13f9640d40b9
	gopkg.in/alecthomas/kingpin.v2 v2.2.6
	gopkg.in/yaml.v2 v2.2.4
	k8s.io/api v0.17.0
	k8s.io/apimachinery v0.17.1
	k8s.io/client-go v0.17.0
	sigs.k8s.io/controller-runtime v0.4.0
	sigs.k8s.io/kustomize/api v0.3.0
	sigs.k8s.io/yaml v1.1.0
)
