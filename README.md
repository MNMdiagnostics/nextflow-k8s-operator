# nextflow-k8s-operator

`nextflow-k8s-operator` (NeKO) is a tool that allows you to run your
[Nextflow](https://www.nextflow.io/) pipelines natively in a Kubernetes
environment, manage and monitor them by means of the Kubernetes client
(`kubectl`).

*[gif]*

NeKO takes the burden of orchestrating your pipelines off your shoulders.  
You can focus on doing science.

## What is a Kubernetes operator?

The **Operator** is a Kubernetes pattern that uses custom resources to
handle applications in a k8s-native way. For example, `nextflow-k8s-operator`
defines a `NextflowLaunch` resource that encompasses the entire launch of a
Nextflow pipeline, including Nextflow configuration, environment settings, and
pipeline parameters. Making it this way allows for reproducible and
easy-to-maintain runs.

Under the hood of an operator is the **controller** - a program running in the
background, taking care of all the tasks that a human operator would typically
do: spawning pods, validating the configuration, monitoring runs, etc.

### Lingo

**controller** - a program that connects with the Kubernetes client, runs in the
background, and manages the custom resources defined by the operator; in our
case, the controller is a binary file called `manager` that you will find in the
`bin/` subdirectory after NeKO has been built (see: _Installation_).

**launch** - a (reusable) artifact that makes a "recipe" for a single run of a
Nextflow pipeline; since they are Kubernetes custom resources, Nextflow launches
are defined as yaml files (see: _Usage_).

**driver** - the main pod launched by the controller, it contains an instance of
Nextflow; a single launch spawns only one driver pod, which creates any number
of worker pods, doing the actual work.

**worker** - a pod that does the "heavy lifting" of the Nextflow pipeline;
one or more workers are launched from within the driver.

## Installation

The controller can be running either as a pod on the Kubernetes cluster, or
as a standalone program running on the local machine (NOTE: although connection
with Kubernetes is required, the controller can be run on any machine, including
your personal computer).

Regardless of the mode of execution, start with installing the custom resources
used by NeKO:

``` sh
make install
```

### As a pod

Build the Docker image:

``` sh
make docker-build IMG=mycontroller:latest
```

A Docker image named `mycontroller` will be created. You can push it to an image
registry:

``` sh
make docker-push IMG=mycontroller:latest
```

Finally, deploy the controller to the Kubernetes cluster:

``` sh
make deploy IMG=mycontroller:latest
```

To uninstall NeKO, run:

``` sh
make undeploy uninstall
```

### As a standalone program

Build the controller:

``` sh
make
```

Following a successful build, run the controller:

``` sh
bin/manager
```

The controller's execution log will be visible in the terminal.

### Service accounts

When launching pipelines on a Kubernetes cluster, users may stumble upon a
permissions problem, usually manifesting itself by throwing 403 errors in
the logs, accompanied by the name of the service account used.

This problem can be solved by binding a more powerful role to that account,
for example:

``` yaml
apiVersion: rbac.authorization.k8s.io/v1
kind: Role
metadata:
  name: nextflow-role
  labels:
    app: nextflow-role
rules:
- apiGroups:
  - ""
  - apps
  - autoscaling
  - batch
  - extensions
  - policy
  - rbac.authorization.k8s.io
  resources:
  - pods
  - pods/status
  - pods/log
  - persistentvolumes
  - persistentvolumeclaims
  - configmaps
  verbs: ["get", "list", "watch", "create", "update", "patch", "delete"]

---
apiVersion: rbac.authorization.k8s.io/v1
kind: RoleBinding
metadata:
  name: nextflow-role-binding
roleRef:
  apiGroup: rbac.authorization.k8s.io
  kind: Role
  name: nextflow-role
subjects:
- kind: ServiceAccount
  name: default   # this is the name of the service account in question
```

## Usage

To start an already prepared Nextflow launch, run: `kubectl apply -f
my-launch.yaml`, where `my-launch.yaml` is the name of your definition file.

As an example, see [hello.yaml](config/samples/hello.yaml); the file contains
the definition of a simple launch (see: _The essentials_ in _Configuring your
pipelines_ for explanation) as well as definitions of a
[PV-PVC](https://kubernetes.io/docs/concepts/storage/persistent-volumes/)
pair.

If your pipeline has finished with success, you will see the results yielded by
the pipeline by viewing the logs from the driver pod (if the name of your launch
is `hello`, it will be named `hello-xxxxxxxx`, where `xxxxxxxx` is a random hash
assigned to the pod), for example: `kubectl logs hello-7a69dc11`.

If you're running the pipeline on a remote cluster, though, it is possible
that your job has failed due to the restrictions imposed on the user by the
environment. Typical issues include so-called
[taints](https://kubernetes.io/docs/concepts/scheduling-eviction/taint-and-toleration/)
which require setting respective tolerations in your launch's definition,
and insufficient permissions assigned to the service account running the
launch. Both topics are described in detail elsewhere in this document.

Either way, if you don't need the launch anymore on your cluster, remove it
with `kubectl delete -f hello.yaml`. (NOTE: if the pipeline has saved any
artifacts to the persistent volume, they will be safe!)

## Configuring your pipelines

As has been mentioned, both the configuration of the computational pipeline
and the Nextflow environment are defined in a yaml file as a Kubernetes
[custom resource](https://kubernetes.io/docs/concepts/extend-kubernetes/api-extension/custom-resources/)
called `NextflowLaunch`.

To accommodate for the complex and demanding runtime environment that Kubernetes
is, some k8s-specific configuration options are available, in addition to the
settings provided by Nextflow.

### The essentials

The most trivial example of a valid Nextflow launch definition can be seen
below:

``` yaml
apiVersion: batch.mnm.bio/v1alpha1
kind: NextflowLaunch
metadata:
  name: hello
spec:
  pipeline:
    source: hello
  k8s:
    storageClaimName: hello-pvc
```

This launch, when run, will download and execute Nextflow's "hello world"
pipeline, available at https://github.com/nextflow-io/hello . This is defined
in the `pipeline.source` section of the yaml file.
In general, the same rules follow as when running Nextflow pipeline from the
command line.

Optionally, the pipeline can be downloaded from a branch other than the main
branch, or from a Git tag/revision. This can be achieved by declaring the
branch/revision in the `pipeline.revision` section.

The other setting that is required is `k8s.storageClaimName`. This is the name
of the persistent volume claim that both the driver and the workers will mount
and use. It can be mounted at any mounting point, and freely used by the
pipeline and other scripts running in the pod.

Let's move on to read about the configuration options that NeKO provides.

### `k8s`, `params` and `env`

These sections (defined within `spec` in the yaml file; see above) are
equivalents of the respective Nextflow scopes. A short example is shown
below:

``` yaml
spec:
  k8s:
    storageClaimName: my-pvc
    storageMountPath: /my-workspace
  params:
    manifest: my_manifest.json
    outputDir: /my-workspace/output
  env:
    SHELL: zsh
```

#### k8s

Here, a PVC called `my-pvc` will be mounted at `/my-workspace`.

NOTE: unless defined explicitly, the vital directories are set as follows:

* `storageMountPath: /workspace`
* `launchDir: <storageMountPath>/<launch_name>`
* `workDir: <launchDir>/work`

This is similar to Nextflow defaults.

For all available configuration options, see
https://www.nextflow.io/docs/edge/config.html#scope-k8s .

#### params

In the example, two pipeline parameters are defined: `manifest` and
`outputDir`.

For reference, see
https://www.nextflow.io/docs/edge/config.html#scope-params .

#### env

Like above, it is possible to set environment variables (`SHELL` in the
example) in the _worker_ pods (for setting variables in the driver pod,
see: _Configuring the driver_).

For reference, see
https://www.nextflow.io/docs/edge/config.html#scope-env .

### Pod options

The `pod` directive is part of the `process` scope. Several k8s-specific
settings are available, including more sophisticated ways of setting
environment variables (from secrets, config maps, etc.).

For a full list, see
https://www.nextflow.io/docs/edge/process.html#process-pod .

Most of these options can be defined as simple key-value maps, for example:

``` yaml
spec:
  pod:
  - label: foo
    value: bar
  - imagePullSecret: my-secret
```

However, some pod configuration options are free-form maps which, for technical
reasons, cannot be easily implemented in a Kubernetes CRD (custom resource
definition). Hence, `nextflow-k8s-operator` provides a dedicated syntax for
declarations which are not simple key-value pairs. For example (note the
`(map)`):

``` yaml
spec:
  pod:
  - toleration: (map)
    key: nextflow
    operator: Equal
    value: "true"
    effect: NoSchedule
```

translates to:

```
pod = [
  [
    toleration: [
      key: 'nextflow',
      operator: 'Equal',
      value: 'true',
      effect: 'NoSchedule',
    ],
  ],
]
```

### Customizing Nextflow

By default, a predefined version of Nextflow is used as a driver for the
launches (it can be changed in the [creators.go](controllers/creators.go)
file, but the code has to be recompiled and re-run afterwards).

To enable more flexibility in the selection of the runtime environment,
the `nextflow` section provides options for customizing the Nextflow
environment used for the driver pod:

`nextflow.image`: the name of the Nextflow Docker image used by the _driver_
(**not** the workers), without version tag. By default, `nextflow/nextflow`.
Please note that is possible to use software other than Nextflow by choosing
a non-Nextflow image!

`nextflow.version`: version tag for the Docker image.

`nextflow.command`: custom command launching Nextflow in the driver pod.
By default, `nextflow run` is exectued with some command-line parameters.
This is a good place to add custom invokations to the Nextflow command,
or execute some other script pre-launch. (NOTE: see examples of command
declarations in Kubernetes pod definitions for reference.)

`nextflow.args`: if you want to keep the default command line and only add
some arguments to it (for example, `-resume`), it's better to specify them
in this section (in the same way you'd specify the command declaration in
`nextflow.command`). Your arguments will be appended to the original command.

`nextflow.home`: this changes the path to Nextflow's home directory.
Point it to a persistent volume if you want to keep the Nextflow environment
between launches.

`nextflow.logPath`: custom path for the log file. (NOTE: it should include
the filename as well.)

`nextflow.scmSecretName`: this important setting allows for downloading
pipelines from private (or otherwise restricted) repositories. It points to
a Kubernetes secret holding the contents of Nextflow SCM configuration file
(see https://www.nextflow.io/docs/latest/sharing.html#scm-configuration-file ).
To create the secret, use `make_scm_secret.sh | kubectl apply -f -`.

### Configuring the driver

The options described in the previous sections impact only the _worker_ pods
(which are handled by the Nextflow process, just like when Nextflow is launched
from the command line). To enable the configuration of the driver pod, some
configuration options have been added to the launch definition that are not
present in Nextflow. These include:

`driver.env`: definitions of environment variables for the driver. This section
is identical with the `env` section in any Kubernetes pod definition (this
includes using secrets and config maps as sources for the variables).

`driver.tolerations`: driver pod tolerations. Defined exactly like in a pod
definition.

An example of the same toleration set both for the driver and the workers is
shown below.

``` yaml
spec:
  pod:
  - toleration: (map)
    key: core
    operator: Equal
    value: "true"
    effect: NoSchedule
  driver:
    tolerations:
    - key: core
      operator: Equal
      value: "true"
      effect: NoSchedule
```

## Acknowledgements

`nextflow-k8s-operator` has been created with [Kubebuilder](https://kubebuilder.io/).
