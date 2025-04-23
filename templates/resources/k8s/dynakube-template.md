```yaml
# DynaKube Custom Resource Template for Dynatrace Operator
# This template includes all available parameters commented out
# Please uncomment and modify the parameters you need for your deployment

apiVersion: dynatrace.com/v1beta2
kind: DynaKube
metadata:
  name: dynakube
  namespace: dynatrace
  # Optional: Feature flags as annotations
  # annotations:
  #   feature.dynatrace.com/automatic-injection: "true"  # Enable/disable automatic injection for monitored namespaces
  #   feature.dynatrace.com/activegate-ignore-proxy: "false"  # Prevent proxy propagation to ActiveGate
  #   feature.dynatrace.com/label-version-detection: "false"  # Enable/disable build label propagation
  #   feature.dynatrace.com/injection-failure-policy: "silent"  # silent or fail
  #   feature.dynatrace.com/initcontainer-seccomp: "true"  # Enable seccomp profile for init container
  #   feature.dynatrace.com/proxy-exclude-list: "host1,host2"  # List of URLs to exclude from proxy config
spec:
  # REQUIRED: Dynatrace apiUrl including the `/api` path at the end
  # For SaaS, set `ENVIRONMENTID` to your environment ID
  # For Managed, change the apiUrl address
  apiUrl: https://ENVIRONMENTID.live.dynatrace.com/api

  # Optional: Name of the secret holding the credentials required to connect to the Dynatrace tenant
  # If unset, the name of this custom resource is used
  # tokens: ""

  # Optional: Defines a custom pull secret for private registry when pulling Dynatrace images
  # The secret must be of type 'kubernetes.io/dockerconfigjson'
  # customPullSecret: "custom-pull-secret"

  # Optional: Disable certificate validation checks for installer download and API communication
  # skipCertCheck: false

  # Optional: Set custom proxy settings either directly or from a secret with the field 'proxy'
  # proxy:
  #   value: "my-proxy-url.com"
  #   valueFrom: "name-of-my-proxy-secret"

  # Optional: Adds custom RootCAs from a configmap
  # Put the certificate under "certs" key within your configmap
  # Applies to Dynatrace Operator, OneAgent and ActiveGate
  # trustedCAs: "name-of-my-ca-configmap"

  # Optional: Sets Network Zone for OneAgent and ActiveGate pods
  # networkZone: "name-of-my-network-zone"

  # Optional: If enabled, and if Istio is installed on the Kubernetes environment,
  # creates VirtualService and ServiceEntry objects to allow access to the Dynatrace cluster
  # enableIstio: false

  # Optional: Configuration for thresholding Dynatrace API requests in minutes
  # dynatraceApiRequestThreshold: 15

  # Optional: Configuration for Metadata Enrichment
  # metadataEnrichment:
  #   # Enables or disables metadata enrichment
  #   enabled: true
  #   # The namespaces in which metadata enrichment should be injected
  #   # If unset, all namespaces will be injected
  #   namespaceSelector:
  #     matchLabels:
  #       app: my-app
  #     matchExpressions:
  #      - key: app
  #        operator: In
  #        values: [my-frontend, my-backend, my-database]

  # Configuration for OneAgent instances
  # oneAgent:
    # Optional: Sets a host group for OneAgent
    # hostGroup: ""

    # Choose ONE of the following monitoring modes:
    # 1. Classic Full Stack Monitoring
    # classicFullStack:
    #   # Optional: If specified, indicates the OneAgent version to use
    #   # version: ""
    #
    #   # Optional: Tolerations to include with the OneAgent DaemonSet
    #   # tolerations:
    #   # - effect: NoSchedule
    #   #   key: node-role.kubernetes.io/master
    #   #   operator: Exists
    #
    #   # Optional: Optional: Node selector to control on which nodes OneAgent will be deployed
    #   # nodeSelector: {}
    #
    #   # Optional: Specify priority class for OneAgent pods
    #   # priorityClassName: ""
    #
    #   # Optional: Define resource requests and limits for OneAgent pods
    #   # resources:
    #   #   requests:
    #   #     cpu: 100m
    #   #     memory: 512Mi
    #   #   limits:
    #   #     cpu: 300m
    #   #     memory: 1.5Gi
    #
    #   # Optional: Automatically update OneAgent pods when a new version is available
    #   # autoUpdate: true
    #
    #   # Optional: Sets the DNS Policy for OneAgent pods
    #   # dnsPolicy: "ClusterFirstWithHostNet"
    #
    #   # Optional: Adds custom annotations to OneAgent pods
    #   # annotations:
    #   #   custom: annotation
    #
    #   # Optional: Adds custom labels to OneAgent pods
    #   # labels:
    #   #   custom: label
    #
    #   # Optional: Sets the URI for the OneAgent installer image
    #   # image: ""
    #
    #   # Optional: The SecComp Profile for OneAgent secure computing mode
    #   # secCompProfile: ""
    #
    #   # Optional: Kubernetes objects that should be skipped for monitoring
    #   # skipCrds: false

    # 2. Cloud Native Full Stack Monitoring
    # cloudNativeFullStack:
    #   # Optional: If specified, indicates the OneAgent version to use
    #   # version: ""
    #
    #   # Optional: Tolerations to include with the OneAgent DaemonSet
    #   # tolerations:
    #   # - effect: NoSchedule
    #   #   key: node-role.kubernetes.io/master
    #   #   operator: Exists
    #
    #   # Optional: Node selector to control on which nodes OneAgent will be deployed
    #   # nodeSelector: {}
    #
    #   # Optional: Specify priority class for OneAgent pods
    #   # priorityClassName: ""
    #
    #   # Optional: Define resource requests and limits for OneAgent pods
    #   # resources:
    #   #   requests:
    #   #     cpu: 100m
    #   #     memory: 512Mi
    #   #   limits:
    #   #     cpu: 300m
    #   #     memory: 1.5Gi
    #
    #   # Optional: Automatically update OneAgent pods when a new version is available
    #   # autoUpdate: true
    #
    #   # Optional: Sets the DNS Policy for OneAgent pods
    #   # dnsPolicy: "ClusterFirstWithHostNet"
    #
    #   # Optional: Adds custom annotations to OneAgent pods
    #   # annotations:
    #   #   custom: annotation
    #
    #   # Optional: Adds custom labels to OneAgent pods
    #   # labels:
    #   #   custom: label
    #
    #   # Optional: Sets the URI for the OneAgent installer image
    #   # image: ""
    #
    #   # Optional: The URI of the image containing the codemodules
    #   # codeModulesImage: ""
    #
    #   # Optional: Defines resources requests and limits for the initContainer
    #   # initResources:
    #   #   requests:
    #   #     cpu: 30m
    #   #     memory: 30Mi
    #   #   limits:
    #   #     cpu: 100m
    #   #     memory: 60Mi
    #
    #   # Optional: The SecComp Profile for secure computing mode
    #   # secCompProfile: ""

    # 3. Application Monitoring Only
    # applicationMonitoring:
    #   # The namespaces which should be monitored
    #   # If unset, all namespaces will be monitored
    #   # namespaceSelector:
    #   #   matchLabels:
    #   #     app: my-app
    #   #   matchExpressions:
    #   #    - key: app
    #   #      operator: In
    #   #      values: [my-frontend, my-backend, my-database]
    #
    #   # Optional: If specified, indicates the OneAgent version to use
    #   # version: ""
    #
    #   # Optional: The URI of the image containing the codemodules
    #   # codeModulesImage: ""
    #
    #   # Optional: Defines resources requests and limits for the initContainer
    #   # initResources:
    #   #   requests:
    #   #     cpu: 30m
    #   #     memory: 30Mi
    #   #   limits:
    #   #     cpu: 100m
    #   #     memory: 60Mi

    # 4. Host Monitoring Only
    # hostMonitoring:
    #   # Optional: Tolerations to include with the OneAgent DaemonSet
    #   # tolerations:
    #   # - effect: NoSchedule
    #   #   key: node-role.kubernetes.io/master
    #   #   operator: Exists
    #
    #   # Optional: Node selector to control on which nodes OneAgent will be deployed
    #   # nodeSelector: {}
    #
    #   # Optional: Specify priority class for OneAgent pods
    #   # priorityClassName: ""
    #
    #   # Optional: Define resource requests and limits for OneAgent pods
    #   # resources:
    #   #   requests:
    #   #     cpu: 100m
    #   #     memory: 512Mi
    #   #   limits:
    #   #     cpu: 300m
    #   #     memory: 1.5Gi
    #
    #   # Optional: Automatically update OneAgent pods when a new version is available
    #   # autoUpdate: true
    #
    #   # Optional: Sets the DNS Policy for OneAgent pods
    #   # dnsPolicy: "ClusterFirstWithHostNet"
    #
    #   # Optional: Adds custom annotations to OneAgent pods
    #   # annotations:
    #   #   custom: annotation
    #
    #   # Optional: Adds custom labels to OneAgent pods
    #   # labels:
    #   #   custom: label
    #
    #   # Optional: Sets the URI for the OneAgent installer image
    #   # image: ""
    #
    #   # Optional: If specified, indicates the OneAgent version to use
    #   # version: ""

  # Optional: Configuration for ActiveGate instances
  # activeGate:
    # Specifies which capabilities will be enabled on ActiveGate instances
    # The following capabilities can be set:
    # - routing (enables OneAgent routing)
    # - kubernetes-monitoring (enables Kubernetes API monitoring)
    # - metrics-ingest (opens metrics ingest endpoint)
    # - dynatrace-api (enables calling Dynatrace API via ActiveGate)
    # capabilities:
    #   - routing
    #   - kubernetes-monitoring
    #   - dynatrace-api

    # Optional: Sets the image used to deploy ActiveGate instances
    # image: ""

    # Optional: Sets how many ActiveGate pods are spawned by the StatefulSet
    # replicas: 1

    # Optional: Recommended: Sets the activation group for ActiveGate instances
    # group: ""

    # Optional: Defines a custom properties file for the ActiveGate
    # customProperties:
    #   value: |
    #     [connectivity]
    #     networkZone=
    #   valueFrom: "myCustomPropertiesSecret"

    # Optional: Specifies resource settings for ActiveGate instances
    # resources:
    #   requests:
    #     cpu: 500m
    #     memory: 512Mi
    #   limits:
    #     cpu: 1000m
    #     memory: 1.5Gi

    # Optional: Sets a node selector to control on which nodes ActiveGate will be deployed
    # nodeSelector: {}

    # Optional: Specifies tolerations for the ActiveGate StatefulSet
    # tolerations:
    # - effect: NoSchedule
    #   key: node-role.kubernetes.io/master
    #   operator: Exists

    # Optional: Adds custom annotations to ActiveGate pods
    # annotations:
    #   custom: annotation

    # Optional: Adds custom labels to ActiveGate pods
    # labels:
    #   custom: label

    # Optional: Set additional environment variables for ActiveGate pods
    # env:
    # - name: MY_ENV_VAR
    #   value: my-value
```
