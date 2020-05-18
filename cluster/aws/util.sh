#!/bin/bash

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

# A library of helper functions and constant for the local config.

# Experimental flags can be removed/renamed at any time.
# The intent is to allow experimentation/advanced functionality before we
# are ready to commit to supporting it.
# Experimental functionality:
#   KUBE_USE_EXISTING_MASTER=true
#     Detect and reuse an existing master; useful if you want to
#     create more nodes, perhaps with a different instance type or in
#     a different subnet/AZ
#   KUBE_SUBNET_CIDR=172.20.1.0/24
#     Override the default subnet CIDR; useful if you want to create
#     a second subnet.  The default subnet is 172.20.0.0/24.  The VPC
#     is created with 172.20.0.0/16; you must pick a sub-CIDR of that.

# Use the config file specified in $KUBE_CONFIG_FILE, or default to
# config-default.sh.
KUBE_ROOT=$(dirname "${BASH_SOURCE}")/../..
source "${KUBE_ROOT}/cluster/aws/${KUBE_CONFIG_FILE-"config-default.sh"}"
source "${KUBE_ROOT}/cluster/common.sh"
source "${KUBE_ROOT}/cluster/lib/util.sh"

SSH_USER=ubuntu

ALLOCATE_NODE_CIDRS=true

NODE_INSTANCE_PREFIX="${INSTANCE_PREFIX}-minion"

# The Auto Scaling Group (ASG) name must be unique, so we include the zone
ASG_NAME="${NODE_INSTANCE_PREFIX}-group-${ZONE}"

# We could allow the master disk volume id to be specified in future
MASTER_DISK_ID=

# Well known tags
TAG_KEY_MASTER_IP="kubernetes.io/master-ip"

OS_DISTRIBUTION=${KUBE_OS_DISTRIBUTION}

# Defaults: ubuntu -> xenial
if [[ "${OS_DISTRIBUTION}" == "ubuntu" ]]; then
  OS_DISTRIBUTION=xenial
fi

# Loads the distro-specific utils script.
# If the distro is not recommended, prints warnings or exits.
function load_distro_utils () {
case "${OS_DISTRIBUTION}" in
  xenial)
    ;;
  vivid)
    echo "vivid is no longer supported by kube-up; please use xenial instead" >&2
    exit 2
    ;;
  coreos)
    echo "coreos is no longer supported by kube-up; please use xenial instead" >&2
    exit 2
    ;;
  trusty)
    echo "trusty is no longer supported by kube-up; please use xenial instead" >&2
    exit 2
    ;;
  wheezy)
    echo "wheezy is no longer supported by kube-up; please use xenial instead" >&2
    exit 2
    ;;
  *)
    echo "Cannot start cluster using os distro: ${OS_DISTRIBUTION}" >&2
    echo "The current recommended distro is xenial" >&2
    exit 2
    ;;
esac

source "${KUBE_ROOT}/cluster/aws/${OS_DISTRIBUTION}/util.sh"
}

load_distro_utils

# This removes the final character in bash (somehow)
re='[a-zA-Z]'
if [[ ${ZONE: -1} =~ $re  ]]; then
  AWS_REGION=${ZONE%?}
else
  AWS_REGION=$ZONE
fi

export AWS_DEFAULT_REGION=${AWS_REGION}
export AWS_DEFAULT_OUTPUT=text
AWS_CMD="aws ec2"
AWS_ASG_CMD="aws autoscaling"

function get_subnet_cidr() {
  local subnet_id=$1
  KUBE_SUBNET_CIDR=$($AWS_CMD describe-subnets --subnet-ids=$subnet_id --query "Subnets[0].CidrBlock")
  KUBE_VPC_CIDR_BASE=$(echo ${KUBE_SUBNET_CIDR} | sed 's/\.*.$//'|sed 's/\.*.$//')
}

if [[ -z "${KUBE_SUBNET_CIDR:-}" ]]; then
  if [[ "${PRESET_INSTANCES_ENABLED:-}" == "yes" ]]; then
    KUBE_SUBNET_CIDR=$PRESET_KUBE_SUBNET_CIDR
    KUBE_VPC_CIDR_BASE=$PRESET_KUBE_VPC_CIDR_BASE
  else
    get_subnet_cidr $SUBNET_ID
  fi  
fi

VPC_CIDR_BASE=${KUBE_VPC_CIDR_BASE:-172.20}
MASTER_IP_SUFFIX=.9
VPC_CIDR=${VPC_CIDR_BASE}.0.0/16
SUBNET_CIDR=${VPC_CIDR_BASE}.0.0/24

if [[ -n "${KUBE_SUBNET_CIDR:-}" ]]; then
  SUBNET_CIDR=${KUBE_SUBNET_CIDR}
fi
if [[ -z "${MASTER_INTERNAL_IP:-}" ]]; then
  MASTER_INTERNAL_IP="${SUBNET_CIDR%.*}${MASTER_IP_SUFFIX}"
fi

MASTER_SG_NAME="kubernetes-master-${CLUSTER_ID}"
NODE_SG_NAME="kubernetes-minion-${CLUSTER_ID}"

IAM_PROFILE_MASTER="kubernetes-master-${CLUSTER_ID}-${VPC_NAME}"
IAM_PROFILE_NODE="kubernetes-minion-${CLUSTER_ID}-${VPC_NAME}"

# Be sure to map all the ephemeral drives.  We can specify more than we actually have.
# TODO: Actually mount the correct number (especially if we have more), though this is non-trivial, and
#  only affects the big storage instance types, which aren't a typical use case right now.
EPHEMERAL_BLOCK_DEVICE_MAPPINGS=",{\"DeviceName\": \"/dev/sdc\",\"VirtualName\":\"ephemeral0\"},{\"DeviceName\": \"/dev/sdd\",\"VirtualName\":\"ephemeral1\"},{\"DeviceName\": \"/dev/sde\",\"VirtualName\":\"ephemeral2\"},{\"DeviceName\": \"/dev/sdf\",\"VirtualName\":\"ephemeral3\"}"

# Experimental: If the user sets KUBE_AWS_STORAGE to ebs, use ebs storage
# in preference to local instance storage We do this by not mounting any
# instance storage.  We could do this better in future (e.g. making instance
# storage available for other purposes)
if [[ "${KUBE_AWS_STORAGE:-}" == "ebs" ]]; then
  EPHEMERAL_BLOCK_DEVICE_MAPPINGS=""
fi

# TODO (bburns) Parameterize this for multiple cluster per project
function get_vpc_id {
  $AWS_CMD describe-vpcs \
           --filters Name=tag:Name,Values=${VPC_NAME} \
                     Name=tag:KubernetesCluster,Values=${CLUSTER_ID} \
           --query Vpcs[].VpcId
}

function get_subnet_id {
  local vpc_id=$1
  local az=$2
  $AWS_CMD describe-subnets \
           --filters Name=tag:KubernetesCluster,Values=${CLUSTER_ID} \
                     Name=availabilityZone,Values=${az} \
                     Name=vpc-id,Values=${vpc_id} \
           --query Subnets[].SubnetId
}

function get_igw_id {
  local vpc_id=$1
  $AWS_CMD describe-internet-gateways \
           --filters Name=attachment.vpc-id,Values=${vpc_id} \
           --query InternetGateways[].InternetGatewayId
}

function get_elbs_in_vpc {
  # ELB doesn't seem to be on the same platform as the rest of AWS; doesn't support filtering
  aws elb --output json describe-load-balancers  | \
    python -c "import json,sys; lst = [str(lb['LoadBalancerName']) for lb in json.load(sys.stdin)['LoadBalancerDescriptions'] if 'VPCId' in lb and lb['VPCId'] == '$1']; print('\n'.join(lst))"
}

function get_instanceid_from_name {
  local tagName=$1
  $AWS_CMD describe-instances \
    --filters Name=tag:Name,Values=${tagName} \
              Name=instance-state-name,Values=running \
              Name=tag:KubernetesCluster,Values=${CLUSTER_ID} \
    --query Reservations[].Instances[].InstanceId
}

function get_instance_public_ip {
  local instance_id=$1
  $AWS_CMD describe-instances \
    --instance-ids ${instance_id} \
    --query Reservations[].Instances[].NetworkInterfaces[0].Association.PublicIp
}

function get_instance_private_ip {
  local instance_id=$1
  $AWS_CMD describe-instances \
    --instance-ids ${instance_id} \
    --query Reservations[].Instances[].NetworkInterfaces[0].PrivateIpAddress
}

# Gets a security group id, by name ($1)
function get_security_group_id {
  local name=$1
  $AWS_CMD describe-security-groups \
           --filters Name=vpc-id,Values=${VPC_ID} \
                     Name=group-name,Values=${name} \
                     Name=tag:KubernetesCluster,Values=${CLUSTER_ID} \
           --query SecurityGroups[].GroupId \
  | tr "\t" "\n"
}

# Finds the master ip, if it is saved (tagged on the master disk)
# Sets KUBE_MASTER_IP
function find-tagged-master-ip {
  find-master-pd
  if [[ -n "${MASTER_DISK_ID:-}" ]]; then
    KUBE_MASTER_IP=$(get-tag ${MASTER_DISK_ID} ${TAG_KEY_MASTER_IP})
  fi
}

# Gets a tag value from an AWS resource
# usage: get-tag <resource-id> <tag-name>
# outputs: the tag value, or "" if no tag
function get-tag {
  $AWS_CMD describe-tags --filters Name=resource-id,Values=${1} \
                                   Name=key,Values=${2} \
                         --query Tags[].Value
}

# Gets an existing master, exiting if not found
# Note that this is called directly by the e2e tests
function detect-master() {
  find-tagged-master-ip
  KUBE_MASTER=${MASTER_NAME}
  if [[ -z "${KUBE_MASTER_IP:-}" ]]; then
    echo "Could not detect Kubernetes master node IP.  Make sure you've launched a cluster with 'kube-up.sh'"
    exit 1
  fi
  echo "Using master: $KUBE_MASTER (external IP: $KUBE_MASTER_IP)"
}

function query-running-minions () {
  local query=$1
  $AWS_CMD describe-instances \
           --filters Name=instance-state-name,Values=running \
                     Name=vpc-id,Values=${VPC_ID} \
                     Name=tag:KubernetesCluster,Values=${CLUSTER_ID} \
                     Name=tag:aws:autoscaling:groupName,Values=${ASG_NAME} \
                     Name=tag:Role,Values=${NODE_TAG} \
           --query ${query}
}

function detect-node-names () {
  # If this is called directly, VPC_ID might not be set
  # (this is case from cluster/log-dump/log-dump.sh)
  if [[ -z "${VPC_ID:-}" ]]; then
    VPC_ID=$(get_vpc_id)
  fi

  NODE_IDS=()
  NODE_NAMES=()
  for id in $(query-running-minions "Reservations[].Instances[].InstanceId"); do
    NODE_IDS+=("${id}")

    # We use the minion ids as the name
    NODE_NAMES+=("${id}")
  done
}

# Called to detect the project on GCE
# Not needed on AWS
function detect-project() {
  :
}

