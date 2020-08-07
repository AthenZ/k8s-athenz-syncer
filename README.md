# k8s-athenz-syncer

K8s-athenz-syncer is a controller that synchronizes the [Athenz](https://athenz.io) domain data including the roles, services and policies
into corresponding Kubernetes [AthenzDomain](https://github.com/yahoo/k8s-athenz-istio-auth/tree/master/pkg) custom resources.

### Architecture
<p align="center">
  <img src=images/K8s-Athenz-Syncer-Controller.png>
</p>

## Table of Contents
1. [Background](#background)
2. [Install](#install)
3. [Configuration](#configuration)
4. [Usage](#usage)
5. [Contribute](#contribute)
6. [Maintenance/Contacts](#maintainers)
6. [License](#license)


## Background
Athenz is a generic RBAC provider for Kubernetes resource access management and Istio service-service
authentication and authorization. An Athenz domain contains a set of roles and policies defined by service admins. The policies can grant or deny an Athenz role with permissions to perform specific actions on services or resources. An Athenz role can comprise of a set of principals which could represent end users or other services.

Every Kubernetes namespace is mapped to an Athenz domain and the Athenz roles and policies
defined within each domain are used to express access control rules to Kubernetes resources such as deployments,
services, ingresses, etc. associated with the namespace.

***
#### Kubernetes namespace to Athenz domain name mapping
A kubernetes namespace is mapped to an Athenz domain with the following pattern:

| Kubernetes | Athenz |
| ---------- | ------ |
| **Namespace** | **Domain** |
| {user-namespace} | {user-managed-domain} |
| {system-namespace} | {special-cluster-admin-managed-domain} |
| - dashes '-' in namespace replaced with '.' in domain | |
| - double dashes '--' in namespace replaced with '-' in domain | |
| *e.g.* sports-frontend| sports.frontend |
| **Resources** | **Resources** |
| - {namespace-scoped}| {user-managed-domain-prefixed} |
| *e.g.* sports-frontend/fantasy-prod-deployment| sports.frontend:fantasy-dashboard|
| - {cluster-scoped}| {special-admin-domain-prefixed} |
| **Service** | **Service** |
| *e.g.* sports-frontend/fantasy-dashboard| sports.frontend::fantasy-dashboard|
| **RBAC** - Role and RoleBindings| Roles and Policies |
| - {Role.rule} | {Policy.assertion} |
| - {RoleBinding.Subjects} | {Role} |

Note: Kubernetes system namespaces such as "kube-system", "istio-system" are mapped to an equivalent Athenz domain
with the format: `<cluster-admin-domain>.<cluster-namespace>`
***

While Athenz ZMS provides APIs to perform resource access checks against user/client credentials, caching the relevant
policies as Kubernetes resources allows any in-cluster auth provider service to perform in-memory validation of user or
client credentials without worrying about the Athenz ZMS/ZTS service availability.

K8s-athenz-syncer calls Athenz ZMS API [GetSignedDomains() API](https://github.com/yahoo/athenz/blob/master/ui/rdl-api.md#getsigneddomainsobj-functionerr-json-response--) to fetch the entire contents of an Athenz domain signed by the ZMS including roles, principals and policies with signatures and creates Kubernetes Custom Resources that store the domain data in the cluster so that applications can do the security checks based on local cached data.

The controller also runs a cron that periodically fetches the list of Athenz domains that were modified during the cron
interval and then fetches the signed contents for each domain and stores them as the AthenzDomain Custom Resource in the cluster in order to keep all policies in local cache updated. There is also a full resync cron that adds all the watched namespaces to the controller work queue so that all of Kubernetes AthenzDomains Custom Resources are resynced after a full resync interval.

#### Example AthenzDomain CR
```
apiVersion: v1
items:
- apiVersion: athenz.io/v1
  kind: AthenzDomain
  metadata:
    creationTimestamp: 2019-01-01T00:00:00Z
    generation:
    name: home.test
    namespace: home-test
    resourceVersion:
    selfLink:
    uid:
  spec:
    domains:
      domain:
        name: home.test
        ypmId: 0
        roles:
          name: home.test:role.admin
        modified: '2019-01-01T00:00:00.000Z'
        members:
          user.tester
        roleMembers:
          memberName: user.tester
        policies:
        contents:
            domain: home.test
            policies:
              name: home.test:policy.admin
            modified: '2019-01-01T00:00:00.000Z'
            assertions:
              role: home.test:role.admin
                resource: home.test:*
                action: "*"
                effect: ALLOW
        signature:
        keyId: xyz
        services: []
        entities:
        modified: '2019-01-01T00:00:00.000Z'
    signature:
    keyId: xyz
```

## Install
#### Prerequisite
There are a variety of prerequisites required in order to run this controller, they are specified below.
- **Kubernetes cluster** - A running Kubernetes cluster is required to run the controller. More information on how to setup a cluster can be found in the official documentation [here](https://kubernetes.io/docs/setup/). This controller was developed and tested with the 1.13 release.
- **Athenz** - Athenz is required for the controller to fetch policy and domain data. More information and setup steps can be found [here](http://www.athenz.io/). The authorization management service (ZMS) and its apis are primarily used for this controller.
- **SIA Provider** - A service identity agent (SIA) must be running in the Kubernetes cluster in order to provision X.509 certificates to instances in order to authenticate with Athenz. The approach we currently use in production can be found [here](https://github.com/yahoo/k8s-athenz-identity).

### Setup
Configuration files which must be applied to run k8s-athenz-syncer which can be found in the k8s directory.

#### Athenz Domain Custom Resource Definition
The Athenz Domain custom resource definition must be first created in order for the controller to sync the custom resource. Run the following command:
```
kubectl apply -f k8s/athenzdomain.yaml
```

#### Service Account
In order to tell SIA which service to provide an X.509 certificate to, a service account must be present. This is required for the controller to authenticate with ZMS for api calls. Run the following command:
```
kubectl apply -f k8s/serviceaccount.yaml
```
or
```
kubectl create serviceaccount k8s-athenz-syncer
```

#### ClusterRole and ClusterRoleBinding
This controller requires RBAC to create, update, delete, watch and list all Athenzdomains Custom Resources in the cluster. It also has a watch and list on namespaces in order to know which domains to look up from Athenz.

**NOTE:** If you are deploying to a non-default namespace, make sure to update the `k8s/clusterrolebinding.yaml` subject namespace accordingly.
```
kubectl apply -f k8s/clusterrole.yaml
kubectl apply -f k8s/clusterrolebinding.yaml
```

#### Deployment
The deployment for the controller contains three containers: sia init, sia refresh, and the controller itself. Build a docker image using the Dockerfile and publish to a docker registry. Make sure to replace the docker images inside of this spec to the ones which are published in your organization. Also, replace the zms url with your instance. Run the following command in order to deploy:
```
kubectl apply -f k8s/deployment.yaml
```

## Configuration
K8s-athenz-syncer has a variety of parameters that can be configured, they are given below.

**Parameters**
```
- cert (default: /var/run/athenz/service.cert.pem): path to X.509 certificate file to use for zms authentication
- key (default: /var/run/athenz/service.key.pem): path to private key file for zms authentication
- zms-url (default: https://zms.url.com): athenz full zms url including api path
- update-cron (default: 1m0s): sleep interval for controller update cron
- resync-cron (default: 1h0m0s) sleep interval for controller full resync cron
- queue-delay-interval (default: 250ms) delay interval time for workqueue
- admin-domain (default: "") admin domain that can be specified in order to fetch admin domains from Athenz
- system-namespaces (default: "") a list of cluster system namespaces that you hope the controller to fetch from Athenz
- disable-keep-alives (default: true) disable keep alive for zms client
```

## Usage
Once the controller is up and running, the controller will create Kubernetes AthenzDomains Custom Resources in the cluster accordingly. Users and Applications can consume those AthenzDomains CR to get security policy information for access control checks.
1. To see all the AthenzDomains CR created, run `kubectl get athenzdomains --all-namespaces`
2. In order to use AthenzDomains CR in applications, create AthenzDomains clientset and informers to retrieve the resources.

## Contribute
Please refer to the [contributing](Contributing.md) file for information about how to get involved. We welcome issues, questions, and pull requests.

## Maintainers
Core Team : omega-core@verizonmedia.com

## License
Copyright 2019 Oath Inc. Licensed under the Apache License, Version 2.0 (the "License")
