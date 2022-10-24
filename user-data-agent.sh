#!/bin/bash -xe

exec > >(tee /var/log/user-data.log|logger -t user-data -s 2>/dev/console) 2>&1
  yum -y update
  curl -sfL https://get.k3s.io | K3S_TOKEN=%s K3S_URL=https://%s:6443 sh -
  reboot