function detect-nodes () {
  detect-node-names

  # This is inefficient, but we want NODE_NAMES / NODE_IDS to be ordered the same as KUBE_NODE_IP_ADDRESSES
  KUBE_NODE_IP_ADDRESSES=()
  for (( i=0; i<${#NODE_NAMES[@]}; i++)); do
    local minion_ip
    if [[ "${ENABLE_NODE_PUBLIC_IP}" == "true" ]]; then
      minion_ip=$(get_instance_public_ip ${NODE_NAMES[$i]})
    else
      minion_ip=$(get_instance_private_ip ${NODE_NAMES[$i]})
    fi
    echo "Found minion ${i}: ${NODE_NAMES[$i]} @ ${minion_ip}"
    KUBE_NODE_IP_ADDRESSES+=("${minion_ip}")
  done

  if [[ "${KUBE_CREATE_NODES}" == "true" ]]; then
    if [[ -z "$KUBE_NODE_IP_ADDRESSES" ]]; then
      echo "Could not detect Kubernetes minion nodes.  Make sure you've launched a cluster with 'kube-up.sh'"
      exit 1
    fi
  else
    echo "Nodes not created because KUBE_CREATE_NODES is set to false"
  fi
}

function detect-security-groups {
  if [[ -z "${MASTER_SG_ID-}" ]]; then
    MASTER_SG_ID=$(get_security_group_id "${MASTER_SG_NAME}")
    if [[ -z "${MASTER_SG_ID}" ]]; then
      echo "Could not detect Kubernetes master security group.  Make sure you've launched a cluster with 'kube-up.sh'"
      exit 1
    else
      echo "Using master security group: ${MASTER_SG_NAME} ${MASTER_SG_ID}"
    fi
  fi
  if [[ -z "${NODE_SG_ID-}" ]]; then
    NODE_SG_ID=$(get_security_group_id "${NODE_SG_NAME}")
    if [[ -z "${NODE_SG_ID}" ]]; then
      echo "Could not detect Kubernetes minion security group.  Make sure you've launched a cluster with 'kube-up.sh'"
      exit 1
    else
      echo "Using minion security group: ${NODE_SG_NAME} ${NODE_SG_ID}"
    fi
  fi
}

# Detects the AMI to use (considering the region)
# This really should be in the various distro-specific util functions,
# but CoreOS uses this for the master, so for now it is here.
#
# TODO: Remove this and just have each distro implement detect-image
#
# Vars set:
#   AWS_IMAGE
function detect-image () {
case "${OS_DISTRIBUTION}" in
  xenial)
    detect-xenial-image
    ;;
  *)
    echo "Please specify AWS_IMAGE directly (distro ${OS_DISTRIBUTION} not recognized)"
    exit 2
    ;;
esac
}

function detect-minion-image() {
  if [[ -z "${KUBE_NODE_IMAGE=-}" ]]; then
    detect-image
    KUBE_NODE_IMAGE=$AWS_IMAGE
  fi
}

function check-minion() {
  local minion_ip=$1

  local output=$(ssh -oStrictHostKeyChecking=no -i "${AWS_SSH_KEY}" ${SSH_USER}@$minion_ip sudo docker ps -a 2>/dev/null)
  if [[ -z "${output}" ]]; then
    ssh -oStrictHostKeyChecking=no -i "${AWS_SSH_KEY}" ${SSH_USER}@$minion_ip sudo service docker start > $LOG 2>&1
    echo "not working yet"
  else
    echo "working"
  fi
}

# Detects the RootDevice to use in the Block Device Mapping (considering the AMI)
#
# Vars set:
#   MASTER_BLOCK_DEVICE_MAPPINGS
#   NODE_BLOCK_DEVICE_MAPPINGS
#
function detect-root-device {
  local master_image=${AWS_IMAGE}
  local node_image=${KUBE_NODE_IMAGE}

  ROOT_DEVICE_MASTER=$($AWS_CMD describe-images --image-ids ${master_image} --query 'Images[].RootDeviceName')
  if [[ "${master_image}" == "${node_image}" ]]; then
      ROOT_DEVICE_NODE=${ROOT_DEVICE_MASTER}
    else
      ROOT_DEVICE_NODE=$($AWS_CMD describe-images --image-ids ${node_image} --query 'Images[].RootDeviceName')
  fi

  MASTER_BLOCK_DEVICE_MAPPINGS="[{\"DeviceName\":\"${ROOT_DEVICE_MASTER}\",\"Ebs\":{\"DeleteOnTermination\":true,\"VolumeSize\":${MASTER_ROOT_DISK_SIZE},\"VolumeType\":\"${MASTER_ROOT_DISK_TYPE}\"}} ${EPHEMERAL_BLOCK_DEVICE_MAPPINGS}]"
  NODE_BLOCK_DEVICE_MAPPINGS="[{\"DeviceName\":\"${ROOT_DEVICE_NODE}\",\"Ebs\":{\"DeleteOnTermination\":true,\"VolumeSize\":${NODE_ROOT_DISK_SIZE},\"VolumeType\":\"${NODE_ROOT_DISK_TYPE}\"}} ${EPHEMERAL_BLOCK_DEVICE_MAPPINGS}]"
}

# Computes the AWS fingerprint for a public key file ($1)
# $1: path to public key file
# Note that this is a different hash from the OpenSSH hash.
# But AWS gives us this public key hash in the describe keys output, so we should stick with this format.
# Hopefully this will be done by the aws cli tool one day: https://github.com/aws/aws-cli/issues/191
# NOTE: This does not work on Mavericks, due to an odd ssh-keygen version, so we use get-ssh-fingerprint instead
function get-aws-fingerprint {
  local -r pubkey_path=$1
  ssh-keygen -f ${pubkey_path} -e -m PKCS8  | openssl rsa -pubin -outform DER | openssl md5 -c | sed -e 's/(stdin)= //g'
}

# Computes the SSH fingerprint for a public key file ($1)
# #1: path to public key file
# Note this is different from the AWS fingerprint; see notes on get-aws-fingerprint
function get-ssh-fingerprint {
  local -r pubkey_path=$1
  ssh-keygen -lf ${pubkey_path} | cut -f2 -d' '
}

# Import an SSH public key to AWS.
# Ignores duplicate names; recommended to use a name that includes the public key hash.
# $1 name
# $2 public key path
function import-public-key {
  local -r name=$1
  local -r path=$2

  local ok=1
  local output=""
  output=$($AWS_CMD import-key-pair --key-name ${name} --public-key-material "fileb://${path}" 2>&1) || ok=0
  if [[ ${ok} == 0 ]]; then
    # Idempotency: ignore if duplicate name
    if [[ "${output}" != *"InvalidKeyPair.Duplicate"* ]]; then
      echo "Error importing public key"
      echo "Output: ${output}"
      exit 1
    fi
  fi
}

# Robustly try to create a security group, if it does not exist.
# $1: The name of security group; will be created if not exists
# $2: Description for security group (used if created)
#
# Note that this doesn't actually return the sgid; we need to re-query
function create-security-group {
  local -r name=$1
  local -r description=$2

  local sgid=$(get_security_group_id "${name}")
  if [[ -z "$sgid" ]]; then
	  echo "Creating security group ${name}."
	  sgid=$($AWS_CMD create-security-group --group-name "${name}" --description "${description}" --vpc-id "${VPC_ID}" --query GroupId)
	  add-tag $sgid KubernetesCluster ${CLUSTER_ID}
  fi
}

# Authorize ingress to a security group.
# Attempts to be idempotent, though we end up checking the output looking for error-strings.
# $1 group-id
# $2.. arguments to pass to authorize-security-group-ingress
function authorize-security-group-ingress {
  local -r sgid=$1
  shift
  local ok=1
  local output=""
  output=$($AWS_CMD authorize-security-group-ingress --group-id "${sgid}" $@ 2>&1) || ok=0
  if [[ ${ok} == 0 ]]; then
    # Idempotency: ignore if duplicate rule
    if [[ "${output}" != *"InvalidPermission.Duplicate"* ]]; then
      echo "Error creating security group ingress rule"
      echo "Output: ${output}"
      exit 1
    fi
  fi
}

# Gets master persistent volume, if exists
# Sets MASTER_DISK_ID
function find-master-pd {
  local name=${MASTER_NAME}-pd
  echo "find-master-pd called for name $name, Zone $ZONE, Cluster $CLUSTER_ID, MASTER_DISK_ID: $MASTER_DISK_ID"
  if [[ -z "${MASTER_DISK_ID}" ]]; then
    local zone_filter="Name=availability-zone,Values=${ZONE}"
    if [[ "${KUBE_USE_EXISTING_MASTER:-}" == "true" ]]; then
      # If we're reusing an existing master, it is likely to be in another zone
      # If running multizone, your cluster must be uniquely named across zones
      zone_filter=""
    fi
    MASTER_DISK_ID=`$AWS_CMD describe-volumes \
                             --filters ${zone_filter} \
                                       Name=tag:Name,Values=${name} \
                                       Name=tag:KubernetesCluster,Values=${CLUSTER_ID} \
                             --query Volumes[].VolumeId`
  fi
  echo "find-master-pd name $name, MASTER_DISK_ID: $MASTER_DISK_ID"
}

# Gets or creates master persistent volume
# Sets MASTER_DISK_ID
function ensure-master-pd {
  local name=${MASTER_NAME}-pd

  find-master-pd

  if [[ -z "${MASTER_DISK_ID}" ]]; then
    echo "Creating master disk: size ${MASTER_DISK_SIZE}GB, type ${MASTER_DISK_TYPE}"
    MASTER_DISK_ID=`$AWS_CMD create-volume --availability-zone ${ZONE} --volume-type ${MASTER_DISK_TYPE} --size ${MASTER_DISK_SIZE} --query VolumeId`
    add-tag ${MASTER_DISK_ID} Name ${name}
    add-tag ${MASTER_DISK_ID} KubernetesCluster ${CLUSTER_ID}
    echo "Created master disk $name with ID $MASTER_DISK_ID"
  fi
}

# Configures a CloudWatch alarm to reboot the instance on failure
function reboot-on-failure {
  local instance_id=$1

  echo "Creating Cloudwatch alarm to reboot instance ${instance_id} on failure"

  local aws_owner_id=`aws ec2 describe-instances --instance-ids ${instance_id} --query Reservations[0].OwnerId`
  if [[ -z "${aws_owner_id}" ]]; then
    echo "Unable to determinate AWS account id for ${instance_id}"
    exit 1
  fi

  aws cloudwatch put-metric-alarm \
                 --alarm-name k8s-${instance_id}-statuscheckfailure-reboot \
                 --alarm-description "Reboot ${instance_id} on status check failure" \
                 --namespace "AWS/EC2" \
                 --dimensions Name=InstanceId,Value=${instance_id} \
                 --statistic Minimum \
                 --metric-name StatusCheckFailed \
                 --comparison-operator GreaterThanThreshold \
                 --threshold 0 \
                 --period 60 \
                 --evaluation-periods 3 \
                 --alarm-actions arn:aws:swf:${AWS_REGION}:${aws_owner_id}:action/actions/AWS_EC2.InstanceId.Reboot/1.0 > $LOG

  # TODO: The IAM role EC2ActionsAccess must have been created
  # See e.g. http://docs.aws.amazon.com/AmazonCloudWatch/latest/DeveloperGuide/UsingIAM.html
}

function delete-instance-alarms {
  local instance_id=$1

  alarm_names=`aws cloudwatch describe-alarms --alarm-name-prefix k8s-${instance_id}- --query MetricAlarms[].AlarmName`
  for alarm_name in ${alarm_names}; do
    aws cloudwatch delete-alarms --alarm-names ${alarm_name} > $LOG
  done
}

# Finds the existing master IP, or creates/reuses an Elastic IP
# If MASTER_RESERVED_IP looks like an IP address, we will use it;
# otherwise we will create a new elastic IP
# Sets KUBE_MASTER_IP
function ensure-master-ip {
  find-tagged-master-ip

  if [[ -z "${KUBE_MASTER_IP:-}" ]]; then
    # Check if MASTER_RESERVED_IP looks like an IPv4 address
    # Note that we used to only allocate an elastic IP when MASTER_RESERVED_IP=auto
    # So be careful changing the IPV4 test, to be sure that 'auto' => 'allocate'
    if [[ "${MASTER_RESERVED_IP}" =~ ^[0-9]+\.[0-9]+\.[0-9]+\.[0-9]+$ ]]; then
      KUBE_MASTER_IP="${MASTER_RESERVED_IP}"
    else
      KUBE_MASTER_IP=`$AWS_CMD allocate-address --domain vpc --query PublicIp`
      echo "Allocated Elastic IP for master: ${KUBE_MASTER_IP}"
    fi

    # We can't tag elastic ips.  Instead we put the tag on the persistent disk.
    # It is a little weird, perhaps, but it sort of makes sense...
    # The master mounts the master PD, and whoever mounts the master PD should also
    # have the master IP
    add-tag ${MASTER_DISK_ID} ${TAG_KEY_MASTER_IP} ${KUBE_MASTER_IP}
  fi
}

# Creates a new DHCP option set configured correctly for Kubernetes when DHCP_OPTION_SET_ID is not specified
# Sets DHCP_OPTION_SET_ID
function create-dhcp-option-set () {
  if [[ -z ${DHCP_OPTION_SET_ID-} ]]; then
    local k8s_dhcp_opt_set=$($AWS_CMD describe-dhcp-options --filters Name=tag-key,Values=KubernetesCluster --query DhcpOptions[0].DhcpOptionsId)
    if [[ ${k8s_dhcp_opt_set} == "None" ]]; then
      echo "No KubernetesCluster DHCP options found"
      DHCP_OPTION_SET_ID=""
    else
      DHCP_OPTION_SET_ID=${k8s_dhcp_opt_set}
      echo "Using pre-existing KubernetesCluster DHCP options $DHCP_OPTION_SET_ID"
    fi
  else
    echo "Using pre-existing, user-specified DHCP options $DHCP_OPTION_SET_ID"
  fi

  if [[ -z ${DHCP_OPTION_SET_ID-} ]]; then
    case "${AWS_REGION}" in
      us-east-1)
        OPTION_SET_DOMAIN=ec2.internal
        ;;

      *)
        OPTION_SET_DOMAIN="${AWS_REGION}.compute.internal"
    esac

    DHCP_OPTION_SET_ID=$($AWS_CMD create-dhcp-options --dhcp-configuration Key=domain-name,Values=${OPTION_SET_DOMAIN} Key=domain-name-servers,Values=AmazonProvidedDNS --query DhcpOptions.DhcpOptionsId)

    add-tag ${DHCP_OPTION_SET_ID} Name kubernetes-dhcp-option-set
    add-tag ${DHCP_OPTION_SET_ID} KubernetesCluster ${CLUSTER_ID}
    echo "Using newly created DHCP option set ${DHCP_OPTION_SET_ID}"
  fi

  $AWS_CMD associate-dhcp-options --dhcp-options-id ${DHCP_OPTION_SET_ID} --vpc-id ${VPC_ID} > $LOG
}

