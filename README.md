# Generic Templating Controller

This controller fetches the given `StackDefinition` custom resource as configuration and reconciles the given CRD, which should exist in cluster already. The controller will construct a reconciler in the initialization phase and register it to the api-server to receive events for the CRD given in `spec.behavior.crd`. The reconciler is constructed with the configuration under `spec.behavior.engine`.

Here is an example `StackDefinition` object that uses `kustomize` engine:

```yaml
---
apiVersion: stacks.crossplane.io/v1alpha1
kind: StackDefinition
metadata:
  name: template-stack-test
spec:
  behavior:
    source:
      image: crossplane/stack-minimal-gcp:0.0.2
      path: "kustomize"
    crd:
      kind: MinimalGCP
      apiVersion: gcp.resourcepacks.crossplane.io/v1alpha1
    engine:
      type: kustomize
      kustomize:
        overlays:
          - apiVersion: gcp.crossplane.io/v1alpha3
            kind: Provider
            name: gcp-provider
            bindings:
              - from: "spec.credentialsSecretRef"
                to: "spec.credentialsSecretRef"
              - from: "spec.projectID"
                to: "spec.projectID"
          - apiVersion: cache.gcp.crossplane.io/v1beta1
            kind: CloudMemorystoreInstanceClass
            name: cloudmemorystore
            bindings:
              - from: "spec.region"
                to: "specTemplate.forProvider.region"
```

The reconciler will use `kustomize` as engine and it will produce an overlay with the given objects above. What's happening there is that a `Provider` object will be created as strategic patch overlay with two bindinds; `from` is the field path for the actual CR instance and `to` is the field path of the field on the `Provider` object.

See `test` folder to give it a spin.

## Build

Run `make` to build the latest version.
