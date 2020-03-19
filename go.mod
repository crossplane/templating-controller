module github.com/crossplane/templating-controller

go 1.13

require (
	github.com/crossplane/crossplane v0.8.0-rc.0.20200219030944-557074689d71
	github.com/crossplane/crossplane-runtime v0.5.1-0.20200219022054-8948e50564b1
	github.com/google/go-cmp v0.4.0
	github.com/pkg/errors v0.9.1
	golang.org/x/net v0.0.0-20191028085509-fe3aa8a45271
	gopkg.in/alecthomas/kingpin.v2 v2.2.6
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