function set-preset-variables-if-have {
  if [[ "${PRESET_INSTANCES_ENABLED:-}" != "yes" ]]; then
    return
  fi
  KUBE_MASTER_IP=$PRESET_KUBE_MASTER_IP
  MASTER_INTERNAL_IP=$PRESET_MASTER_INTERNAL_IP
  KUBE_MINION1_IP=$PRESET_KUBE_MINION1_IP    
  if [[ -n ${PRESET_KUBE_MINION2_IP:-} ]]; then
    KUBE_MINION2_IP=$PRESET_KUBE_MINION2_IP
    NUM_NODES=2
  else
    NUM_NODES=1
  fi
  if [[ -z ${JENKINS_HOME:-} ]]; then    
    ACCESS_FILE=${HOME}/ArktosE2E
  else
    ACCESS_FILE=${JENKINS_HOME}/secrets/ArktosE2E
  fi
}

# Verify prereqs
function verify-prereqs {
  if [[ "${PRESET_INSTANCES_ENABLED:-}" != "yes" && "$(which aws)" == "" ]]; then
    echo "Can't find aws in PATH, please fix and retry."
    exit 1
  fi
}

# Create a temp dir that'll be deleted at the end of this bash session.
#
# Vars set:
#   KUBE_TEMP
function ensure-temp-dir {
  if [[ -z ${KUBE_TEMP-} ]]; then
    KUBE_TEMP=$(mktemp -d -t kubernetes.XXXXXX)
    trap 'rm -rf "${KUBE_TEMP}"' EXIT
  fi
}

# Take the local tar files and upload them to S3.  They will then be
# downloaded by the master as part of the start up script for the master.
#
# Assumed vars:
#   SERVER_BINARY_TAR
# Vars set:
#   SERVER_BINARY_TAR_URL
function upload-server-tars() {
  SERVER_BINARY_TAR_URL=
  SERVER_BINARY_TAR_HASH=
  BOOTSTRAP_SCRIPT_URL=
  BOOTSTRAP_SCRIPT_HASH=

  ensure-temp-dir

  SERVER_BINARY_TAR_HASH=$(sha1sum-file "${SERVER_BINARY_TAR}")
  BOOTSTRAP_SCRIPT_HASH=$(sha1sum-file "${BOOTSTRAP_SCRIPT}")

  if [[ "${PRESET_INSTANCES_ENABLED:-}" == "yes" ]]; then
    SERVER_BINARY_TAR_URL=$PRESET_SERVER_BINARY_TAR_URL
    BOOTSTRAP_SCRIPT_URL=$PRESET_BOOTSTRAP_SCRIPT_URL

    scp -o 'StrictHostKeyChecking no' -i ${ACCESS_FILE} ${SERVER_BINARY_TAR} ${BOOTSTRAP_SCRIPT} ${SSH_USER}@${KUBE_MASTER_IP}:/tmp
    scp -o 'StrictHostKeyChecking no' -i ${ACCESS_FILE} ${SERVER_BINARY_TAR} ${BOOTSTRAP_SCRIPT} ${SSH_USER}@${KUBE_MINION1_IP}:/tmp
    if [[ -n ${KUBE_MINION2_IP:-} ]]; then
      scp -o 'StrictHostKeyChecking no' -i ${ACCESS_FILE} ${SERVER_BINARY_TAR} ${BOOTSTRAP_SCRIPT} ${SSH_USER}@${KUBE_MINION2_IP}:/tmp
    fi
  else
    if [[ -z ${AWS_S3_BUCKET-} ]]; then
          local project_hash=
          local key=$(aws configure get aws_access_key_id)
          if which md5 > /dev/null 2>&1; then
            project_hash=$(md5 -q -s "${USER} ${key} ${INSTANCE_PREFIX}")
          else
            project_hash=$(echo -n "${USER} ${key} ${INSTANCE_PREFIX}" | md5sum | awk '{ print $1 }')
          fi
          AWS_S3_BUCKET="kubernetes-staging-${project_hash}"
      fi

      echo "Uploading to Amazon S3"

      if ! aws s3api get-bucket-location --bucket ${AWS_S3_BUCKET} > /dev/null 2>&1 ; then
        echo "Creating ${AWS_S3_BUCKET}"

        # Buckets must be globally uniquely named, so always create in a known region
        # We default to us-east-1 because that's the canonical region for S3,
        # and then the bucket is most-simply named (s3.amazonaws.com)
        aws s3 mb "s3://${AWS_S3_BUCKET}" --region ${AWS_S3_REGION}

        echo "Confirming bucket was created..."

        local attempt=0
        while true; do
          if ! aws s3 ls --region ${AWS_S3_REGION} "s3://${AWS_S3_BUCKET}" > /dev/null 2>&1; then
            if (( attempt > 120 )); then
              echo
              echo -e "${color_red}Unable to confirm bucket creation." >&2
              echo "Please ensure that s3://${AWS_S3_BUCKET} exists" >&2
              echo -e "and run the script again. (sorry!)${color_norm}" >&2
              exit 1
            fi
          else
            break
          fi
          attempt=$(($attempt+1))
          sleep 1
        done
      fi

      local s3_bucket_location=$(aws s3api get-bucket-location --bucket ${AWS_S3_BUCKET})
      local s3_url_base=https://s3-${s3_bucket_location}.amazonaws.com
      if [[ "${s3_bucket_location}" == "None" ]]; then
        # "US Classic" does not follow the pattern
        s3_url_base=https://s3.amazonaws.com
        s3_bucket_location=us-east-1
      elif [[ "${s3_bucket_location}" == "cn-north-1" ]]; then
        s3_url_base=https://s3.cn-north-1.amazonaws.com.cn
      fi

      local -r staging_path="devel"

      local -r local_dir="${KUBE_TEMP}/s3/"
      mkdir ${local_dir}

      echo "+++ Staging server tars to S3 Storage: ${AWS_S3_BUCKET}/${staging_path}"
      cp -a "${SERVER_BINARY_TAR}" ${local_dir}
      cp -a "${BOOTSTRAP_SCRIPT}" ${local_dir}

      aws s3 sync --region ${s3_bucket_location} --exact-timestamps ${local_dir} "s3://${AWS_S3_BUCKET}/${staging_path}/"

      local server_binary_path="${staging_path}/${SERVER_BINARY_TAR##*/}"
      aws s3api put-object-acl --region ${s3_bucket_location} --bucket ${AWS_S3_BUCKET} --key "${server_binary_path}" --grant-read 'uri="http://acs.amazonaws.com/groups/global/AllUsers"'
      SERVER_BINARY_TAR_URL="${s3_url_base}/${AWS_S3_BUCKET}/${server_binary_path}"

      local bootstrap_script_path="${staging_path}/${BOOTSTRAP_SCRIPT##*/}"
      aws s3api put-object-acl --region ${s3_bucket_location} --bucket ${AWS_S3_BUCKET} --key "${bootstrap_script_path}" --grant-read 'uri="http://acs.amazonaws.com/groups/global/AllUsers"'
      BOOTSTRAP_SCRIPT_URL="${s3_url_base}/${AWS_S3_BUCKET}/${bootstrap_script_path}"
  fi

  echo "Uploaded server tars:"
  echo "  SERVER_BINARY_TAR_URL: ${SERVER_BINARY_TAR_URL}"
  echo "  BOOTSTRAP_SCRIPT_URL: ${BOOTSTRAP_SCRIPT_URL}"
}

# Adds a tag to an AWS resource
# usage: add-tag <resource-id> <tag-name> <tag-value>
function add-tag {
  echo "Adding tag to ${1}: ${2}=${3}"

  # We need to retry in case the resource isn't yet fully created
  n=0
  until [ $n -ge 25 ]; do
    $AWS_CMD create-tags --resources ${1} --tags Key=${2},Value=${3} > $LOG && return
    n=$[$n+1]
    sleep 3
  done

  echo "Unable to add tag to AWS resource"
  exit 1
}

# Creates the IAM profile, based on configuration files in templates/iam
# usage: create-iam-profile kubernetes-master-us-west-1a-chom kubernetes-master
function create-iam-profile {
  local key=$1
  local role=$2

  local conf_dir=file://${KUBE_ROOT}/cluster/aws/templates/iam

  echo "Creating IAM role: ${key}"
  aws iam create-role --role-name ${key} --assume-role-policy-document ${conf_dir}/${role}-role.json > $LOG

  echo "Creating IAM role-policy: ${key}"
  aws iam put-role-policy --role-name ${key} --policy-name ${key} --policy-document ${conf_dir}/${role}-policy.json > $LOG

  echo "Creating IAM instance-policy: ${key}"
  aws iam create-instance-profile --instance-profile-name ${key} > $LOG

  echo "Adding IAM role to instance-policy: ${key}"
  aws iam add-role-to-instance-profile --instance-profile-name ${key} --role-name ${key} > $LOG
}

# Creates the IAM roles (if they do not already exist)
function ensure-iam-profiles {
  echo "Creating master IAM profile: ${IAM_PROFILE_MASTER}"
  create-iam-profile ${IAM_PROFILE_MASTER} kubernetes-master

  echo "Creating minion IAM profile: ${IAM_PROFILE_NODE}"
  create-iam-profile ${IAM_PROFILE_NODE} kubernetes-minion
}

# Wait for instance to be in specified state
function wait-for-instance-state {
  instance_id=$1
  state=$2

  while true; do
    instance_state=$($AWS_CMD describe-instances --instance-ids ${instance_id} --query Reservations[].Instances[].State.Name)
    if [[ "$instance_state" == "${state}" ]]; then
      break
    else
      echo "Waiting for instance ${instance_id} to be ${state} (currently ${instance_state})"
      echo "Sleeping for 3 seconds..."
      sleep 3
    fi
  done
}

