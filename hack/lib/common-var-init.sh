#!/usr/bin/env bash

# Copyright 2020 Authors of Arktos.
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#     http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.

# --------------------------------------------------------------------------------------------
# Start of Common environment variables used in arktos-up.sh& arktos-apiserver-partition.sh
# --------------------------------------------------------------------------------------------
APISERVERS_EXTRA=${APISERVERS_EXTRA:-""}
if [[ ${APISERVERS_EXTRA} != "" ]]; then
    APISERVERS_EXTRA=${APISERVERS_EXTRA/:/\",\"}
fi

PARTITION_CONFIG_DIR=${PARTITION_CONFIG_DIR:-"/var/run/kubernetes/"}

VIRTLET_METADATA_DIR=${VIRTLET_METADATA_DIR:-"/var/lib/virtlet"}
VIRTLET_LOG_DIR=${VIRTLET_LOG_DIR:-"/var/log/virtlet"}
VIRTLET_DEPLOYMENT_FILES_DIR=${VIRTLET_DEPLOYMENT_FILES_DIR:-"/tmp"}

# Default to current arktos-vm-runtime release-0.5 branch
VIRTLET_DEPLOYMENT_FILES_SRC=${VIRTLET_DEPLOYMENT_FILES_SRC:-"https://raw.githubusercontent.com/futurewei-cloud/arktos-vm-runtime/release-0.6/deploy"}
OVERWRITE_DEPLOYMENT_FILES=${OVERWRITE_DEPLOYMENT_FILES:-"true"}

APPARMOR_ENABLED=${APPARMOR_ENABLED:-""}

# containerd socket file path
export CONTAINERD_SOCK_PATH="/run/containerd/containerd.sock"
export VIRTLET_SOCK_PATH="/run/virtlet.sock"

# This is required by virtlet deamonset installation
export ALLOW_PRIVILEGED=true

# onebox has option to choose cni plugin: mizar(default), bridge, alktron(neutron integration)
CNIPLUGIN=${CNIPLUGIN:-"mizar"}

# This command builds and runs a local kubernetes cluster.
# You may need to run this as root to allow kubelet to open docker's socket,
# and to write the test CA in /var/run/kubernetes.
DOCKER_OPTS=${DOCKER_OPTS:-""}
export DOCKER=(docker "${DOCKER_OPTS[@]}")
DOCKER_ROOT=${DOCKER_ROOT:-""}
ALLOW_PRIVILEGED=${ALLOW_PRIVILEGED:-""}
DENY_SECURITY_CONTEXT_ADMISSION=${DENY_SECURITY_CONTEXT_ADMISSION:-""}
PSP_ADMISSION=${PSP_ADMISSION:-""}
NODE_ADMISSION=${NODE_ADMISSION:-""}
RUNTIME_CONFIG=${RUNTIME_CONFIG:-""}
KUBELET_AUTHORIZATION_WEBHOOK=${KUBELET_AUTHORIZATION_WEBHOOK:-""}
KUBELET_AUTHENTICATION_WEBHOOK=${KUBELET_AUTHENTICATION_WEBHOOK:-""}
POD_MANIFEST_PATH=${POD_MANIFEST_PATH:-"/var/run/kubernetes/static-pods"}
KUBELET_FLAGS=${KUBELET_FLAGS:-""}
KUBELET_IMAGE=${KUBELET_IMAGE:-""}
# many dev environments run with swap on, so we don't fail in this env
FAIL_SWAP_ON=${FAIL_SWAP_ON:-"false"}
# Name of the network plugin, eg: "kubenet"
NET_PLUGIN=${NET_PLUGIN:-""}
# Place the config files and binaries required by NET_PLUGIN in these directory,
# eg: "/etc/cni/net.d" for config files, and "/opt/cni/bin" for binaries.
CNI_CONF_DIR=${CNI_CONF_DIR:-""}
CNI_BIN_DIR=${CNI_BIN_DIR:-""}
SERVICE_CLUSTER_IP_RANGE=${SERVICE_CLUSTER_IP_RANGE:-10.0.0.0/24}
FIRST_SERVICE_CLUSTER_IP=${FIRST_SERVICE_CLUSTER_IP:-10.0.0.1}
# if enabled, must set CGROUP_ROOT
CGROUPS_PER_QOS=${CGROUPS_PER_QOS:-true}
# name of the cgroup driver, i.e. cgroupfs or systemd
CGROUP_DRIVER=${CGROUP_DRIVER:-""}
# if cgroups per qos is enabled, optionally change cgroup root
CGROUP_ROOT=${CGROUP_ROOT:-""}
# owner of client certs, default to current user if not specified
USER=${USER:-$(whoami)}

# enables testing eviction scenarios locally.
EVICTION_HARD=${EVICTION_HARD:-"memory.available<100Mi,nodefs.available<10%,nodefs.inodesFree<5%"}
EVICTION_SOFT=${EVICTION_SOFT:-""}
EVICTION_PRESSURE_TRANSITION_PERIOD=${EVICTION_PRESSURE_TRANSITION_PERIOD:-"1m"}

# ensures all places of cert/node naming related to hostname are lower-cased for consistency
lohostname=$(hostname | tr '[:upper:]' '[:lower:]')
hostip=$(hostname -i)

# This script uses docker0 (or whatever container bridge docker is currently using)
# and we don't know the IP of the DNS pod to pass in as --cluster-dns.
# To set this up by hand, set this flag and change DNS_SERVER_IP.
# Note also that you need API_HOST (defined above) for correct DNS.
KUBE_PROXY_MODE=${KUBE_PROXY_MODE:-""}
ENABLE_CLUSTER_DNS=${KUBE_ENABLE_CLUSTER_DNS:-true}
ENABLE_NODELOCAL_DNS=${KUBE_ENABLE_NODELOCAL_DNS:-false}
DNS_SERVER_IP=${KUBE_DNS_SERVER_IP:-10.0.0.10}
LOCAL_DNS_IP=${KUBE_LOCAL_DNS_IP:-169.254.20.10}
DNS_MEMORY_LIMIT=${KUBE_DNS_MEMORY_LIMIT:-170Mi}
DNS_DOMAIN=${KUBE_DNS_NAME:-"cluster.local"}
KUBECTL=${KUBECTL:-"${KUBE_ROOT}/cluster/kubectl.sh"}
WAIT_FOR_URL_API_SERVER=${WAIT_FOR_URL_API_SERVER:-60}
MAX_TIME_FOR_URL_API_SERVER=${MAX_TIME_FOR_URL_API_SERVER:-1}
ENABLE_DAEMON=${ENABLE_DAEMON:-false}
HOSTNAME_OVERRIDE=${HOSTNAME_OVERRIDE:-"${lohostname}"}
EXTERNAL_CLOUD_PROVIDER=${EXTERNAL_CLOUD_PROVIDER:-false}
EXTERNAL_CLOUD_PROVIDER_BINARY=${EXTERNAL_CLOUD_PROVIDER_BINARY:-""}
CLOUD_PROVIDER=${CLOUD_PROVIDER:-""}
CLOUD_CONFIG=${CLOUD_CONFIG:-""}
FEATURE_GATES=${FEATURE_GATES:-"AllAlpha=false"}
STORAGE_BACKEND=${STORAGE_BACKEND:-"etcd3"}
STORAGE_MEDIA_TYPE=${STORAGE_MEDIA_TYPE:-""}
# preserve etcd data. you also need to set ETCD_DIR.
PRESERVE_ETCD="${PRESERVE_ETCD:-false}"
# enable Pod priority and preemption
ENABLE_POD_PRIORITY_PREEMPTION=${ENABLE_POD_PRIORITY_PREEMPTION:-""}

# enable kubernetes dashboard
ENABLE_CLUSTER_DASHBOARD=${KUBE_ENABLE_CLUSTER_DASHBOARD:-false}

# RBAC Mode options
AUTHORIZATION_MODE=${AUTHORIZATION_MODE:-"Node,RBAC"}
KUBECONFIG_TOKEN=${KUBECONFIG_TOKEN:-""}
AUTH_ARGS=${AUTH_ARGS:-""}

# cloud fabric controller manager config
WORKLOAD_CONTROLLER_CONFIG_PATH=${WORKLOAD_CONTROLLER_CONFIG_PATH:-"/var/run/kubernetes/controllerconfig.json"}

# Install a default storage class (enabled by default)
DEFAULT_STORAGE_CLASS=${KUBE_DEFAULT_STORAGE_CLASS:-true}

# Do not run the mutation detector by default on a local cluster.
# It is intended for a specific type of testing and inherently leaks memory.
KUBE_CACHE_MUTATION_DETECTOR="${KUBE_CACHE_MUTATION_DETECTOR:-false}"
export KUBE_CACHE_MUTATION_DETECTOR

# panic the server on watch decode errors since they are considered coder mistakes
KUBE_PANIC_WATCH_DECODE_ERROR="${KUBE_PANIC_WATCH_DECODE_ERROR:-true}"
export KUBE_PANIC_WATCH_DECODE_ERROR

# Default list of admission Controllers to invoke prior to persisting objects in cluster
# The order defined here does not matter.
if [[ "${ENABLE_POD_VERTICAL_SCALING:-false}" == "true" ]]; then
  ENABLE_ADMISSION_PLUGINS=${ENABLE_ADMISSION_PLUGINS:-"NamespaceLifecycle,LimitRanger,ServiceAccount,DefaultStorageClass,DefaultTolerationSeconds,NodeRestriction,PodResourceAllocation,MutatingAdmissionWebhook,ValidatingAdmissionWebhook,ResourceQuota,TenantExists"}
else
  ENABLE_ADMISSION_PLUGINS=${ENABLE_ADMISSION_PLUGINS:-"NamespaceLifecycle,LimitRanger,ServiceAccount,DefaultStorageClass,DefaultTolerationSeconds,MutatingAdmissionWebhook,ValidatingAdmissionWebhook,ResourceQuota,TenantExists"}
fi
DISABLE_ADMISSION_PLUGINS=${DISABLE_ADMISSION_PLUGINS:-""}
ADMISSION_CONTROL_CONFIG_FILE=${ADMISSION_CONTROL_CONFIG_FILE:-""}

# START_MODE can be 'all', 'kubeletonly', 'nokubelet', or 'nokubeproxy'
START_MODE=${START_MODE:-"all"}

# A list of controllers to enable
KUBE_CONTROLLERS="${KUBE_CONTROLLERS:-"*"}"

# Kube-controller-manager paramters
KUBE_CONTROLLER_MANAGER_CLUSTER_CIDR=${KUBE_CONTROLLER_MANAGER_CLUSTER_CIDR:-"10.244.0.0/16"}
KUBE_CONTROLLER_MANAGER_ALLOCATE_NODE_CIDR=${KUBE_CONTROLLER_MANAGER_ALLOCATE_NODE_CIDR:-true}

# Audit policy
AUDIT_POLICY_FILE=${AUDIT_POLICY_FILE:-""}

# Kube-apiserver instance number
APISERVER_NUMBER=${APISERVER_NUMBER:-"1"}
# Kube-apiserver service group id
APISERVER_SERVICEGROUPID=${APISERVER_SERVICEGROUPID:-"1"}

# --------------------------------------------------------------------------------------------
# End of 1st Common environment variables used in arktos-up.sh& arktos-apiserver-partition.sh
# --------------------------------------------------------------------------------------------


API_PORT=${API_PORT:-8080}
API_SECURE_PORT=${API_SECURE_PORT:-6443}

# WARNING: For DNS to work on most setups you should export API_HOST as the docker0 ip address,
API_HOST=${API_HOST:-"${lohostname}"}
API_HOST_IP=${API_HOST_IP:-"0.0.0.0"}
API_HOST_IP_EXTERNAL=${API_HOST_IP_EXTERNAL:-"${hostip}"}
ADVERTISE_ADDRESS=${ADVERTISE_ADDRESS:-""}
NODE_PORT_RANGE=${NODE_PORT_RANGE:-""}
API_BIND_ADDR=${API_BIND_ADDR:-"0.0.0.0"}
EXTERNAL_HOSTNAME=${EXTERNAL_HOSTNAME:-"${lohostname}"}

KUBELET_HOST=${KUBELET_HOST:-"127.0.0.1"}
# By default only allow CORS for requests on localhost
API_CORS_ALLOWED_ORIGINS=${API_CORS_ALLOWED_ORIGINS:-/127.0.0.1(:[0-9]+)?$,/localhost(:[0-9]+)?$}
KUBELET_PORT=${KUBELET_PORT:-10250}
LOG_LEVEL=${LOG_LEVEL:-3}
# Use to increase verbosity on particular files, e.g. LOG_SPEC=token_controller*=5,other_controller*=4
LOG_SPEC=${LOG_SPEC:-""}
LOG_DIR=${LOG_DIR:-"/tmp"}
CONTAINER_RUNTIME=${CONTAINER_RUNTIME:-"remote"}
CONTAINER_RUNTIME_ENDPOINT=${CONTAINER_RUNTIME_ENDPOINT:-"containerRuntime,container,${CONTAINERD_SOCK_PATH};vmRuntime,vm,${VIRTLET_SOCK_PATH}"}

RUNTIME_REQUEST_TIMEOUT=${RUNTIME_REQUEST_TIMEOUT:-"2m"}
IMAGE_SERVICE_ENDPOINT=${IMAGE_SERVICE_ENDPOINT:-""}
CHAOS_CHANCE=${CHAOS_CHANCE:-0.0}
CPU_CFS_QUOTA=${CPU_CFS_QUOTA:-true}
ENABLE_HOSTPATH_PROVISIONER=${ENABLE_HOSTPATH_PROVISIONER:-"false"}
CLAIM_BINDER_SYNC_PERIOD=${CLAIM_BINDER_SYNC_PERIOD:-"15s"} # current k8s default
ENABLE_CONTROLLER_ATTACH_DETACH=${ENABLE_CONTROLLER_ATTACH_DETACH:-"true"} # current default
# This is the default dir and filename where the apiserver will generate a self-signed cert
# which should be able to be used as the CA to verify itself
CERT_DIR=${CERT_DIR:-"/var/run/kubernetes"}
ROOT_CA_FILE=${CERT_DIR}/server-ca.crt
ROOT_CA_KEY=${CERT_DIR}/server-ca.key
CLUSTER_SIGNING_CERT_FILE=${CLUSTER_SIGNING_CERT_FILE:-"${ROOT_CA_FILE}"}
CLUSTER_SIGNING_KEY_FILE=${CLUSTER_SIGNING_KEY_FILE:-"${ROOT_CA_KEY}"}
# Reuse certs will skip generate new ca/cert files under CERT_DIR
# it's useful with PRESERVE_ETCD=true because new ca will make existed service account secrets invalided
REUSE_CERTS=${REUSE_CERTS:-false}

# --------------------------------------------------------------------------------------------
# End of 2nd Common environment variables used in arktos-up.sh& arktos-apiserver-partition.sh
# --------------------------------------------------------------------------------------------
