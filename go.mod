module github.com/crossplaneio/templating-controller

go 1.13

require (
	github.com/crossplaneio/crossplane v0.7.0-rc.0.20200206230838-4534223ff95e
	github.com/crossplaneio/crossplane-runtime v0.4.1-0.20200201005410-a6bb086be888
	github.com/google/go-cmp v0.4.0
	github.com/pkg/errors v0.9.1
	golang.org/x/net v0.0.0-20191028085509-fe3aa8a45271
	gopkg.in/alecthomas/kingpin.v2 v2.2.6
	gopkg.in/yaml.v2 v2.2.4
	gopkg.in/yaml.v3 v3.0.0-20190905181640-827449938966
	helm.sh/helm/v3 v3.0.0-20200205083830-5ec70ab27fbf
	k8s.io/api v0.17.2
	k8s.io/apiextensions-apiserver v0.17.2
	k8s.io/apimachinery v0.17.2
	k8s.io/client-go v0.17.2
	sigs.k8s.io/controller-runtime v0.4.0
	sigs.k8s.io/kustomize/api v0.3.0
	sigs.k8s.io/yaml v1.1.0
)

replace (
	// Helm requires these in this version. In master, they are not required,
	// don't forget to update when you bump the version of helm.
	github.com/Azure/go-autorest => github.com/Azure/go-autorest v13.3.2+incompatible
	github.com/docker/distribution => github.com/docker/distribution v0.0.0-20191216044856-a8371794149d
	github.com/russross/blackfriday => github.com/russross/blackfriday v1.5.2
)
