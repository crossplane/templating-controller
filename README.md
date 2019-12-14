# Resource Pack Reconciler

You can use this generic reconciler to produce a Crossplane Stack
with minimal effort added to having only kustomize resources.

## Quick Start

### Boilerplate

First, you need to lay down the boilerplate needed for a stack.

1. Install [Kubebuilder]
1. Install [Crossplane CLI].
2. Create your project folder and initialize it with the following command:
```bash
GO111MODULE=on kubebuilder init --domain helloworld.stacks.crossplane.io
```
4. Create your API object, i.e. CRD
```bash
GO111MODULE=on kubebuilder create api --controller=true --example=true --group samples --version v1alpha1 --kind HelloWorld --make=true --namespaced=false --resource=true
GO111MODULE=on make manager manifests
```
5. Initialize Stack boilerplate:
```bash
kubectl crossplane stack init --cluster 'crossplane-examples/hello-world'
```

At this point, you have everything you need to have a Kubernetes Controller alongside
the boilerplate you need to make it a Crossplane Stack.

### Defining CRD

You need to define the fields of your CRD struct in `api/v1alpha1/samples_types.go`
You can add any field you want in `Spec` and `Status`.

In order for the generic reconciler to work, your object needs to have `Conditions`
array in its status. Run the following to get `crossplane-runtime`:
```bash
go get github.com/crossplaneio/crossplane-runtime@075a01671bb776eae70242d535960cda6a0a2b51
```

Now add `ConditionedStatus` to `Status` field of your CRD. It's recommended to add
it as inline in the status. Also, you need to define the following functions for
your main CRD struct, which is `HelloWorld` in that case:
```go
func (hw *HelloWorld) GetCondition(ct v1alpha1.ConditionType) v1alpha1.Condition {
	return hw.Status.GetCondition(ct)
}

func (hw *HelloWorld) SetConditions(c ...v1alpha1.Condition) {
	hw.Status.SetConditions(c...)
}
```

An example status and additional functions look like the following:
```go
// HelloWorldStatus defines the observed state of HelloWorld
type HelloWorldStatus struct {
	v1alpha1.ConditionedStatus `json:",inline"`
}

func (mg *HelloWorld) GetCondition(ct v1alpha1.ConditionType) v1alpha1.Condition {
 	return mg.Status.GetCondition(ct)
 }
 
 func (mg *HelloWorld) SetConditions(c ...v1alpha1.Condition) {
 	mg.Status.SetConditions(c...)
 }
```

There are other utilities crossplane-runtime that could be useful such as `SecretKeySelector`
If you need to refer to a credential secret, you can use `SecretKeySelector`
as type of that field from crossplane-runtime. See [SecretKeySelector]

Run the following command to have your CRD resource generated for you:
```bash
GO111MODULE=on make manager manifests
```

### Calling Generic Reconciler

Now we have everything in place, the last step we need to take is to import this
module and call the generic reconciler.
```bash
go get github.com/muvaf/crossplane-resourcepacks
```

You need to change your `SetupWithManager` function in `controllers/helloworld_controller.go` 
to look like the following:
```go
var (
	HelloWorldKind             = reflect.TypeOf(v1alpha1.HelloWorld{}).Name()
	HelloWorldKindAPIVersion   = HelloWorldKind + "." + v1alpha1.GroupVersion.String()
	HelloWorldGroupVersionKind = v1alpha1.GroupVersion.WithKind(HelloWorldKind)
)

// HelloWorldReconciler reconciles a HelloWorld object
type HelloWorldReconciler struct {
	client.Client
	Log    logr.Logger
	Scheme *runtime.Scheme
}

func (r *HelloWorldReconciler) SetupWithManager(mgr ctrl.Manager) error {
	csr := controllers.NewResourcePackReconciler(mgr, HelloWorldGroupVersionKind)
	return ctrl.NewControllerManagedBy(mgr).
		For(&v1alpha1..HelloWorld{}).
		Complete(csr)
}
```

### Resources

Now, we need to put the resources that we want to be deployed by our controller.
Currently, only `kustomize` engine is supported, so, it has to be in `kustomize` format.
The name of the folder that you need to add your `kustomize` resources is `resources`
at the top level of your project.

There is one additional thing you have to do if you want to propagate down
some values from you CR instance to the deployed resources. You can add a 
kustomization file named `kustomization.yaml.tmpl` that describes the variant
references between your resources and your CR. If provided, this template
file is used as template for the overlay that your controller will apply each time
it deploys your resources.

Example: [Minimal GCP Resource Pack]

### Build and Push!

It's all set up now. You can fill the stack metadata information in `config/stack/app.yaml`
file if you'd like to but it's not mandatory. Run the following to get a Crossplane
Stack image:
```bash
kubectl crossplane stack build
```

Now that you've got your stack image, you can push it to a registry and use it in
your cluster.

### Local Testing

If you'd like to test your controller in your local machine, you can do so by first
registering your CRD with the following command:
```bash
kubectl apply -k config/crd
```

Then you can run your controller just like any other Go program after building it.
Example:
```bash
GO111MODULE=on make manager manifests
go build main.go
KUBECONFIG=<path to your kubeconfig> ./main
```

[Minimal GCP Resource Pack]: https://github.com/muvaf/minimal-gcp/tree/master/resources
[Kubebuilder]: https://book.kubebuilder.io/quick-start.html#installation
[Crossplane CLI]: https://github.com/crossplaneio/crossplane-cli/#installation
[SecretKeySelector]: https://github.com/crossplaneio/crossplane-runtime/blob/ca4b6b4/apis/core/v1alpha1/resource.go#L77