# Allocates new Elastic IP from Amazon
# Output: allocated IP address
function allocate-elastic-ip {
  $AWS_CMD allocate-address --domain vpc --query PublicIp
}

# Attaches an elastic IP to the specified instance
function attach-ip-to-instance {
  local ip_address=$1
  local instance_id=$2

  local elastic_ip_allocation_id=$($AWS_CMD describe-addresses --public-ips $ip_address --query Addresses[].AllocationId)
  echo "Attaching IP ${ip_address} to instance ${instance_id} , allocation-id '${elastic_ip_allocation_id}'"
  $AWS_CMD associate-address --instance-id ${instance_id} --allocation-id ${elastic_ip_allocation_id} > $LOG
}

# Releases an elastic IP
function release-elastic-ip {
  local ip_address=$1

  echo "Releasing Elastic IP: ${ip_address}"
  elastic_ip_allocation_id=$($AWS_CMD describe-addresses --public-ips $ip_address --query Addresses[].AllocationId 2> $LOG) || true
  if [[ -z "${elastic_ip_allocation_id}" ]]; then
    echo "Elastic IP already released"
  else
    $AWS_CMD release-address --allocation-id ${elastic_ip_allocation_id} > $LOG
  fi
}

# Deletes a security group
# usage: delete_security_group <sgid>
function delete_security_group {
  local -r sg_id=${1}

  echo "Deleting security group: ${sg_id}"

  # We retry in case there's a dependent resource - typically an ELB
  local n=0
  until [ $n -ge 20 ]; do
    $AWS_CMD delete-security-group --group-id ${sg_id} > $LOG && return
    n=$[$n+1]
    sleep 3
  done
  echo "Unable to delete security group: ${sg_id}"
  exit 1
}

# Deletes master and minion IAM roles and instance profiles
# usage: delete-iam-instance-profiles
function delete-iam-profiles {
  for iam_profile_name in ${IAM_PROFILE_MASTER} ${IAM_PROFILE_NODE};do
    echo "Removing role from instance profile: ${iam_profile_name}"
    conceal-no-such-entity-response aws iam remove-role-from-instance-profile --instance-profile-name "${iam_profile_name}" --role-name "${iam_profile_name}"

    echo "Deleting IAM Instance-Profile: ${iam_profile_name}"
    conceal-no-such-entity-response aws iam delete-instance-profile --instance-profile-name "${iam_profile_name}"

    echo "Delete IAM role policy: ${iam_profile_name}"
    conceal-no-such-entity-response aws iam delete-role-policy --role-name "${iam_profile_name}" --policy-name "${iam_profile_name}"

    echo "Deleting IAM Role: ${iam_profile_name}"
    conceal-no-such-entity-response aws iam delete-role --role-name "${iam_profile_name}"
  done
}

# Detects NoSuchEntity response from AWS cli stderr output and conceals error
# Otherwise the error is treated as fatal
# usage: conceal-no-such-entity-response ...args
function conceal-no-such-entity-response {
  # in plain english: redirect stderr to stdout, and stdout to the log file
  local -r errMsg=$($@ 2>&1 > $LOG)
  if [[ "$errMsg" == "" ]];then
    return
  fi

  echo $errMsg
  if [[ "$errMsg" =~ " (NoSuchEntity) " ]];then
    echo " -> no such entity response detected. will assume operation is not necessary due to prior incomplete teardown"
    return
  fi

  echo "Error message is fatal. Will exit"
  exit 1
}

function ssh-key-setup {
  if [[ ! -f "$AWS_SSH_KEY" ]]; then
    ssh-keygen -f "$AWS_SSH_KEY" -N ''
  fi

  # Note that we use get-ssh-fingerprint, so this works on OSX Mavericks
  # get-aws-fingerprint gives the same fingerprint that AWS computes,
  # but OSX Mavericks ssh-keygen can't compute it
  AWS_SSH_KEY_FINGERPRINT=$(get-ssh-fingerprint ${AWS_SSH_KEY}.pub)
  echo "Using SSH key with (AWS) fingerprint: ${AWS_SSH_KEY_FINGERPRINT}"
  AWS_SSH_KEY_NAME="kubernetes-${AWS_SSH_KEY_FINGERPRINT//:/}"

  import-public-key ${AWS_SSH_KEY_NAME} ${AWS_SSH_KEY}.pub
}

function vpc-setup {
  if [[ -z "${VPC_ID:-}" ]]; then
    VPC_ID=$(get_vpc_id)
    echo "Using pre-existing KubernetesCluster VPC $VPC_ID"
  else
    echo "Using pre-existing, user-specified VPC $VPC_ID"
  fi
  if [[ -z "$VPC_ID" ]]; then
	  echo "Creating vpc."
	  VPC_ID=$($AWS_CMD create-vpc --cidr-block ${VPC_CIDR} --query Vpc.VpcId)
	  $AWS_CMD modify-vpc-attribute --vpc-id $VPC_ID --enable-dns-support '{"Value": true}' > $LOG
	  $AWS_CMD modify-vpc-attribute --vpc-id $VPC_ID --enable-dns-hostnames '{"Value": true}' > $LOG
	  add-tag $VPC_ID Name ${VPC_NAME}
	  add-tag $VPC_ID KubernetesCluster ${CLUSTER_ID}
          echo "Using newly created KubernetesCluster VPC $VPC_ID"
  fi
}

function subnet-setup {
  if [[ -z "${SUBNET_ID:-}" ]]; then
    SUBNET_ID=$(get_subnet_id $VPC_ID $ZONE)
    echo "Using subnet $SUBNET_ID in VPC $VPC_ID Zone $ZONE"
  else
    echo "Using pre-existing, user-specified subnet $SUBNET_ID"
  fi

  if [[ -z "$SUBNET_ID" ]]; then
    echo "Creating subnet."
    SUBNET_ID=$($AWS_CMD create-subnet --cidr-block ${SUBNET_CIDR} --vpc-id $VPC_ID --availability-zone ${ZONE} --query Subnet.SubnetId)
    add-tag $SUBNET_ID KubernetesCluster ${CLUSTER_ID}
    echo "Using newly created KubernetesCluster subnet $SUBNET_ID"
  else
    EXISTING_CIDR=$($AWS_CMD describe-subnets --subnet-ids ${SUBNET_ID} --query Subnets[].CidrBlock)
    echo "Using pre-existing, user-specified subnet with pre-existing CIDR $EXISTING_CIDR"
    if [ ! $SUBNET_CIDR = $EXISTING_CIDR ]; then
      MASTER_INTERNAL_IP="${EXISTING_CIDR%.*}${MASTER_IP_SUFFIX}"
      echo "Assuming MASTER_INTERNAL_IP=${MASTER_INTERNAL_IP}"
    fi
  fi
}

function start-flannel-ds {
  pushd $KUBE_TEMP
  wget https://raw.githubusercontent.com/coreos/flannel/master/Documentation/kube-flannel.yml
  ${KUBE_ROOT}/cluster/kubectl.sh --kubeconfig=${LOCAL_KUBECONFIG} create -f $KUBE_TEMP/kube-flannel.yml
  popd
}

function start-weave-ds {
  local kubectl_ver="$(${KUBE_ROOT}/cluster/kubectl.sh version | base64 | tr -d '\n')"
  ${KUBE_ROOT}/cluster/kubectl.sh --kubeconfig=${LOCAL_KUBECONFIG} create -f "https://cloud.weave.works/k8s/net?k8s-version=$kubectl_ver"
}

function start-bridge-networking {
  :
}

function start-cluster-networking {
  case "${NETWORK_PROVIDER}" in
    flannel)
    start-flannel-ds
    ;;
    weave)
    start-weave-ds
    ;;
    bridge)
    start-bridge-networking
    ;;
  esac
}

function kube-up {
  echo "Starting cluster using os distro: ${OS_DISTRIBUTION}" >&2

  get-tokens

  detect-image
  detect-minion-image

  if [[ "${PRESET_INSTANCES_ENABLED:-}" != "yes" ]]; then
    detect-root-device
  fi

  find-release-tars

  ensure-temp-dir

  create-bootstrap-script

  write-controller-config
  
  upload-server-tars

  if [[ "${PRESET_INSTANCES_ENABLED:-}" != "yes" ]]; then
    ensure-iam-profiles
  fi

  load-or-gen-kube-basicauth
  load-or-gen-kube-bearertoken

  if [[ "${PRESET_INSTANCES_ENABLED:-}" != "yes" ]]; then
    ssh-key-setup
  fi

  vpc-setup

  if [[ "${PRESET_INSTANCES_ENABLED:-}" != "yes" ]]; then
    create-dhcp-option-set
  fi  

  if [[ "${PRESET_INSTANCES_ENABLED:-}" != "yes" ]]; then
    subnet-setup
  fi  

  if [[ "${PRESET_INSTANCES_ENABLED:-}" == "yes" ]]; then
    IGW_ID=$PRESET_IGW_ID
  else
    IGW_ID=$(get_igw_id $VPC_ID)
    if [[ -z "$IGW_ID" ]]; then
      echo "Creating Internet Gateway."
      IGW_ID=$($AWS_CMD create-internet-gateway --query InternetGateway.InternetGatewayId)
      $AWS_CMD attach-internet-gateway --internet-gateway-id $IGW_ID --vpc-id $VPC_ID > $LOG
    fi
  fi
  echo "Using Internet Gateway $IGW_ID"

  if [[ "${PRESET_INSTANCES_ENABLED:-}" == "yes" ]]; then
    ROUTE_TABLE_ID=$PRESET_ROUTE_TABLE_ID
  else
    echo "Associating route table."
    ROUTE_TABLE_ID=$($AWS_CMD describe-route-tables \
                              --filters Name=vpc-id,Values=${VPC_ID} \
                                        Name=tag:KubernetesCluster,Values=${CLUSTER_ID} \
                              --query RouteTables[].RouteTableId)
    if [[ -z "${ROUTE_TABLE_ID}" ]]; then
      echo "Creating route table"
      ROUTE_TABLE_ID=$($AWS_CMD create-route-table \
                                --vpc-id=${VPC_ID} \
                                --query RouteTable.RouteTableId)
      add-tag ${ROUTE_TABLE_ID} KubernetesCluster ${CLUSTER_ID}
    fi

    echo "Associating route table ${ROUTE_TABLE_ID} to subnet ${SUBNET_ID}"
    $AWS_CMD associate-route-table --route-table-id $ROUTE_TABLE_ID --subnet-id $SUBNET_ID > $LOG || true
    echo "Adding gateway route $IGW_ID to route table ${ROUTE_TABLE_ID}"
    $AWS_CMD create-route --route-table-id $ROUTE_TABLE_ID --destination-cidr-block 0.0.0.0/0 --gateway-id $IGW_ID > $LOG || true
  fi
  echo "Using Route Table $ROUTE_TABLE_ID"

  if [[ "${PRESET_INSTANCES_ENABLED:-}" == "yes" ]]; then
    MASTER_SG_ID=$PRESET_MASTER_SG_ID
    NODE_SG_ID=$PRESET_NODE_SG_ID
  else
    # Create security groups
    MASTER_SG_ID=$(get_security_group_id "${MASTER_SG_NAME}")
    if [[ -z "${MASTER_SG_ID}" ]]; then
      echo "Creating master security group."
      create-security-group "${MASTER_SG_NAME}" "Kubernetes security group applied to master nodes"
    fi
    NODE_SG_ID=$(get_security_group_id "${NODE_SG_NAME}")
    if [[ -z "${NODE_SG_ID}" ]]; then
      echo "Creating minion security group."
      create-security-group "${NODE_SG_NAME}" "Kubernetes security group applied to minion nodes"
    fi

    detect-security-groups

    # Masters can talk to master
    authorize-security-group-ingress "${MASTER_SG_ID}" "--source-group ${MASTER_SG_ID} --protocol all"

    # Minions can talk to minions
    authorize-security-group-ingress "${NODE_SG_ID}" "--source-group ${NODE_SG_ID} --protocol all"

    # Masters and minions can talk to each other
    authorize-security-group-ingress "${MASTER_SG_ID}" "--source-group ${NODE_SG_ID} --protocol all"
    authorize-security-group-ingress "${NODE_SG_ID}" "--source-group ${MASTER_SG_ID} --protocol all"

    # SSH is open to the world
    authorize-security-group-ingress "${MASTER_SG_ID}" "--protocol tcp --port 22 --cidr ${SSH_CIDR}"
    authorize-security-group-ingress "${NODE_SG_ID}" "--protocol tcp --port 22 --cidr ${SSH_CIDR}"

    # HTTPS to the master is allowed (for API access)
    authorize-security-group-ingress "${MASTER_SG_ID}" "--protocol tcp --port 443 --cidr ${HTTP_API_CIDR}"

    # Allow secure access to API
    authorize-security-group-ingress "${MASTER_SG_ID}" "--protocol tcp --port 6443 --cidr ${HTTP_API_CIDR}"

    # Allow access to API via proxy - TODO: Limit to our CIDR
    authorize-security-group-ingress "${MASTER_SG_ID}" "--protocol tcp --port 8001"
  fi

  # KUBE_USE_EXISTING_MASTER is used to add minions to an existing master
  if [[ "${KUBE_USE_EXISTING_MASTER:-}" == "true" ]]; then
    detect-master
    parse-master-env

    if [[ "${ENABLE_CLUSTER_AUTOSCALER:-false}" == "true" ]]; then
      # Start minions
      start-minions
      wait-minions
    fi
  else
    # Create the master
    start-master

    if [[ "${KUBE_CREATE_NODES}" == "true" ]]; then
      # Start minions
      start-minions
      if [[ "${PRESET_INSTANCES_ENABLED:-}" != "yes" ]]; then
        wait-minions
      fi
    fi

    # Wait for the master to be ready
    wait-master

    # Start cluster networking
    start-cluster-networking
  fi

  # Check the cluster is OK
  check-cluster
}

