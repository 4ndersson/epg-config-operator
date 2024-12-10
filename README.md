# epg-config-operator
This operator is used to manage a CRD called `Epgconf`. Based on that object, which will be created in a namespace. An EPG is created in ACI. The operator will also add nescessary configuration on the EPG such as BD, VMM, and default contracts.

## Getting Started

### Prerequisites
- go version v1.22.0+
- docker version 17.03+.
- kubectl version v1.11.3+.
- Access to a Kubernetes v1.11.3+ cluster.
- A custom configmap created which contains provided and consumed contracts that will be added to each synced EPG, see `config/samples/default-epg-contracts.yaml`.

### To Deploy on the cluster (alt 1)
**Clone this repo:**
```sh
git clone https://github.com/4ndersson/epg-config-operator
cd epg-config-operator
```

**Build and push your image to the location specified by `IMG`:**

```sh
make docker-build docker-push IMG=<some-registry>/epg-config-operator:tag
```

**NOTE:** This image ought to be published in the personal registry you specified.
And it is required to have access to pull the image from the working environment.
Make sure you have the proper permission to the registry if the above commands don’t work.

**Install the CRDs into the cluster:**

```sh
make install
```

**Deploy the Manager to the cluster with the image specified by `IMG`:**

```sh
make deploy IMG=<some-registry>/epg-config-operator:tag
```

> **NOTE**: If you encounter RBAC errors, you may need to grant yourself cluster-admin
privileges or be logged in as admin.

**Create instances of your solution**
You can apply the samples (examples) from the config/sample:

```sh
kubectl apply -k config/samples/
```

#### To Uninstall (alt 1)
**Delete the instances (CRs) from the cluster:**

```sh
kubectl delete -k config/samples/
```

**Delete the APIs(CRDs) from the cluster:**

```sh
make uninstall
```

**UnDeploy the controller from the cluster:**

```sh
make undeploy
```

### To Deploy on the cluster (alt 2)
**Create a catalogsource:**
```sh
apiVersion: operators.coreos.com/v1alpha1
kind: CatalogSource
metadata:
  name: custom-catalog-source
  namespace: openshift-marketplace 
spec:
  sourceType: grpc
  image: docker.io/4ndersson/operator-index:latest
```

**Create a subscription:**
```sh
apiVersion: operators.coreos.com/v1alpha1
kind: Subscription
  name: epg-config-operator
  namespace: openshift-operators
spec:
  channel: latest
  installPlanApproval: Automatic
  name: epg-config-operator
  source: custom-catalog-source
  sourceNamespace: openshift-marketplace
```

#### To Uninstall (alt 2)
**Delete the instances from the cluster - UI:**

1. Navigate to the Operators → Installed Operators page.
2. Scroll or enter a keyword into the Filter by name field to find the Operator that you want to remove. Then, click on it.
3. On the right side of the Operator Details page, select Uninstall Operator from the Actions list.
4. An Uninstall Operator? dialog box is displayed.
5. Select Uninstall to remove the Operator, Operator deployments, and pods. Following this action, the Operator stops running and no longer receives updates.

**Delete the instances (CRs) from the cluster - CLI:**

```sh
oc get subscription epg-config-operator -n openshift-operators -o yaml | grep currentCSV
kubectl delete subscription -n openshift-operators epg-config-operator
oc delete clusterserviceversion epg-config-operator.v<VERSION> -n openshift-operators
```

## License

Copyright 2024.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
