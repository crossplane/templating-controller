
# Image URL to use all building/pushing image targets
IMG ?= crossplane/templating-controller
VERSION ?= "0.2.0"

all: test docker-build

# Run tests
test: fmt vet
	go test ./... -coverprofile cover.out

# Build manager binary
manager: fmt vet
	go build -o bin/manager main.go

# Run against the configured Kubernetes cluster in ~/.kube/config
run: fmt vet manifests
	go run ./main.go

# Run go fmt against code
fmt:
	go fmt ./...

# Run go vet against code
vet:
	go vet ./...

# Build the docker image
docker-build: test
	docker build . -t ${IMG}:${VERSION}

# Push the docker image
docker-push:
	docker push ${IMG}:${VERSION}