# Builds the bootstrap script and saves it to a local temp file
# Sets BOOTSTRAP_SCRIPT to the path of the script
function create-bootstrap-script() {
  ensure-temp-dir

  POD_NETWORK_CIDR=""
  if [[ $NETWORK_PROVIDER == "flannel" ]]; then
    POD_NETWORK_CIDR="10.244.0.0/16"
  fi

  BOOTSTRAP_SCRIPT="${KUBE_TEMP}/bootstrap-script"

  (
    # Include the default functions from the configure-vm script
    sed '/^#+AWS_OVERRIDES_HERE/,$d' "${KUBE_ROOT}/cluster/aws/configure-vm.sh"
    # Include the configure-vm directly-executed code
    sed -e '1,/^#+AWS_OVERRIDES_HERE/d' "${KUBE_ROOT}/cluster/aws/configure-vm.sh"
  ) > "${BOOTSTRAP_SCRIPT}"
}

# Builds the vm cleanup script and saves it to a local temp file
# Sets CLEANUP_SCRIPT to the path of the script
function create-cleanup-script() {
  ensure-temp-dir

  CLEANUP_SCRIPT="${KUBE_TEMP}/cleanup-script"

  (
    # Include the default functions from the cleanup-preset-vm script
    sed '/^#+AWS_OVERRIDES_HERE/,$d' "${KUBE_ROOT}/cluster/aws/cleanup-preset-vm.sh"
    # Include the cleanup-preset-vm directly-executed code
    sed -e '1,/^#+AWS_OVERRIDES_HERE/d' "${KUBE_ROOT}/cluster/aws/cleanup-preset-vm.sh"
  ) > "${CLEANUP_SCRIPT}"
}

# copy controller config into a temporary file.
# Assumed vars
function write-controller-config {
  if [[ -f ${KUBE_ROOT}/cmd/workload-controller-manager/config/controllerconfig.json ]]; then
    cp "${KUBE_ROOT}/cmd/workload-controller-manager/config/controllerconfig.json" "${KUBE_TEMP}/controllerconfig.json"
  else
    cat <<EOF >${KUBE_TEMP}/controllerconfig.json
{
    "controllers": [
        {
            "type":    "node",
            "workers":     5
        },
        {
            "type":    "replicaset",
            "workers":    10
        }
    ]
}
EOF
  fi
}

# Starts the master node
function start-master() {

  if [[ "${PRESET_INSTANCES_ENABLED:-}" != "yes" ]]; then
    # Ensure RUNTIME_CONFIG is populated
    build-runtime-config

    # Get or create master persistent volume
    ensure-master-pd

    # Get or create master elastic IP
    ensure-master-ip
  fi  

  # This key is no longer needed, and this enables us to get under the 16KB size limit
  KUBECFG_CERT_BASE64=""
  KUBECFG_KEY_BASE64=""

  write-master-env

  if [[ "${PRESET_INSTANCES_ENABLED:-}" == "yes" ]]; then
    (
      # We pipe this to the ami as a startup script in the user-data field.  Requires a compatible ami
      echo "#! /bin/bash"
      echo "mkdir -p /var/cache/kubernetes-install"
      echo "cd /var/cache/kubernetes-install"

      echo "cat > kube_env.yaml << __EOF_MASTER_KUBE_ENV_YAML"
      cat ${KUBE_TEMP}/master-kube-env.yaml
      echo "AUTO_UPGRADE: 'true'"
      # TODO: get rid of these exceptions / harmonize with common or GCE
      echo "DOCKER_STORAGE: $(yaml-quote ${DOCKER_STORAGE:-})"
      echo "API_SERVERS: $(yaml-quote ${MASTER_INTERNAL_IP:-})"
      echo "API_BIND_PORT: $(yaml-quote ${API_BIND_PORT:-6443})"
      echo "MASTER_EXTERNAL_IP: $(yaml-quote ${KUBE_MASTER_IP:-})"
      echo "POD_NETWORK_CIDR: $(yaml-quote ${POD_NETWORK_CIDR:-})"
      echo "__EOF_MASTER_KUBE_ENV_YAML"
      echo ""
      echo "cat > workload-controller-manager.manifest << __EOF_WORKLOAD_CONTROLLER_MANAGER_MANIFEST"
      cat ${KUBE_ROOT}/cluster/aws/manifests/workload-controller-manager.manifest
      echo "__EOF_WORKLOAD_CONTROLLER_MANAGER_MANIFEST"
      echo ""
      echo "cat > workload-controllerconfig.json << __EOF_WORKLOAD_CONTROLLER_CONFIG_JSON"
      cat ${KUBE_TEMP}/controllerconfig.json
      echo ""
      echo "__EOF_WORKLOAD_CONTROLLER_CONFIG_JSON"
      echo ""
      echo "cp ${BOOTSTRAP_SCRIPT_URL} bootstrap"
      echo "chmod +x bootstrap"
      echo "mkdir -p /etc/kubernetes"
      echo "mv kube_env.yaml /etc/kubernetes"
      echo "mv bootstrap /etc/kubernetes/"
    ) > "${KUBE_TEMP}/master-user-data"

    scp -o 'StrictHostKeyChecking no' -i ${ACCESS_FILE} ${KUBE_TEMP}/master-user-data ${SSH_USER}@${KUBE_MASTER_IP}:/tmp
    ssh -o 'StrictHostKeyChecking no' -i ${ACCESS_FILE} ${SSH_USER}@${KUBE_MASTER_IP} "sudo mkdir -p /mnt/master-pd && chmod 755 /tmp/master-user-data && sudo /tmp/master-user-data"
    ssh -o 'StrictHostKeyChecking no' -i ${ACCESS_FILE} ${SSH_USER}@${KUBE_MASTER_IP} "sudo /etc/kubernetes/bootstrap &>/tmp/bootstrap.log & disown"

    echo "Master is running: public_ip= $KUBE_MASTER_IP"
  else
    (
      # We pipe this to the ami as a startup script in the user-data field.  Requires a compatible ami
      echo "#! /bin/bash"
      echo "mkdir -p /var/cache/kubernetes-install"
      echo "cd /var/cache/kubernetes-install"

      echo "cat > kube_env.yaml << __EOF_MASTER_KUBE_ENV_YAML"
      cat ${KUBE_TEMP}/master-kube-env.yaml
      echo "AUTO_UPGRADE: 'true'"
      # TODO: get rid of these exceptions / harmonize with common or GCE
      echo "DOCKER_STORAGE: $(yaml-quote ${DOCKER_STORAGE:-})"
      echo "API_SERVERS: $(yaml-quote ${MASTER_INTERNAL_IP:-})"
      echo "API_BIND_PORT: $(yaml-quote ${API_BIND_PORT:-6443})"
      echo "MASTER_EXTERNAL_IP: $(yaml-quote ${KUBE_MASTER_IP:-})"
      echo "POD_NETWORK_CIDR: $(yaml-quote ${POD_NETWORK_CIDR:-})"
      echo "__EOF_MASTER_KUBE_ENV_YAML"
      echo ""
      echo "cat > workload-controller-manager.manifest << __EOF_WORKLOAD_CONTROLLER_MANAGER_MANIFEST"
      cat ${KUBE_ROOT}/cluster/aws/manifests/workload-controller-manager.manifest
      echo "__EOF_WORKLOAD_CONTROLLER_MANAGER_MANIFEST"
      echo ""
      echo "cat > workload-controllerconfig.json << __EOF_WORKLOAD_CONTROLLER_CONFIG_JSON"
      cat ${KUBE_TEMP}/controllerconfig.json
      echo ""
      echo "__EOF_WORKLOAD_CONTROLLER_CONFIG_JSON"
      echo ""
      echo "wget -O bootstrap ${BOOTSTRAP_SCRIPT_URL}"
      echo "chmod +x bootstrap"
      echo "mkdir -p /etc/kubernetes"
      echo "mv kube_env.yaml /etc/kubernetes"
      echo "mv bootstrap /etc/kubernetes/"
      echo "cat > /etc/rc.local << EOF_RC_LOCAL"
      echo "#!/bin/sh -e"
      # We want to be sure that we don't pass an argument to bootstrap
      echo "/etc/kubernetes/bootstrap"
      echo "exit 0"
      echo "EOF_RC_LOCAL"
      echo "/etc/kubernetes/bootstrap"
    ) > "${KUBE_TEMP}/master-user-data"

    # Compress the data to fit under the 16KB limit (cloud-init accepts compressed data)
    gzip "${KUBE_TEMP}/master-user-data"

    echo "Starting Master"
    master_id=$($AWS_CMD run-instances \
      --image-id $AWS_IMAGE \
      --iam-instance-profile Name=$IAM_PROFILE_MASTER \
      --instance-type $MASTER_SIZE \
      --subnet-id $SUBNET_ID \
      --private-ip-address $MASTER_INTERNAL_IP \
      --key-name ${AWS_SSH_KEY_NAME} \
      --security-group-ids ${MASTER_SG_ID} \
      --no-associate-public-ip-address \
      --block-device-mappings "${MASTER_BLOCK_DEVICE_MAPPINGS}" \
      --user-data fileb://${KUBE_TEMP}/master-user-data.gz \
      --query Instances[].InstanceId)
    add-tag $master_id Name $MASTER_NAME
    add-tag $master_id Role $MASTER_TAG
    add-tag $master_id KubernetesCluster ${CLUSTER_ID}

    echo "Waiting for master to be ready"
    local attempt=0

    local ip=""
    while true; do
      echo -n Attempt "$(($attempt+1))" to check for master node
      if [[ -z "${ip}" ]]; then
        # We are not able to add an elastic ip, a route or volume to the instance until that instance is in "running" state.
        wait-for-instance-state ${master_id} "running"

        KUBE_MASTER=${MASTER_NAME}
        echo -e " ${color_green}[master running]${color_norm}"

        attach-ip-to-instance ${KUBE_MASTER_IP} ${master_id}

        # This is a race between instance start and volume attachment.  There appears to be no way to start an AWS instance with a volume attached.
        # To work around this, we wait for volume to be ready in setup-master-pd.sh
        echo "Attaching persistent data volume (${MASTER_DISK_ID}) to master"
        $AWS_CMD attach-volume --volume-id ${MASTER_DISK_ID} --device /dev/sdb --instance-id ${master_id}

        sleep 10
        $AWS_CMD create-route --route-table-id $ROUTE_TABLE_ID --destination-cidr-block ${MASTER_IP_RANGE} --instance-id $master_id > $LOG

        break
      fi
      echo -e " ${color_yellow}[master not working yet]${color_norm}"
      attempt=$(($attempt+1))
      if (( attempt > 10 )); then
        echo
        echo -e "${color_red}master failed to start. Your cluster is unlikely" >&2
        echo "to work correctly. Please run ./cluster/kube-down.sh and re-create the" >&2
        echo -e "cluster. (sorry!)${color_norm}" >&2
        exit 1
      fi
      sleep 10
      ip=$(get_instance_public_ip ${master_id})
      echo "Started master: public_ip= $ip"
    done
  fi
}

# Creates an ASG for the minion nodes
function start-minions() {
  # Minions don't currently use runtime config, but call it anyway for sanity
  build-runtime-config

  echo "Creating minion configuration"

  write-node-env

  if [[ "${PRESET_INSTANCES_ENABLED:-}" == "yes" ]]; then
    (
      # We pipe this to the ami as a startup script in the user-data field.  Requires a compatible ami
      echo "#! /bin/bash"
      echo "mkdir -p /var/cache/kubernetes-install"
      echo "cd /var/cache/kubernetes-install"
      echo "cat > kube_env.yaml << __EOF_KUBE_ENV_YAML"
      cat ${KUBE_TEMP}/node-kube-env.yaml
      echo "AUTO_UPGRADE: 'true'"
      # TODO: get rid of these exceptions / harmonize with common or GCE
      echo "DOCKER_STORAGE: $(yaml-quote ${DOCKER_STORAGE:-})"
      echo "API_SERVERS: $(yaml-quote ${MASTER_INTERNAL_IP:-})"
      echo "__EOF_KUBE_ENV_YAML"
      echo ""
      echo "cp ${BOOTSTRAP_SCRIPT_URL} bootstrap"
      echo "chmod +x bootstrap"
      echo "mkdir -p /etc/kubernetes"
      echo "mv kube_env.yaml /etc/kubernetes"
      echo "mv bootstrap /etc/kubernetes/"
    ) > "${KUBE_TEMP}/node-user-data"

    scp -o 'StrictHostKeyChecking no' -i ${ACCESS_FILE} ${KUBE_TEMP}/node-user-data ${SSH_USER}@${KUBE_MINION1_IP}:/tmp
    ssh -o 'StrictHostKeyChecking no' -i ${ACCESS_FILE} ${SSH_USER}@${KUBE_MINION1_IP} "chmod 755 /tmp/node-user-data && sudo /tmp/node-user-data"
    ssh -o 'StrictHostKeyChecking no' -i ${ACCESS_FILE} ${SSH_USER}@${KUBE_MINION1_IP} "sudo /etc/kubernetes/bootstrap"
    echo "Minion1 is running"

    if [[ -n ${KUBE_MINION2_IP:-} ]]; then
      scp -o 'StrictHostKeyChecking no' -i ${ACCESS_FILE} ${KUBE_TEMP}/node-user-data ${SSH_USER}@${KUBE_MINION2_IP}:/tmp
      ssh -o 'StrictHostKeyChecking no' -i ${ACCESS_FILE} ${SSH_USER}@${KUBE_MINION2_IP} "chmod 755 /tmp/node-user-data && sudo /tmp/node-user-data"
      ssh -o 'StrictHostKeyChecking no' -i ${ACCESS_FILE} ${SSH_USER}@${KUBE_MINION2_IP} "sudo /etc/kubernetes/bootstrap"
      echo "Minion2 is running"
    fi
  else
    (
      # We pipe this to the ami as a startup script in the user-data field.  Requires a compatible ami
      echo "#! /bin/bash"
      echo "mkdir -p /var/cache/kubernetes-install"
      echo "cd /var/cache/kubernetes-install"
      echo "cat > kube_env.yaml << __EOF_KUBE_ENV_YAML"
      cat ${KUBE_TEMP}/node-kube-env.yaml
      echo "AUTO_UPGRADE: 'true'"
      # TODO: get rid of these exceptions / harmonize with common or GCE
      echo "DOCKER_STORAGE: $(yaml-quote ${DOCKER_STORAGE:-})"
      echo "API_SERVERS: $(yaml-quote ${MASTER_INTERNAL_IP:-})"
      echo "__EOF_KUBE_ENV_YAML"
      echo ""
      echo "wget -O bootstrap ${BOOTSTRAP_SCRIPT_URL}"
      echo "chmod +x bootstrap"
      echo "mkdir -p /etc/kubernetes"
      echo "mv kube_env.yaml /etc/kubernetes"
      echo "mv bootstrap /etc/kubernetes/"
      echo "cat > /etc/rc.local << EOF_RC_LOCAL"
      echo "#!/bin/sh -e"
      # We want to be sure that we don't pass an argument to bootstrap
      echo "/etc/kubernetes/bootstrap"
      echo "exit 0"
      echo "EOF_RC_LOCAL"
      echo "/etc/kubernetes/bootstrap"
    ) > "${KUBE_TEMP}/node-user-data"

    # Compress the data to fit under the 16KB limit (cloud-init accepts compressed data)
    gzip "${KUBE_TEMP}/node-user-data"

    local public_ip_option
    if [[ "${ENABLE_NODE_PUBLIC_IP}" == "true" ]]; then
      public_ip_option="--associate-public-ip-address"
    else
      public_ip_option="--no-associate-public-ip-address"
    fi
    local spot_price_option
    if [[ -n "${NODE_SPOT_PRICE:-}" ]]; then
      spot_price_option="--spot-price ${NODE_SPOT_PRICE}"
    else
      spot_price_option=""
    fi
    ${AWS_ASG_CMD} create-launch-configuration \
        --launch-configuration-name ${ASG_NAME} \
        --image-id $KUBE_NODE_IMAGE \
        --iam-instance-profile ${IAM_PROFILE_NODE} \
        --instance-type $NODE_SIZE \
        --key-name ${AWS_SSH_KEY_NAME} \
        --security-groups ${NODE_SG_ID} \
        ${public_ip_option} \
        ${spot_price_option} \
        --block-device-mappings "${NODE_BLOCK_DEVICE_MAPPINGS}" \
        --user-data "fileb://${KUBE_TEMP}/node-user-data.gz"

    echo "Creating autoscaling group for num nodes $NUM_NODES"
    ${AWS_ASG_CMD} create-auto-scaling-group \
        --auto-scaling-group-name ${ASG_NAME} \
        --launch-configuration-name ${ASG_NAME} \
        --min-size ${NUM_NODES} \
        --max-size ${NUM_NODES} \
        --vpc-zone-identifier ${SUBNET_ID} \
        --tags ResourceId=${ASG_NAME},ResourceType=auto-scaling-group,Key=Name,Value=${NODE_INSTANCE_PREFIX} \
              ResourceId=${ASG_NAME},ResourceType=auto-scaling-group,Key=Role,Value=${NODE_TAG} \
              ResourceId=${ASG_NAME},ResourceType=auto-scaling-group,Key=KubernetesCluster,Value=${CLUSTER_ID}
  fi
}

function wait-minions {
  # Wait for the minions to be running
  # TODO(justinsb): This is really not needed any more
  local attempt=0
  local max_attempts=30
  # Spot instances are slower to launch
  if [[ -n "${NODE_SPOT_PRICE:-}" ]]; then
    max_attempts=90
  fi
  while true; do    
    detect-node-names > $LOG
    if [[ ${#NODE_IDS[@]} == ${NUM_NODES} ]]; then
      echo -e " ${color_green}${#NODE_IDS[@]} minions started; ready${color_norm}"
      break
    fi

    if (( attempt > max_attempts )); then
      echo
      echo "Expected number of minions did not start in time"
      echo
      echo -e "${color_red}Expected number of minions failed to start.  Your cluster is unlikely" >&2
      echo "to work correctly. Please run ./cluster/kube-down.sh and re-create the" >&2
      echo -e "cluster. (sorry!)${color_norm}" >&2
      exit 1
    fi

    echo -e " ${color_yellow}${#NODE_IDS[@]} minions started; waiting${color_norm}"
    attempt=$(($attempt+1))
    sleep 10
  done
}

# Wait for the master to be started
function wait-master() {
  if [[ "${PRESET_INSTANCES_ENABLED:-}" != "yes" ]]; then
    detect-master > $LOG
  fi

  echo "Waiting for cluster initialization."
  echo
  echo "  This will continually check to see if the API for kubernetes is reachable."
  echo "  This might loop forever if there was some uncaught error during start"
  echo "  up."
  echo

  local sshSts=0
  if [[ -f "$HOME/.ssh/known_hosts" ]]; then
    ssh-keygen -f "$HOME/.ssh/known_hosts" -R ${KUBE_MASTER_IP}
  fi
  while : ; do
    set +e
    if [[ "${PRESET_INSTANCES_ENABLED:-}" == "yes" ]]; then
      ssh -o 'StrictHostKeyChecking no' -i ${ACCESS_FILE} ${SSH_USER}@${KUBE_MASTER_IP} "ls /home/${SSH_USER}/.kube/config" &> /dev/null
    else
      ssh -o 'StrictHostKeyChecking no' -t ${SSH_USER}@${KUBE_MASTER_IP} "ls /home/${SSH_USER}/.kube/config" &> /dev/null
    fi 
    sshSts=$?
    set -e
    if [ $sshSts -eq 0 ]; then
      if [ -z "${LOCAL_KUBECONFIG:-}" ]; then
        mkdir -p $HOME/.kube
        if [[ "${PRESET_INSTANCES_ENABLED:-}" == "yes" ]]; then
          ssh -o 'StrictHostKeyChecking no' -i ${ACCESS_FILE} ${SSH_USER}@${KUBE_MASTER_IP} "cat /home/${SSH_USER}/.kube/config" > $HOME/.kube/config
        else
          ssh -o 'StrictHostKeyChecking no' -t ${SSH_USER}@${KUBE_MASTER_IP} "cat /home/${SSH_USER}/.kube/config" > $HOME/.kube/config
        fi
        sed -i "s/server: https:\/\/.*:6443/server: https:\/\/${KUBE_MASTER_IP}:6443/" $HOME/.kube/config
        LOCAL_KUBECONFIG="$HOME/.kube/config"
      else
        if [[ "${PRESET_INSTANCES_ENABLED:-}" == "yes" ]]; then
          ssh -o 'StrictHostKeyChecking no' -i ${ACCESS_FILE} ${SSH_USER}@${KUBE_MASTER_IP} "cat /home/${SSH_USER}/.kube/config" > $LOCAL_KUBECONFIG
        else
          ssh -o 'StrictHostKeyChecking no' -t ${SSH_USER}@${KUBE_MASTER_IP} "cat /home/${SSH_USER}/.kube/config" > $LOCAL_KUBECONFIG
        fi        
        sed -i "s/server: https:\/\/.*:${API_BIND_PORT}/server: https:\/\/${KUBE_MASTER_IP}:${API_BIND_PORT}/" $LOCAL_KUBECONFIG
      fi
      break
    fi
    printf "."
    sleep 5
  done

  echo "Kubernetes cluster created."
}

# Sanity check the cluster and print confirmation messages
function check-cluster() {
  echo "Sanity checking cluster..."

  if [[ "${PRESET_INSTANCES_ENABLED:-}" != "yes" ]]; then
    sleep 5
    
    detect-nodes > $LOG

    # Don't bail on errors, we want to be able to print some info.
    set +e

    # Basic sanity checking
    # TODO(justinsb): This is really not needed any more
    local rc # Capture return code without exiting because of errexit bash option
    for (( i=0; i<${#KUBE_NODE_IP_ADDRESSES[@]}; i++)); do
        # Make sure docker is installed and working.
        local attempt=0
        while true; do
          local minion_ip=${KUBE_NODE_IP_ADDRESSES[$i]}
          echo -n "Attempt $(($attempt+1)) to check Docker on node @ ${minion_ip} ..."
          local output=`check-minion ${minion_ip}`
          echo $output
          if [[ "${output}" != "working" ]]; then
            if (( attempt > 20 )); then
              echo
              echo -e "${color_red}Your cluster is unlikely to work correctly." >&2
              echo "Please run ./cluster/kube-down.sh and re-create the" >&2
              echo -e "cluster. (sorry!)${color_norm}" >&2
              exit 1
            fi
          else
            break
          fi
          attempt=$(($attempt+1))
          sleep 30
        done
    done
  fi

  # ensures KUBECONFIG is set
  get-kubeconfig-basicauth
  echo
  echo -e "${color_green}Kubernetes cluster is running.  The master is running at:"
  echo
  echo -e "${color_yellow}  https://${KUBE_MASTER_IP}"
  echo
  echo -e "${color_green}The user name and password to use is located in ${KUBECONFIG}.${color_norm}"
  echo
}

function kube-down {
  local vpc_id=$(get_vpc_id)
  if [[ -n "${vpc_id}" ]]; then
    local elb_ids=$(get_elbs_in_vpc ${vpc_id})
    if [[ -n "${elb_ids}" ]]; then
      echo "Deleting ELBs in: ${vpc_id}"
      for elb_id in ${elb_ids}; do
        aws elb delete-load-balancer --load-balancer-name=${elb_id} >$LOG
      done

      echo "Waiting for ELBs to be deleted"
      while true; do
        elb_ids=$(get_elbs_in_vpc ${vpc_id})
        if [[ -z "$elb_ids"  ]]; then
          echo "All ELBs deleted"
          break
        else
          echo "ELBs not yet deleted: $elb_ids"
          echo "Sleeping for 3 seconds..."
          sleep 3
        fi
      done
    fi

    if [[ -z "${KUBE_MASTER_ID-}" ]]; then
      KUBE_MASTER_ID=$(get_instanceid_from_name ${MASTER_NAME})
    fi
    if [[ -n "${KUBE_MASTER_ID-}" ]]; then
      delete-instance-alarms ${KUBE_MASTER_ID}
    fi

    echo "Deleting instances in VPC: ${vpc_id}"
    instance_ids=$($AWS_CMD describe-instances \
                            --filters Name=vpc-id,Values=${vpc_id} \
                                      Name=tag:KubernetesCluster,Values=${CLUSTER_ID} \
                            --query Reservations[].Instances[].InstanceId)

    if [[ -n "${instance_ids}" ]]; then
      asg_groups=$($AWS_CMD   describe-instances \
                              --query 'Reservations[].Instances[].Tags[?Key==`aws:autoscaling:groupName`].Value[]' \
                              --instance-ids ${instance_ids})
      for asg_group in ${asg_groups}; do
        if [[ -n $(${AWS_ASG_CMD} describe-auto-scaling-groups --auto-scaling-group-names ${asg_group} --query AutoScalingGroups[].AutoScalingGroupName) ]]; then
          echo "Deleting auto-scaling group: ${asg_group}"
          ${AWS_ASG_CMD} delete-auto-scaling-group --force-delete --auto-scaling-group-name ${asg_group}
        fi
        if [[ -n $(${AWS_ASG_CMD} describe-launch-configurations --launch-configuration-names ${asg_group} --query LaunchConfigurations[].LaunchConfigurationName) ]]; then
          echo "Deleting auto-scaling launch configuration: ${asg_group}"
          ${AWS_ASG_CMD} delete-launch-configuration --launch-configuration-name ${asg_group}
        fi
      done

      $AWS_CMD terminate-instances --instance-ids ${instance_ids} > $LOG
      echo "Waiting for instances to be deleted"
      for instance_id in ${instance_ids}; do
        wait-for-instance-state ${instance_id} "terminated"
      done
      echo "All instances deleted"
    fi
    if [[ -n $(${AWS_ASG_CMD} describe-launch-configurations --launch-configuration-names ${ASG_NAME} --query LaunchConfigurations[].LaunchConfigurationName) ]]; then
      echo "Warning: default auto-scaling launch configuration ${ASG_NAME} still exists, attempting to delete"
      echo "  (This may happen if kube-up leaves just the launch configuration but no auto-scaling group.)"
      ${AWS_ASG_CMD} delete-launch-configuration --launch-configuration-name ${ASG_NAME} || true
    fi

    find-master-pd
    find-tagged-master-ip

    if [[ -n "${KUBE_MASTER_IP:-}" ]]; then
      release-elastic-ip ${KUBE_MASTER_IP}
    fi

    if [[ -n "${MASTER_DISK_ID:-}" ]]; then
      echo "Deleting volume ${MASTER_DISK_ID}"
      $AWS_CMD delete-volume --volume-id ${MASTER_DISK_ID} > $LOG
    fi

    echo "Cleaning up resources in VPC: ${vpc_id}"
    default_sg_id=$($AWS_CMD describe-security-groups \
                             --filters Name=vpc-id,Values=${vpc_id} \
                                       Name=group-name,Values=default \
                             --query SecurityGroups[].GroupId \
                    | tr "\t" "\n")
    sg_ids=$($AWS_CMD describe-security-groups \
                      --filters Name=vpc-id,Values=${vpc_id} \
                                Name=tag:KubernetesCluster,Values=${CLUSTER_ID} \
                      --query SecurityGroups[].GroupId \
             | tr "\t" "\n")
    # First delete any inter-security group ingress rules
    # (otherwise we get dependency violations)
    for sg_id in ${sg_ids}; do
      # EC2 doesn't let us delete the default security group
      if [[ "${sg_id}" == "${default_sg_id}" ]]; then
        continue
      fi

      echo "Cleaning up security group: ${sg_id}"
      other_sgids=$(${AWS_CMD} describe-security-groups --group-id "${sg_id}" --query SecurityGroups[].IpPermissions[].UserIdGroupPairs[].GroupId)
      for other_sgid in ${other_sgids}; do
        $AWS_CMD revoke-security-group-ingress --group-id "${sg_id}" --source-group "${other_sgid}" --protocol all > $LOG
      done
    done

    for sg_id in ${sg_ids}; do
      # EC2 doesn't let us delete the default security group
      if [[ "${sg_id}" == "${default_sg_id}" ]]; then
        continue
      fi

      delete_security_group ${sg_id}
    done

    subnet_ids=$($AWS_CMD describe-subnets \
                          --filters Name=vpc-id,Values=${vpc_id} \
                                    Name=tag:KubernetesCluster,Values=${CLUSTER_ID} \
                          --query Subnets[].SubnetId \
             | tr "\t" "\n")
    for subnet_id in ${subnet_ids}; do
      $AWS_CMD delete-subnet --subnet-id ${subnet_id} > $LOG
    done

    igw_ids=$($AWS_CMD describe-internet-gateways \
                       --filters Name=attachment.vpc-id,Values=${vpc_id} \
                       --query InternetGateways[].InternetGatewayId \
             | tr "\t" "\n")
    for igw_id in ${igw_ids}; do
      $AWS_CMD detach-internet-gateway --internet-gateway-id $igw_id --vpc-id $vpc_id > $LOG
      $AWS_CMD delete-internet-gateway --internet-gateway-id $igw_id > $LOG
    done

    route_table_ids=$($AWS_CMD describe-route-tables \
                               --filters Name=vpc-id,Values=$vpc_id \
                                         Name=route.destination-cidr-block,Values=0.0.0.0/0 \
                               --query RouteTables[].RouteTableId \
                      | tr "\t" "\n")
    for route_table_id in ${route_table_ids}; do
      $AWS_CMD delete-route --route-table-id $route_table_id --destination-cidr-block 0.0.0.0/0 > $LOG
    done
    route_table_ids=$($AWS_CMD describe-route-tables \
                               --filters Name=vpc-id,Values=$vpc_id \
                                         Name=tag:KubernetesCluster,Values=${CLUSTER_ID} \
                               --query RouteTables[].RouteTableId \
                      | tr "\t" "\n")
    for route_table_id in ${route_table_ids}; do
      $AWS_CMD delete-route-table --route-table-id $route_table_id > $LOG
    done

    echo "Deleting VPC: ${vpc_id}"
    $AWS_CMD delete-vpc --vpc-id $vpc_id > $LOG
  elif [[ ! -z "$VPC_ID" ]]; then
    echo "Cleaning up resources for cluster created with pre-existing VPC $VPC_ID"
    if [[ -z "${KUBE_MASTER_ID-}" ]]; then
      KUBE_MASTER_ID=$(get_instanceid_from_name ${MASTER_NAME})
    fi
    echo "Deleting instance alarms for master $KUBE_MASTER_ID"
    if [[ -n "${KUBE_MASTER_ID-}" ]]; then
      delete-instance-alarms ${KUBE_MASTER_ID}
    fi

    instance_ids=$($AWS_CMD describe-instances \
                            --filters Name=vpc-id,Values=$VPC_ID \
                                      Name=tag:KubernetesCluster,Values=${CLUSTER_ID} \
                            --query Reservations[].Instances[].InstanceId)

    echo "Terminating cluster instances in VPC: $VPC_ID, Instances: $instance_ids"
    if [[ -n "${instance_ids}" ]]; then
      asg_groups=$($AWS_CMD   describe-instances \
                              --query 'Reservations[].Instances[].Tags[?Key==`aws:autoscaling:groupName`].Value[]' \
                              --instance-ids ${instance_ids})
      for asg_group in ${asg_groups}; do
        if [[ -n $(${AWS_ASG_CMD} describe-auto-scaling-groups --auto-scaling-group-names ${asg_group} --query AutoScalingGroups[].AutoScalingGroupName) ]]; then
          echo "Deleting auto-scaling group: ${asg_group}"
          ${AWS_ASG_CMD} delete-auto-scaling-group --force-delete --auto-scaling-group-name ${asg_group}
        fi
        if [[ -n $(${AWS_ASG_CMD} describe-launch-configurations --launch-configuration-names ${asg_group} --query LaunchConfigurations[].LaunchConfigurationName) ]]; then
          echo "Deleting auto-scaling launch configuration: ${asg_group}"
          ${AWS_ASG_CMD} delete-launch-configuration --launch-configuration-name ${asg_group}
        fi
      done

      $AWS_CMD terminate-instances --instance-ids ${instance_ids} > $LOG
      echo "Waiting for instances to be deleted"
      for instance_id in ${instance_ids}; do
        wait-for-instance-state ${instance_id} "terminated"
      done
      echo "All instances deleted"
    fi

    find-master-pd
    find-tagged-master-ip

    if [[ -n "${KUBE_MASTER_IP:-}" ]]; then
      echo "Releasing External IP $KUBE_MASTER_IP"
      release-elastic-ip ${KUBE_MASTER_IP}
    fi

    if [[ -n "${MASTER_DISK_ID:-}" ]]; then
      echo "Deleting volume ${MASTER_DISK_ID}"
      $AWS_CMD delete-volume --volume-id ${MASTER_DISK_ID} > $LOG
    fi

    echo "Cleaning up specific resources in pre-existing VPC: $VPC_ID"
    default_sg_id=$($AWS_CMD describe-security-groups \
                             --filters Name=vpc-id,Values=$VPC_ID \
                                       Name=group-name,Values=default \
                             --query SecurityGroups[].GroupId \
                    | tr "\t" "\n")
    sg_ids=$($AWS_CMD describe-security-groups \
                      --filters Name=vpc-id,Values=$VPC_ID \
                                Name=tag:KubernetesCluster,Values=${CLUSTER_ID} \
                      --query SecurityGroups[].GroupId \
             | tr "\t" "\n")
    # First delete any inter-security group ingress rules
    # (otherwise we get dependency violations)
    for sg_id in ${sg_ids}; do
      # EC2 doesn't let us delete the default security group
      if [[ "${sg_id}" == "${default_sg_id}" ]]; then
        continue
      fi

      echo "Cleaning up security group: ${sg_id}"
      other_sgids=$(${AWS_CMD} describe-security-groups --group-id "${sg_id}" --query SecurityGroups[].IpPermissions[].UserIdGroupPairs[].GroupId)
      for other_sgid in ${other_sgids}; do
      echo "Revoking ingress for group: $other_sgid"
        $AWS_CMD revoke-security-group-ingress --group-id "${sg_id}" --source-group "${other_sgid}" --protocol all > $LOG
      done
    done

    for sg_id in ${sg_ids}; do
      # EC2 doesn't let us delete the default security group
      if [[ "${sg_id}" == "${default_sg_id}" ]]; then
        continue
      fi

      echo "Deleting security group $sg_id"
      delete_security_group ${sg_id}
    done

    echo "Deleting specific route tables in VPC $VPC_ID"
    route_table_ids=$($AWS_CMD describe-route-tables \
                               --filters Name=vpc-id,Values=$VPC_ID \
                                         Name=tag:KubernetesCluster,Values=${CLUSTER_ID} \
                               --query RouteTables[].RouteTableId \
                      | tr "\t" "\n")
    for route_table_id in ${route_table_ids}; do
      echo "Disassociating and deleting route table $route_table_id"
      assoc_id=$($AWS_CMD describe-route-tables --route-table-id $route_table_id --query RouteTables[].Associations[].RouteTableAssociationId --output=text)
      if [ ! -z "$assoc_id" ]; then
        $AWS_CMD disassociate-route-table --association-id $assoc_id
      fi
      $AWS_CMD delete-route-table --route-table-id $route_table_id > $LOG
    done

  else
    echo "" >&2
    echo -e "${color_red}Cluster NOT deleted!${color_norm}" >&2
    echo "" >&2
    echo "No VPC was found with tag KubernetesCluster=${CLUSTER_ID}" >&2
    echo "" >&2
    echo "If you are trying to delete a cluster in a shared VPC," >&2
    echo "please consider using one of the methods in the kube-deploy repo." >&2
    echo "See: https://github.com/kubernetes/kube-deploy/blob/master/docs/delete_cluster.md" >&2
    echo "" >&2
    echo "Note: You may be seeing this message may be because the cluster was already deleted, or" >&2
    echo "has a name other than '${CLUSTER_ID}'." >&2
  fi

  if [[ -z "${DHCP_OPTION_SET_ID:-}" ]]; then
    dhcp_option_ids=$($AWS_CMD describe-dhcp-options \
                               --output text \
                               --filters Name=tag:Name,Values=kubernetes-dhcp-option-set \
                                         Name=tag:KubernetesCluster,Values=${CLUSTER_ID} \
                               --query DhcpOptions[].DhcpOptionsId \
                      | tr "\t" "\n")
    if [[ -z "$VPC_ID" ]]; then
      for dhcp_option_id in ${dhcp_option_ids}; do
        echo "Deleting DHCP option set: ${dhcp_option_id}"
        $AWS_CMD delete-dhcp-options --dhcp-options-id $dhcp_option_id > $LOG
      done
    else
      echo "Not deleting DHCP options $dhcp_option_ids for pre-existing VPC. Please clean up manually if desired"
    fi
  else
    echo "Skipping deletion of pre-existing DHCP option set: ${DHCP_OPTION_SET_ID}"
  fi

  echo "Deleting IAM Instance profiles"
  delete-iam-profiles
}

function reboot-remote-vm {
  local ip="$1"

  if [[ "${PRESET_INSTANCES_ENABLED:-}" == "yes" ]]; then
    ssh -o 'StrictHostKeyChecking no' -i ${ACCESS_FILE} ${SSH_USER}@${ip} "sudo reboot" || true
  else
    ssh -o 'StrictHostKeyChecking no' -t ${SSH_USER}@${ip} "sudo reboot" || true
  fi 

  echo "Waiting for vm reboot."
  echo
  echo "  This will continually check to see if vm is reachable."
  echo "  This might loop forever if there was some uncaught error during reboot."
  echo

  local sshSts=0
  while : ; do
    set +e
    if [[ "${PRESET_INSTANCES_ENABLED:-}" == "yes" ]]; then
      ssh -o 'StrictHostKeyChecking no' -i ${ACCESS_FILE} ${SSH_USER}@${ip} "ls /" &> /dev/null
    else
      ssh -o 'StrictHostKeyChecking no' -t ${SSH_USER}@${ip} "ls /" &> /dev/null
    fi 
    sshSts=$?
    set -e
    if [ $sshSts -eq 0 ]; then      
      break
    fi
    printf "."
    sleep 5
  done
  echo "done vm reboot."
}

function kube-down-for-preset-machines {
  create-cleanup-script

  scp -o 'StrictHostKeyChecking no' -i ${ACCESS_FILE} ${CLEANUP_SCRIPT} ${SSH_USER}@${KUBE_MASTER_IP}:/tmp
  ssh -o 'StrictHostKeyChecking no' -i ${ACCESS_FILE} ${SSH_USER}@${KUBE_MASTER_IP} "chmod 755 /tmp/cleanup-script && sudo /tmp/cleanup-script && rm -rf /tmp/cleanup-script"
  reboot-remote-vm ${KUBE_MASTER_IP}
  ssh -o 'StrictHostKeyChecking no' -i ${ACCESS_FILE} ${SSH_USER}@${KUBE_MASTER_IP} "sudo rm -rf /var/lib/kubelet"  

  scp -o 'StrictHostKeyChecking no' -i ${ACCESS_FILE} ${CLEANUP_SCRIPT} ${SSH_USER}@${KUBE_MINION1_IP}:/tmp
  ssh -o 'StrictHostKeyChecking no' -i ${ACCESS_FILE} ${SSH_USER}@${KUBE_MINION1_IP} "chmod 755 /tmp/cleanup-script && sudo /tmp/cleanup-script && rm -rf /tmp/cleanup-script"
  reboot-remote-vm ${KUBE_MINION1_IP}
  ssh -o 'StrictHostKeyChecking no' -i ${ACCESS_FILE} ${SSH_USER}@${KUBE_MINION1_IP} "sudo rm -rf /var/lib/kubelet"

  if [[ -n ${KUBE_MINION2_IP:-} ]]; then
    scp -o 'StrictHostKeyChecking no' -i ${ACCESS_FILE} ${CLEANUP_SCRIPT} ${SSH_USER}@${KUBE_MINION2_IP}:/tmp
    ssh -o 'StrictHostKeyChecking no' -i ${ACCESS_FILE} ${SSH_USER}@${KUBE_MINION2_IP} "chmod 755 /tmp/cleanup-script && sudo /tmp/cleanup-script && rm -rf /tmp/cleanup-script"
    reboot-remote-vm ${KUBE_MINION2_IP}
    ssh -o 'StrictHostKeyChecking no' -i ${ACCESS_FILE} ${SSH_USER}@${KUBE_MINION2_IP} "sudo rm -rf /var/lib/kubelet"
  fi
}

# Update a kubernetes cluster with latest source
function kube-push {
  detect-master

  # Make sure we have the tar files staged on Google Storage
  find-release-tars
  create-bootstrap-script
  upload-server-tars

  (
    echo "#! /bin/bash"
    echo "mkdir -p /var/cache/kubernetes-install"
    echo "cd /var/cache/kubernetes-install"
    echo "readonly SERVER_BINARY_TAR_URL='${SERVER_BINARY_TAR_URL}'"
    grep -v "^#" "${KUBE_ROOT}/cluster/aws/templates/common.sh"
    grep -v "^#" "${KUBE_ROOT}/cluster/aws/templates/download-release.sh"
    echo "echo Executing configuration"
  ) | ssh -oStrictHostKeyChecking=no -i "${AWS_SSH_KEY}" ${SSH_USER}@${KUBE_MASTER_IP} sudo bash

  get-kubeconfig-basicauth

  echo
  echo "Kubernetes cluster is running.  The master is running at:"
  echo
  echo "  https://${KUBE_MASTER_IP}"
  echo

}

# -----------------------------------------------------------------------------
# Cluster specific test helpers used from hack/e2e.go

# Execute prior to running tests to build a release if required for env.
#
# Assumed Vars:
#   KUBE_ROOT
function test-build-release {
  # Make a release
  "${KUBE_ROOT}/build/release.sh"
}

# Execute prior to running tests to initialize required structure. This is
# called from hack/e2e.go only when running -up.
#
# Assumed vars:
#   Variables from config.sh
function test-setup {
  "${KUBE_ROOT}/cluster/kube-up.sh"

  if [[ -z "${VPC_ID:-}" ]]; then
    VPC_ID=$(get_vpc_id)
  fi
  detect-security-groups

  # Open up port 80 & 8080 so common containers on minions can be reached
  # TODO(roberthbailey): Remove this once we are no longer relying on hostPorts.
  authorize-security-group-ingress "${NODE_SG_ID}" "--protocol tcp --port 80 --cidr 0.0.0.0/0"
  authorize-security-group-ingress "${NODE_SG_ID}" "--protocol tcp --port 8080 --cidr 0.0.0.0/0"

  # Open up the NodePort range
  # TODO(justinsb): Move to main setup, if we decide whether we want to do this by default.
  authorize-security-group-ingress "${NODE_SG_ID}" "--protocol all --port 30000-32767 --cidr 0.0.0.0/0"

  echo "test-setup complete"
}

# Execute after running tests to perform any required clean-up. This is called
# from hack/e2e.go
function test-teardown {
  # (ingress rules will be deleted along with the security group)
  echo "Shutting down test cluster."
  "${KUBE_ROOT}/cluster/kube-down.sh"
}


# Gets the hostname (or IP) that we should SSH to for the given nodename
# For the master, we use the nodename, for the nodes we use their instanceids
function get_ssh_hostname {
  local node="$1"

  if [[ "${node}" == "${MASTER_NAME}" ]]; then
    node=$(get_instanceid_from_name ${MASTER_NAME})
    if [[ -z "${node-}" ]]; then
      echo "Could not detect Kubernetes master node.  Make sure you've launched a cluster with 'kube-up.sh'" 1>&2
      exit 1
    fi
  fi

  local ip=$(get_instance_public_ip ${node})
  if [[ -z "$ip" ]]; then
    echo "Could not detect IP for ${node}." 1>&2
    exit 1
  fi
  echo ${ip}
}

# SSH to a node by name ($1) and run a command ($2).
function ssh-to-node {
  local node="$1"
  local cmd="$2"

  local ip=$(get_ssh_hostname ${node})

  for try in {1..5}; do
    if ssh -oLogLevel=quiet -oConnectTimeout=30 -oStrictHostKeyChecking=no -i "${AWS_SSH_KEY}" ${SSH_USER}@${ip} "echo test > /dev/null"; then
      break
    fi
    sleep 5
  done
  ssh -oLogLevel=quiet -oConnectTimeout=30 -oStrictHostKeyChecking=no -i "${AWS_SSH_KEY}" ${SSH_USER}@${ip} "${cmd}"
}

# Perform preparations required to run e2e tests
function prepare-e2e() {
  # (AWS runs detect-project, I don't think we need to anything)
  # Note: we can't print anything here, or else the test tools will break with the extra output
  return
}

function get-tokens() {
  KUBELET_TOKEN=$(dd if=/dev/urandom bs=128 count=1 2>/dev/null | base64 | tr -d "=+/" | dd bs=32 count=1 2>/dev/null)
  KUBE_PROXY_TOKEN=$(dd if=/dev/urandom bs=128 count=1 2>/dev/null | base64 | tr -d "=+/" | dd bs=32 count=1 2>/dev/null)
}
